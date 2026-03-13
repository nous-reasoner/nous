package network

import (
	"fmt"
	"log"
	"net"
	"path/filepath"
	"sync"
	"time"
)

// ServerConfig holds configuration for a Server.
type ServerConfig struct {
	ListenAddr string   // e.g. ":9333"
	Magic      uint32   // network magic bytes
	Seeds      []string // seed node addresses (e.g. "seed1.nous.io:9333")
	MaxPeers   int      // override max connections (0 = default)
	DataDir    string   // data directory for persistence (peers.json)
}

// DefaultConfig returns a sensible default configuration.
func DefaultConfig() ServerConfig {
	return ServerConfig{
		ListenAddr: fmt.Sprintf(":%d", DefaultPort),
		Magic:      MainNetMagic,
		Seeds:      nil,
		MaxPeers:   MaxConnections,
	}
}

// MessageHandler is called when a message is received from a peer.
type MessageHandler func(peer *Peer, msg Message)

// Server is the main P2P network node.
type Server struct {
	config     ServerConfig
	listener   net.Listener
	peers      *PeerManager
	addrBook   *AddressBook
	mempool    *Mempool
	protection *PeerProtection
	handlers   map[string]MessageHandler

	blockHeight uint64 // our current block height

	quit chan struct{}
	wg   sync.WaitGroup
	mu   sync.RWMutex
}

// NewServer creates a new P2P server.
func NewServer(config ServerConfig) *Server {
	var addrBook *AddressBook
	if config.DataDir != "" {
		path := filepath.Join(config.DataDir, "peers.json")
		var err error
		addrBook, err = LoadAddressBook(path, config.Seeds)
		if err != nil {
			log.Printf("network: load address book: %v", err)
			addrBook = NewAddressBook(config.Seeds)
		}
	} else {
		addrBook = NewAddressBook(config.Seeds)
	}
	return &Server{
		config:     config,
		peers:      NewPeerManager(),
		addrBook:   addrBook,
		mempool:    NewMempool(),
		protection: NewPeerProtection(),
		handlers:   make(map[string]MessageHandler),
		quit:       make(chan struct{}),
	}
}

// SetBlockHeight updates the server's advertised block height.
func (s *Server) SetBlockHeight(height uint64) {
	s.mu.Lock()
	s.blockHeight = height
	s.mu.Unlock()
}

// BlockHeight returns the current block height.
func (s *Server) BlockHeight() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.blockHeight
}

// Peers returns the peer manager.
func (s *Server) Peers() *PeerManager { return s.peers }

// Protection returns the peer protection manager.
func (s *Server) Protection() *PeerProtection { return s.protection }

// Mempool returns the transaction mempool.
func (s *Server) Mempool() *Mempool { return s.mempool }

// AddrBook returns the address book.
func (s *Server) AddrBook() *AddressBook { return s.addrBook }

// OnMessage registers a handler for a specific command.
func (s *Server) OnMessage(cmd string, handler MessageHandler) {
	s.handlers[cmd] = handler
}

// Start begins listening for connections and connects to seed nodes.
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", s.config.ListenAddr)
	if err != nil {
		return fmt.Errorf("network: listen: %w", err)
	}
	s.listener = ln

	// Register default handlers.
	s.registerDefaults()

	// Accept inbound connections.
	s.wg.Add(1)
	go s.acceptLoop()

	// Connect to seed nodes.
	s.wg.Add(1)
	go s.connectSeeds()

	// Periodic maintenance (ping, cleanup).
	s.wg.Add(1)
	go s.maintenanceLoop()

	// Periodic addr broadcast, auto-connect, and address book persistence.
	s.wg.Add(1)
	go s.addrLoop()

	return nil
}

// Stop gracefully shuts down the server.
func (s *Server) Stop() error {
	close(s.quit)
	if s.listener != nil {
		s.listener.Close()
	}
	// Save address book.
	s.saveAddrBook()
	// Close all peers.
	for _, p := range s.peers.All() {
		p.Close()
	}
	s.wg.Wait()
	return nil
}

// saveAddrBook persists the address book to disk if DataDir is configured.
func (s *Server) saveAddrBook() {
	if s.config.DataDir == "" {
		return
	}
	path := filepath.Join(s.config.DataDir, "peers.json")
	if err := s.addrBook.SaveToFile(path); err != nil {
		log.Printf("network: save address book: %v", err)
	}
}

// ListenAddr returns the actual listen address (useful for ephemeral ports).
func (s *Server) ListenAddr() string {
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return s.config.ListenAddr
}

// Connect establishes an outbound connection to a peer.
func (s *Server) Connect(addr string) error {
	// Don't connect to banned peers.
	if s.protection.IsBanned(addr) {
		return fmt.Errorf("network: peer %s is banned", addr)
	}

	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return err
	}

	peer := NewPeer(addr, conn, false)
	if !s.peers.Add(peer) {
		conn.Close()
		return fmt.Errorf("network: connection limit reached")
	}

	// Start handling the peer.
	s.wg.Add(1)
	go s.handlePeer(peer)

	// Send version handshake.
	s.sendVersion(peer)

	return nil
}

// BroadcastMessage sends a message to all connected, handshaked peers.
func (s *Server) BroadcastMessage(msg Message) {
	for _, p := range s.peers.All() {
		if p.Handshaked {
			p.SendMessage(s.config.Magic, msg)
		}
	}
}

// --- internal loops ---

func (s *Server) acceptLoop() {
	defer s.wg.Done()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.quit:
				return
			default:
				continue
			}
		}

		addr := conn.RemoteAddr().String()

		// Reject banned peers.
		if s.protection.IsBanned(addr) {
			conn.Close()
			continue
		}

		peer := NewPeer(addr, conn, true)
		if !s.peers.Add(peer) {
			conn.Close()
			continue
		}

		s.wg.Add(1)
		go s.handlePeer(peer)
	}
}

func (s *Server) connectSeeds() {
	defer s.wg.Done()
	for _, seed := range s.config.Seeds {
		select {
		case <-s.quit:
			return
		default:
		}
		if err := s.Connect(seed); err != nil {
			log.Printf("network: seed %s: %v", seed, err)
		}
	}
}

func (s *Server) maintenanceLoop() {
	defer s.wg.Done()
	ticker := time.NewTicker(PingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.quit:
			return
		case <-ticker.C:
			// Remove inactive peers.
			s.peers.RemoveInactive()
			// Ping remaining peers.
			nonce := uint64(time.Now().UnixNano())
			for _, p := range s.peers.All() {
				if p.Handshaked {
					p.SendMessage(s.config.Magic, &MsgPing{Nonce: nonce})
				}
			}
		}
	}
}

const (
	// AddrBroadcastInterval is how often we broadcast addresses to peers.
	AddrBroadcastInterval = 30 * time.Minute

	// AutoConnectInterval is how often we try to connect to new peers.
	AutoConnectInterval = 60 * time.Second

	// AddrBookSaveInterval is how often we persist the address book.
	AddrBookSaveInterval = 5 * time.Minute

	// MaxAddrFailures is the number of connection failures before removing an address.
	MaxAddrFailures = 3

	// StaleAddrAge is the maximum age of an address entry before it is removed.
	StaleAddrAge = 3 * time.Hour
)

func (s *Server) addrLoop() {
	defer s.wg.Done()

	addrTicker := time.NewTicker(AddrBroadcastInterval)
	connectTicker := time.NewTicker(AutoConnectInterval)
	saveTicker := time.NewTicker(AddrBookSaveInterval)
	staleTicker := time.NewTicker(StaleAddrAge)
	defer addrTicker.Stop()
	defer connectTicker.Stop()
	defer saveTicker.Stop()
	defer staleTicker.Stop()

	for {
		select {
		case <-s.quit:
			return

		case <-addrTicker.C:
			s.broadcastAddresses()

		case <-connectTicker.C:
			s.autoConnect()

		case <-saveTicker.C:
			s.saveAddrBook()

		case <-staleTicker.C:
			if n := s.addrBook.RemoveStale(StaleAddrAge); n > 0 {
				log.Printf("network: removed %d stale addresses", n)
			}
		}
	}
}

// broadcastAddresses sends our own address + up to 10 random from the book.
func (s *Server) broadcastAddresses() {
	var addrs []NetAddress

	// Add our own listen address.
	if s.listener != nil {
		if tcpAddr, ok := s.listener.Addr().(*net.TCPAddr); ok {
			// Only broadcast our address if we know our external IP.
			// For now, broadcast with the listen port so peers can learn it.
			addrs = append(addrs, NetAddress{
				IP:   tcpAddr.IP.String(),
				Port: uint16(tcpAddr.Port),
			})
		}
	}

	// Add up to 10 random addresses from the book.
	known := s.addrBook.GetAddresses(10)
	addrs = append(addrs, known...)

	if len(addrs) == 0 {
		return
	}

	msg := &MsgAddr{Addresses: addrs}
	for _, p := range s.peers.All() {
		if p.Handshaked {
			p.SendMessage(s.config.Magic, msg)
		}
	}
}

// autoConnect tries to fill outbound slots from the address book.
// Seed nodes are always retried regardless of failure count.
func (s *Server) autoConnect() {
	// Count current outbound peers.
	outbound := 0
	connected := make(map[string]bool)
	for _, p := range s.peers.All() {
		if !p.Inbound {
			outbound++
		}
		// Track both outbound addr and inbound addr (peer may connect to us).
		connected[p.Addr] = true
	}

	// Always retry seed nodes first — seeds must stay connected.
	seedSet := make(map[string]bool)
	for _, seed := range s.config.Seeds {
		seedSet[seed] = true
		if connected[seed] {
			continue
		}
		// Check if seed is already connected as inbound (different port).
		alreadyConnected := false
		seedHost, _, _ := net.SplitHostPort(seed)
		for _, p := range s.peers.All() {
			peerHost, _, _ := net.SplitHostPort(p.Addr)
			if peerHost == seedHost {
				alreadyConnected = true
				break
			}
		}
		if alreadyConnected {
			continue
		}
		if s.protection.IsBanned(seed) {
			continue
		}
		if err := s.Connect(seed); err == nil {
			outbound++
		}
	}

	if outbound >= MaxOutbound {
		return
	}

	// Pick from address book, skipping connected and high-failure addresses.
	candidates := s.addrBook.GetGoodAddresses(MaxOutbound*2, MaxAddrFailures)
	for _, addr := range candidates {
		if outbound >= MaxOutbound {
			break
		}
		key := net.JoinHostPort(addr.IP, fmt.Sprintf("%d", addr.Port))
		if connected[key] || seedSet[key] {
			continue
		}
		if s.protection.IsBanned(key) {
			continue
		}
		if err := s.Connect(key); err != nil {
			s.addrBook.RecordFailure(key, MaxAddrFailures)
		} else {
			outbound++
		}
	}
}

func (s *Server) handlePeer(peer *Peer) {
	defer s.wg.Done()
	defer func() {
		s.peers.Remove(peer.Addr)
		s.protection.RemovePeer(peer.Addr)
	}()

	// Handshake timeout: the first message must arrive within HandshakeTimeout.
	if peer.Conn != nil {
		peer.Conn.SetReadDeadline(time.Now().Add(HandshakeTimeout))
	}

	for {
		select {
		case <-s.quit:
			return
		default:
		}

		msg, err := DecodeMessage(peer.Conn, s.config.Magic)
		if err != nil {
			return
		}

		peer.UpdateActivity()

		// After handshake completes, use the normal inactive timeout.
		if peer.Handshaked && peer.Conn != nil {
			peer.Conn.SetReadDeadline(time.Now().Add(InactiveTimeout))
		}

		// Rate limit check — exempt block-sync messages (block, inv, getblocks,
		// getdata) so initial block download doesn't trigger bans.
		cmd := msg.Command()
		isSyncMsg := cmd == CmdBlock || cmd == CmdInv || cmd == CmdGetBlocks || cmd == CmdGetData
		if !isSyncMsg && !s.protection.CheckRate(peer.Addr) {
			if s.protection.AddScore(peer.Addr, BanScoreRateExceeded) {
				log.Printf("network: disconnecting banned peer %s (rate exceeded)", peer.Addr)
				return
			}
			continue // drop the message but keep the connection
		}

		// Dispatch to handler.
		if handler, ok := s.handlers[cmd]; ok {
			handler(peer, msg)
		} else {
			// Unknown command → penalize.
			if s.protection.AddScore(peer.Addr, BanScoreUnknownCmd) {
				log.Printf("network: disconnecting banned peer %s (unknown cmd %q)", peer.Addr, cmd)
				return
			}
		}
	}
}

func (s *Server) registerDefaults() {
	// Version handshake.
	if _, ok := s.handlers[CmdVersion]; !ok {
		s.handlers[CmdVersion] = s.handleVersion
	}
	if _, ok := s.handlers[CmdVerAck]; !ok {
		s.handlers[CmdVerAck] = s.handleVerAck
	}
	if _, ok := s.handlers[CmdPing]; !ok {
		s.handlers[CmdPing] = s.handlePing
	}
	if _, ok := s.handlers[CmdPong]; !ok {
		s.handlers[CmdPong] = s.handlePong
	}
	if _, ok := s.handlers[CmdAddr]; !ok {
		s.handlers[CmdAddr] = s.handleAddr
	}
	if _, ok := s.handlers[CmdGetAddr]; !ok {
		s.handlers[CmdGetAddr] = s.handleGetAddr
	}
}

func (s *Server) sendVersion(peer *Peer) {
	listenPort := uint16(DefaultPort)
	if s.listener != nil {
		if addr, ok := s.listener.Addr().(*net.TCPAddr); ok {
			listenPort = uint16(addr.Port)
		}
	}
	msg := &MsgVersion{
		Version:     ProtocolVersion,
		BlockHeight: s.BlockHeight(),
		Timestamp:   uint64(time.Now().Unix()),
		Nonce:       uint64(time.Now().UnixNano()),
		UserAgent:   "nous/0.1.0",
		ListenPort:  listenPort,
	}
	peer.SendMessage(s.config.Magic, msg)
}

func (s *Server) handleVersion(peer *Peer, msg Message) {
	ver := msg.(*MsgVersion)
	peer.Version = ver.Version
	peer.BlockHeight = ver.BlockHeight

	// Reject peers running an incompatible protocol version.
	if ver.Version < MinSupportedVersion {
		log.Printf("network: rejecting peer %s: version %d < minimum %d", peer.Addr, ver.Version, MinSupportedVersion)
		peer.Close()
		return
	}

	// Send verack.
	peer.SendMessage(s.config.Magic, &MsgVerAck{})

	// If this is an inbound connection, send our version too.
	if peer.Inbound && !peer.Handshaked {
		s.sendVersion(peer)
	}
}

func (s *Server) handleVerAck(peer *Peer, msg Message) {
	peer.Handshaked = true
	// Request known addresses from this peer.
	peer.SendMessage(s.config.Magic, &MsgGetAddr{})
}

func (s *Server) handlePing(peer *Peer, msg Message) {
	ping := msg.(*MsgPing)
	peer.SendMessage(s.config.Magic, &MsgPong{Nonce: ping.Nonce})
}

func (s *Server) handlePong(peer *Peer, msg Message) {
	peer.UpdateActivity()
}

func (s *Server) handleAddr(peer *Peer, msg Message) {
	addrMsg := msg.(*MsgAddr)
	filtered := s.filterAddresses(addrMsg.Addresses)
	s.addrBook.AddAddresses(filtered)
}

func (s *Server) handleGetAddr(peer *Peer, msg Message) {
	addrs := s.addrBook.GetAddresses(MaxAddrCount)
	if len(addrs) > 0 {
		peer.SendMessage(s.config.Magic, &MsgAddr{Addresses: addrs})
	}
}

// filterAddresses removes addresses that are self, already connected, or private.
func (s *Server) filterAddresses(addrs []NetAddress) []NetAddress {
	selfPort := uint16(DefaultPort)
	if s.listener != nil {
		if addr, ok := s.listener.Addr().(*net.TCPAddr); ok {
			selfPort = uint16(addr.Port)
		}
	}

	var result []NetAddress
	for _, addr := range addrs {
		// Skip private/unroutable IPs.
		if isPrivateIP(addr.IP) {
			continue
		}
		// Skip self.
		if addr.Port == selfPort && isSelfIP(addr.IP) {
			continue
		}
		// Skip already connected peers.
		key := net.JoinHostPort(addr.IP, fmt.Sprintf("%d", addr.Port))
		if s.peers.Get(key) != nil {
			continue
		}
		result = append(result, addr)
	}
	return result
}

// isPrivateIP returns true for loopback and RFC1918 addresses.
func isPrivateIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return true // unparseable → reject
	}
	if ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	// RFC1918: 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16
	if ip4 := ip.To4(); ip4 != nil {
		if ip4[0] == 10 {
			return true
		}
		if ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31 {
			return true
		}
		if ip4[0] == 192 && ip4[1] == 168 {
			return true
		}
	}
	return false
}

// isSelfIP returns true if the IP is a local address on this machine.
func isSelfIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	if ip.IsLoopback() {
		return true
	}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}
	for _, a := range addrs {
		if ipNet, ok := a.(*net.IPNet); ok && ipNet.IP.Equal(ip) {
			return true
		}
	}
	return false
}

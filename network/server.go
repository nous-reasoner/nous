package network

import (
	"fmt"
	"log"
	"math/rand"
	"net"
	"path/filepath"
	"sort"
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
	addrMgr    *AddrManager
	mempool    *Mempool
	protection *PeerProtection
	handlers   map[string]MessageHandler

	blockHeight     uint64 // our current block height
	protocolVersion uint32 // override ProtocolVersion if non-zero

	quit chan struct{}
	wg   sync.WaitGroup
	mu   sync.RWMutex
}

// NewServer creates a new P2P server.
func NewServer(config ServerConfig) *Server {
	var filePath string
	if config.DataDir != "" {
		filePath = filepath.Join(config.DataDir, "peers.json")
	}
	addrMgr := NewAddrManager(filePath, config.Seeds)
	if filePath != "" {
		if err := addrMgr.LoadFromFile(); err != nil {
			log.Printf("network: load addr manager: %v", err)
		}
	}
	return &Server{
		config:     config,
		peers:      NewPeerManager(),
		addrMgr:    addrMgr,
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

// SetProtocolVersion overrides the protocol version this server advertises.
func (s *Server) SetProtocolVersion(v uint32) {
	s.mu.Lock()
	s.protocolVersion = v
	s.mu.Unlock()
}

// getProtocolVersion returns the protocol version to advertise.
func (s *Server) getProtocolVersion() uint32 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.protocolVersion != 0 {
		return s.protocolVersion
	}
	return ProtocolVersion
}

// Peers returns the peer manager.
func (s *Server) Peers() *PeerManager { return s.peers }

// Protection returns the peer protection manager.
func (s *Server) Protection() *PeerProtection { return s.protection }

// Mempool returns the transaction mempool.
func (s *Server) Mempool() *Mempool { return s.mempool }

// AddrMgr returns the address manager.
func (s *Server) AddrMgr() *AddrManager { return s.addrMgr }

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
	// Save address manager.
	s.saveAddrMgr()
	// Close all peers.
	for _, p := range s.peers.All() {
		p.Close()
	}
	s.wg.Wait()
	return nil
}

// saveAddrMgr persists the address manager to disk.
func (s *Server) saveAddrMgr() {
	if err := s.addrMgr.SaveToFile(); err != nil {
		log.Printf("network: save addr manager: %v", err)
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
			// Inbound full — try to evict the worst inbound peer.
			victim := s.selectEvictionCandidate()
			if victim == "" {
				conn.Close()
				continue
			}
			s.peers.Remove(victim)
			if !s.peers.Add(peer) {
				conn.Close()
				continue
			}
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
			// Ping remaining peers and track send time for latency.
			nonce := uint64(time.Now().UnixNano())
			for _, p := range s.peers.All() {
				if p.Handshaked {
					p.PingSentAt = time.Now()
					p.SendMessage(s.config.Magic, &MsgPing{Nonce: nonce})
				}
			}
		}
	}
}

const (
	// AutoConnectInterval is how often we try to connect to new peers.
	AutoConnectInterval = 30 * time.Second

	// AddrMgrSaveInterval is how often we persist the address manager.
	AddrMgrSaveInterval = 15 * time.Minute
)

func (s *Server) addrLoop() {
	defer s.wg.Done()

	connectTicker := time.NewTicker(AutoConnectInterval)
	saveTicker := time.NewTicker(AddrMgrSaveInterval)
	defer connectTicker.Stop()
	defer saveTicker.Stop()

	for {
		select {
		case <-s.quit:
			return

		case <-connectTicker.C:
			s.autoConnect()

		case <-saveTicker.C:
			s.saveAddrMgr()
		}
	}
}

// autoConnect tries to fill outbound slots from the address manager.
// Seed nodes are always retried regardless of failure count.
func (s *Server) autoConnect() {
	// Count current outbound peers.
	outbound := 0
	connected := make(map[string]bool)
	for _, p := range s.peers.All() {
		if !p.Inbound {
			outbound++
		}
		connected[p.Addr] = true
	}

	// Build a set of connected peer IPs for dedup.
	connectedIPs := make(map[string]bool)
	for _, p := range s.peers.All() {
		peerHost, _, _ := net.SplitHostPort(p.Addr)
		connectedIPs[peerHost] = true
	}

	// Always retry seed nodes first — seeds must stay connected.
	seedSet := make(map[string]bool)
	for _, seed := range s.addrMgr.Seeds() {
		seedSet[seed] = true
		if connected[seed] {
			continue
		}
		seedHost, _, _ := net.SplitHostPort(seed)
		alreadyConnected := connectedIPs[seedHost]
		if !alreadyConnected {
			if ips, err := net.LookupHost(seedHost); err == nil {
				for _, ip := range ips {
					if connectedIPs[ip] {
						alreadyConnected = true
						break
					}
				}
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

	// Pick from addr manager.
	for outbound < MaxOutbound {
		addr := s.addrMgr.SelectForOutbound()
		if addr == nil {
			break
		}
		key := addrKey(*addr)
		if connected[key] || seedSet[key] {
			continue
		}
		if s.protection.IsBanned(key) {
			continue
		}
		s.addrMgr.MarkAttempt(*addr)
		if err := s.Connect(key); err != nil {
			s.addrMgr.MarkFailed(*addr)
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
		isSyncMsg := cmd == CmdBlock || cmd == CmdInv || cmd == CmdGetBlocks || cmd == CmdGetData || cmd == CmdGetHeaders || cmd == CmdHeaders
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
			// Unknown command: payload was already read and checksum verified
			// by DecodeMessage. Silently discard for forward compatibility.
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
		Version:     s.getProtocolVersion(),
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
	peer.StartingHeight = ver.BlockHeight
	peer.BlockHeight = ver.BlockHeight
	peer.ListenPort = ver.ListenPort

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

	// Request addresses only from v3+ peers.
	if peer.Version >= AddrProtocolVersion {
		peer.SendMessage(s.config.Magic, &MsgGetAddr{})
	}

	// Advertise our own address to v3+ peers (if we accept inbound).
	if peer.Version >= AddrProtocolVersion && s.listener != nil {
		if tcpAddr, ok := s.listener.Addr().(*net.TCPAddr); ok {
			ownAddr := NetAddress{
				IP:   tcpAddr.IP.String(),
				Port: uint16(tcpAddr.Port),
			}
			if !isPrivateIP(ownAddr.IP) {
				peer.SendMessage(s.config.Magic, &MsgAddr{Addresses: []NetAddress{ownAddr}})
			}
		}
	}

	// Mark the peer as good in the addr manager.
	if peer.ListenPort > 0 {
		peerHost, _, _ := net.SplitHostPort(peer.Addr)
		s.addrMgr.MarkGood(NetAddress{IP: peerHost, Port: peer.ListenPort})
	}
}

func (s *Server) handlePing(peer *Peer, msg Message) {
	ping := msg.(*MsgPing)
	peer.SendMessage(s.config.Magic, &MsgPong{Nonce: ping.Nonce})
}

func (s *Server) handlePong(peer *Peer, msg Message) {
	peer.UpdateActivity()
	// Track minimum ping latency for eviction protection.
	if !peer.PingSentAt.IsZero() {
		latency := time.Since(peer.PingSentAt)
		if peer.MinPingLatency == 0 || latency < peer.MinPingLatency {
			peer.MinPingLatency = latency
		}
	}
}

func (s *Server) handleAddr(peer *Peer, msg Message) {
	addrMsg := msg.(*MsgAddr)
	if len(addrMsg.Addresses) > MaxAddrCount {
		s.protection.AddScore(peer.Addr, 20)
		return
	}

	filtered := s.filterAddresses(addrMsg.Addresses)
	if len(filtered) == 0 {
		return
	}

	// Add to addr manager with source tracking.
	peerHost, _, _ := net.SplitHostPort(peer.Addr)
	source := net.ParseIP(peerHost)
	if source == nil {
		source = net.IPv4zero
	}
	s.addrMgr.AddAddresses(filtered, source)

	// Relay: only forward small addr messages (likely fresh announcements,
	// not getaddr responses) to 1-2 random v3 peers.
	if len(addrMsg.Addresses) <= 10 {
		s.relayAddr(peer.Addr, filtered)
	}
}

func (s *Server) handleGetAddr(peer *Peer, msg Message) {
	// Once-per-peer: only respond to the first getaddr from each peer.
	if peer.GetAddrReceived {
		return
	}
	peer.GetAddrReceived = true

	addrs := s.addrMgr.GetAddresses()
	if len(addrs) > 0 {
		peer.SendMessage(s.config.Magic, &MsgAddr{Addresses: addrs})
	}
}

// relayAddr forwards addresses to 1-2 random v3 peers (not the sender).
func (s *Server) relayAddr(senderAddr string, addrs []NetAddress) {
	var candidates []*Peer
	for _, p := range s.peers.All() {
		if p.Handshaked && p.Addr != senderAddr && p.Version >= AddrProtocolVersion {
			candidates = append(candidates, p)
		}
	}
	if len(candidates) == 0 {
		return
	}

	// Pick 1-2 random targets.
	n := 2
	if len(candidates) < n {
		n = len(candidates)
	}
	rand.Shuffle(len(candidates), func(i, j int) {
		candidates[i], candidates[j] = candidates[j], candidates[i]
	})
	msg := &MsgAddr{Addresses: addrs}
	for _, p := range candidates[:n] {
		p.SendMessage(s.config.Magic, msg)
	}
}

// selectEvictionCandidate picks the worst inbound peer to evict.
// Returns the peer address to evict, or empty string if no peer can be evicted.
// Protects valuable peers: recent block senders, low-latency, recent tx senders, longest uptime.
func (s *Server) selectEvictionCandidate() string {
	var candidates []*Peer
	for _, p := range s.peers.All() {
		if p.Inbound && p.Handshaked {
			candidates = append(candidates, p)
		}
	}
	if len(candidates) == 0 {
		return ""
	}

	// Build protected set.
	protected := make(map[string]bool)

	// Protect top 4 by most recent valid block.
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].LastBlockTime.After(candidates[j].LastBlockTime)
	})
	for i := 0; i < 4 && i < len(candidates); i++ {
		if !candidates[i].LastBlockTime.IsZero() {
			protected[candidates[i].Addr] = true
		}
	}

	// Protect top 4 by lowest ping latency.
	sort.Slice(candidates, func(i, j int) bool {
		li, lj := candidates[i].MinPingLatency, candidates[j].MinPingLatency
		if li == 0 {
			return false
		}
		if lj == 0 {
			return true
		}
		return li < lj
	})
	for i := 0; i < 4 && i < len(candidates); i++ {
		if candidates[i].MinPingLatency > 0 {
			protected[candidates[i].Addr] = true
		}
	}

	// Protect top 4 by most recent valid transaction.
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].LastTxTime.After(candidates[j].LastTxTime)
	})
	for i := 0; i < 4 && i < len(candidates); i++ {
		if !candidates[i].LastTxTime.IsZero() {
			protected[candidates[i].Addr] = true
		}
	}

	// Protect top 4 by longest uptime (oldest ConnectedAt).
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].ConnectedAt.Before(candidates[j].ConnectedAt)
	})
	for i := 0; i < 4 && i < len(candidates); i++ {
		protected[candidates[i].Addr] = true
	}

	// From unprotected candidates, group by /16 subnet.
	// Pick the subnet with the most peers, then evict the one with oldest LastActive.
	type subnetGroup struct {
		subnet string
		peers  []*Peer
	}
	subnets := make(map[string]*subnetGroup)
	for _, p := range candidates {
		if protected[p.Addr] {
			continue
		}
		host, _, _ := net.SplitHostPort(p.Addr)
		subnet := addrGroup(host)
		sg, ok := subnets[subnet]
		if !ok {
			sg = &subnetGroup{subnet: subnet}
			subnets[subnet] = sg
		}
		sg.peers = append(sg.peers, p)
	}

	if len(subnets) == 0 {
		return "" // all peers are protected
	}

	// Find the subnet with the most peers.
	var worst *subnetGroup
	for _, sg := range subnets {
		if worst == nil || len(sg.peers) > len(worst.peers) {
			worst = sg
		}
	}

	// From that subnet, evict the peer with the oldest LastActive.
	var victim *Peer
	for _, p := range worst.peers {
		if victim == nil || p.LastActive.Before(victim.LastActive) {
			victim = p
		}
	}
	if victim == nil {
		return ""
	}
	return victim.Addr
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

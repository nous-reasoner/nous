package network

import (
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

// ServerConfig holds configuration for a Server.
type ServerConfig struct {
	ListenAddr string   // e.g. ":9333"
	Magic      uint32   // network magic bytes
	Seeds      []string // seed node addresses (e.g. "seed1.nous.io:9333")
	MaxPeers   int      // override max connections (0 = default)
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
	config   ServerConfig
	listener net.Listener
	peers    *PeerManager
	addrBook *AddressBook
	mempool  *Mempool
	handlers map[string]MessageHandler

	blockHeight uint64 // our current block height

	quit chan struct{}
	wg   sync.WaitGroup
	mu   sync.RWMutex
}

// NewServer creates a new P2P server.
func NewServer(config ServerConfig) *Server {
	return &Server{
		config:   config,
		peers:    NewPeerManager(),
		addrBook: NewAddressBook(config.Seeds),
		mempool:  NewMempool(),
		handlers: make(map[string]MessageHandler),
		quit:     make(chan struct{}),
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

	return nil
}

// Stop gracefully shuts down the server.
func (s *Server) Stop() error {
	close(s.quit)
	if s.listener != nil {
		s.listener.Close()
	}
	// Close all peers.
	for _, p := range s.peers.All() {
		p.Close()
	}
	s.wg.Wait()
	return nil
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

func (s *Server) handlePeer(peer *Peer) {
	defer s.wg.Done()
	defer s.peers.Remove(peer.Addr)

	for {
		select {
		case <-s.quit:
			return
		default:
		}

		// Set read deadline to detect dead connections.
		if peer.Conn != nil {
			peer.Conn.SetReadDeadline(time.Now().Add(InactiveTimeout))
		}

		msg, err := DecodeMessage(peer.Conn, s.config.Magic)
		if err != nil {
			return
		}

		peer.UpdateActivity()

		// Dispatch to handler.
		cmd := msg.Command()
		if handler, ok := s.handlers[cmd]; ok {
			handler(peer, msg)
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
}

func (s *Server) sendVersion(peer *Peer) {
	msg := &MsgVersion{
		Version:     ProtocolVersion,
		BlockHeight: s.BlockHeight(),
		Timestamp:   uint64(time.Now().Unix()),
		Nonce:       uint64(time.Now().UnixNano()),
		UserAgent:   "nous/0.1.0",
		ListenPort:  DefaultPort,
	}
	peer.SendMessage(s.config.Magic, msg)
}

func (s *Server) handleVersion(peer *Peer, msg Message) {
	ver := msg.(*MsgVersion)
	peer.Version = ver.Version
	peer.BlockHeight = ver.BlockHeight

	// Send verack.
	peer.SendMessage(s.config.Magic, &MsgVerAck{})

	// If this is an inbound connection, send our version too.
	if peer.Inbound && !peer.Handshaked {
		s.sendVersion(peer)
	}
}

func (s *Server) handleVerAck(peer *Peer, msg Message) {
	peer.Handshaked = true
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
	s.addrBook.AddAddresses(addrMsg.Addresses)
}

package network

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"
	"time"
)

// Peer connection limits.
const (
	MaxOutbound    = 8
	MaxInbound     = 117
	MaxConnections = MaxOutbound + MaxInbound // 125

	// InactiveTimeout is the duration after which a peer is considered dead.
	InactiveTimeout = 5 * time.Minute

	// PingInterval is how often we send ping messages.
	PingInterval = 2 * time.Minute
)

// Peer represents a connected P2P node.
type Peer struct {
	Addr        string
	Conn        net.Conn
	Version     uint32
	BlockHeight uint64 // best known height (max of all signals) — use for peer selection
	Inbound     bool
	LastActive  time.Time
	Handshaked  bool

	// Height tracking (inspired by Bitcoin Core's synced_headers/synced_blocks).
	StartingHeight   uint64 // height reported in version handshake (never updated after)
	LastBlockHeight  uint64 // height of last complete block received from this peer
	LastHeaderHeight uint64 // height of last header received from this peer

	// Eviction protection metadata.
	ConnectedAt    time.Time     // when this peer connected
	LastBlockTime  time.Time     // last time peer sent a valid block
	LastTxTime     time.Time     // last time peer sent a valid transaction
	MinPingLatency time.Duration // minimum observed ping round-trip
	PingSentAt     time.Time     // when the last ping was sent (for latency calc)

	// Addr protocol state.
	GetAddrReceived bool // true after first getaddr response (once-per-peer)
	ListenPort      uint16 // peer's advertised listen port (from version msg)

	mu      sync.Mutex
	writeMu sync.Mutex // protects concurrent Conn.Write
	closed  bool
}

// NewPeer creates a new Peer from a connection.
func NewPeer(addr string, conn net.Conn, inbound bool) *Peer {
	return &Peer{
		Addr:        addr,
		Conn:        conn,
		Inbound:     inbound,
		LastActive:  time.Now(),
		ConnectedAt: time.Now(),
	}
}

// UpdateBestHeight updates the peer's best known height if the new value is higher.
// This is the single point for all height updates — BestKnownHeight only ever increases.
func (p *Peer) UpdateBestHeight(height uint64) {
	if height > p.BlockHeight {
		p.BlockHeight = height
	}
}

// UpdateActivity marks the peer as recently active.
func (p *Peer) UpdateActivity() {
	p.mu.Lock()
	p.LastActive = time.Now()
	p.mu.Unlock()
}

// IsActive returns true if the peer has been active within the timeout.
func (p *Peer) IsActive() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return time.Since(p.LastActive) < InactiveTimeout
}

// Close closes the peer connection.
func (p *Peer) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	if p.Conn != nil {
		return p.Conn.Close()
	}
	return nil
}

// SendMessage encodes and writes a message to the peer.
func (p *Peer) SendMessage(magic uint32, msg Message) error {
	data, err := EncodeMessage(magic, msg)
	if err != nil {
		return err
	}
	p.writeMu.Lock()
	_, err = p.Conn.Write(data)
	p.writeMu.Unlock()
	return err
}

// PeerManager tracks all connected peers.
type PeerManager struct {
	mu    sync.RWMutex
	peers map[string]*Peer // keyed by addr
}

// NewPeerManager creates an empty peer manager.
func NewPeerManager() *PeerManager {
	return &PeerManager{
		peers: make(map[string]*Peer),
	}
}

// Add registers a new peer. Returns false if the connection limit is reached.
func (pm *PeerManager) Add(p *Peer) bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if len(pm.peers) >= MaxConnections {
		return false
	}

	// Check per-direction limits.
	inbound, outbound := pm.countDirections()
	if p.Inbound && inbound >= MaxInbound {
		return false
	}
	if !p.Inbound && outbound >= MaxOutbound {
		return false
	}

	pm.peers[p.Addr] = p
	return true
}

// Remove disconnects and removes a peer.
func (pm *PeerManager) Remove(addr string) {
	pm.mu.Lock()
	p, ok := pm.peers[addr]
	if ok {
		delete(pm.peers, addr)
	}
	pm.mu.Unlock()

	if ok {
		p.Close()
	}
}

// Get returns a peer by address, or nil.
func (pm *PeerManager) Get(addr string) *Peer {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.peers[addr]
}

// Count returns the number of connected peers.
func (pm *PeerManager) Count() int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return len(pm.peers)
}

// All returns a snapshot of all peers.
func (pm *PeerManager) All() []*Peer {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	result := make([]*Peer, 0, len(pm.peers))
	for _, p := range pm.peers {
		result = append(result, p)
	}
	return result
}

// BestPeer returns the peer with the highest block height.
func (pm *PeerManager) BestPeer() *Peer {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var best *Peer
	for _, p := range pm.peers {
		if !p.Handshaked {
			continue
		}
		if best == nil || p.BlockHeight > best.BlockHeight {
			best = p
		}
	}
	return best
}

// RemoveInactive disconnects all peers that have been inactive.
func (pm *PeerManager) RemoveInactive() []string {
	pm.mu.Lock()
	var toRemove []string
	for addr, p := range pm.peers {
		if !p.IsActive() {
			toRemove = append(toRemove, addr)
		}
	}
	for _, addr := range toRemove {
		if p, ok := pm.peers[addr]; ok {
			p.Close()
			delete(pm.peers, addr)
		}
	}
	pm.mu.Unlock()
	return toRemove
}

func (pm *PeerManager) countDirections() (inbound, outbound int) {
	for _, p := range pm.peers {
		if p.Inbound {
			inbound++
		} else {
			outbound++
		}
	}
	return
}

// addrEntry tracks a known address with metadata for persistence.
type addrEntry struct {
	Addr     NetAddress `json:"addr"`
	LastSeen time.Time  `json:"last_seen"`
	Failures int        `json:"failures"`
}

// AddressBook maintains a list of known peer addresses for discovery.
type AddressBook struct {
	mu        sync.RWMutex
	addresses map[string]*addrEntry // keyed by "IP:Port"
	seeds     []string             // seed node addresses
}

// NewAddressBook creates an address book with the given seed nodes.
func NewAddressBook(seeds []string) *AddressBook {
	return &AddressBook{
		addresses: make(map[string]*addrEntry),
		seeds:     seeds,
	}
}

// AddAddress adds a known address.
func (ab *AddressBook) AddAddress(addr NetAddress) {
	ab.mu.Lock()
	defer ab.mu.Unlock()
	key := net.JoinHostPort(addr.IP, fmt.Sprintf("%d", addr.Port))
	if e, ok := ab.addresses[key]; ok {
		e.LastSeen = time.Now()
		e.Failures = 0 // reset on re-advertisement
	} else {
		ab.addresses[key] = &addrEntry{Addr: addr, LastSeen: time.Now()}
	}
}

// AddAddresses adds multiple addresses at once.
func (ab *AddressBook) AddAddresses(addrs []NetAddress) {
	ab.mu.Lock()
	defer ab.mu.Unlock()
	for _, addr := range addrs {
		key := net.JoinHostPort(addr.IP, fmt.Sprintf("%d", addr.Port))
		if e, ok := ab.addresses[key]; ok {
			e.LastSeen = time.Now()
			e.Failures = 0
		} else {
			ab.addresses[key] = &addrEntry{Addr: addr, LastSeen: time.Now()}
		}
	}
}

// GetAddresses returns up to n known addresses.
func (ab *AddressBook) GetAddresses(n int) []NetAddress {
	ab.mu.RLock()
	defer ab.mu.RUnlock()
	result := make([]NetAddress, 0, n)
	for _, e := range ab.addresses {
		result = append(result, e.Addr)
		if len(result) >= n {
			break
		}
	}
	return result
}

// GetGoodAddresses returns up to n addresses with fewer than maxFailures.
func (ab *AddressBook) GetGoodAddresses(n, maxFailures int) []NetAddress {
	ab.mu.RLock()
	defer ab.mu.RUnlock()
	result := make([]NetAddress, 0, n)
	for _, e := range ab.addresses {
		if e.Failures >= maxFailures {
			continue
		}
		result = append(result, e.Addr)
		if len(result) >= n {
			break
		}
	}
	return result
}

// RecordFailure increments the failure count for an address.
// Returns true if the address was removed (failures >= maxFailures).
func (ab *AddressBook) RecordFailure(addr string, maxFailures int) bool {
	ab.mu.Lock()
	defer ab.mu.Unlock()
	e, ok := ab.addresses[addr]
	if !ok {
		return false
	}
	e.Failures++
	if e.Failures >= maxFailures {
		delete(ab.addresses, addr)
		return true
	}
	return false
}

// RemoveStale removes entries not seen within the given duration.
func (ab *AddressBook) RemoveStale(maxAge time.Duration) int {
	ab.mu.Lock()
	defer ab.mu.Unlock()
	cutoff := time.Now().Add(-maxAge)
	removed := 0
	for key, e := range ab.addresses {
		if e.LastSeen.Before(cutoff) {
			delete(ab.addresses, key)
			removed++
		}
	}
	return removed
}

// Seeds returns the configured seed node addresses.
func (ab *AddressBook) Seeds() []string {
	return ab.seeds
}

// Count returns the number of known addresses.
func (ab *AddressBook) Count() int {
	ab.mu.RLock()
	defer ab.mu.RUnlock()
	return len(ab.addresses)
}

// addrBookJSON is the JSON-serializable form of the address book.
type addrBookJSON struct {
	Addresses []addrEntry `json:"addresses"`
}

// SaveToFile writes the address book to a JSON file.
func (ab *AddressBook) SaveToFile(path string) error {
	ab.mu.RLock()
	entries := make([]addrEntry, 0, len(ab.addresses))
	for _, e := range ab.addresses {
		entries = append(entries, *e)
	}
	ab.mu.RUnlock()

	data, err := json.MarshalIndent(addrBookJSON{Addresses: entries}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// LoadAddressBook reads an address book from a JSON file.
func LoadAddressBook(path string, seeds []string) (*AddressBook, error) {
	ab := NewAddressBook(seeds)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ab, nil // no saved state
		}
		return nil, err
	}
	var stored addrBookJSON
	if err := json.Unmarshal(data, &stored); err != nil {
		return ab, nil // corrupted file, start fresh
	}
	for _, e := range stored.Addresses {
		eCopy := e
		key := net.JoinHostPort(e.Addr.IP, fmt.Sprintf("%d", e.Addr.Port))
		ab.addresses[key] = &eCopy
	}
	return ab, nil
}

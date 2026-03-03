package network

import (
	"log"
	"sync"
	"time"
)

// Ban score penalties.
const (
	BanScoreInvalidBlock = 100 // consensus-invalid block
	BanScoreConsensusTx  = 25  // consensus-invalid transaction
	BanScorePolicyTx     = 5   // policy-invalid transaction
	BanScoreRateExceeded = 50  // message rate limit exceeded
	BanScoreUnknownCmd   = 10  // unknown message command
	BanScoreBadMessage   = 20  // malformed / decode error
	BanScoreOversized    = 20  // oversized message

	BanScoreThreshold = 100 // cumulative score that triggers a ban
	BanDuration       = 24 * time.Hour

	RateLimitPerSecond = 100
	RateLimitWindow    = time.Second

	HandshakeTimeout = 10 * time.Second
)

// banEntry records when a peer was banned.
type banEntry struct {
	BannedAt time.Time
	Score    int
}

// peerRateTracker tracks message rate for a single peer.
type peerRateTracker struct {
	count     int
	windowEnd time.Time
}

// PeerProtection manages ban scores and rate limiting for all peers.
type PeerProtection struct {
	mu     sync.Mutex
	scores map[string]int              // host → cumulative ban score
	banned map[string]*banEntry        // host → ban info
	rates  map[string]*peerRateTracker // host → rate tracker
}

// NewPeerProtection creates a new protection manager.
func NewPeerProtection() *PeerProtection {
	return &PeerProtection{
		scores: make(map[string]int),
		banned: make(map[string]*banEntry),
		rates:  make(map[string]*peerRateTracker),
	}
}

// IsBanned returns true if the peer is currently banned.
func (pp *PeerProtection) IsBanned(addr string) bool {
	pp.mu.Lock()
	defer pp.mu.Unlock()
	host := hostOnly(addr)
	entry, ok := pp.banned[host]
	if !ok {
		return false
	}
	if time.Since(entry.BannedAt) >= BanDuration {
		delete(pp.banned, host)
		delete(pp.scores, host)
		return false
	}
	return true
}

// AddScore adds penalty points to a peer. If the score reaches the threshold,
// the peer is banned. Returns true if the peer is now banned.
func (pp *PeerProtection) AddScore(addr string, score int) bool {
	pp.mu.Lock()
	defer pp.mu.Unlock()
	host := hostOnly(addr)
	pp.scores[host] += score
	if pp.scores[host] >= BanScoreThreshold {
		pp.banned[host] = &banEntry{BannedAt: time.Now(), Score: pp.scores[host]}
		log.Printf("protection: banned %s (score %d)", host, pp.scores[host])
		return true
	}
	return false
}

// GetScore returns the current ban score for a peer.
func (pp *PeerProtection) GetScore(addr string) int {
	pp.mu.Lock()
	defer pp.mu.Unlock()
	return pp.scores[hostOnly(addr)]
}

// CheckRate checks if a peer has exceeded the rate limit.
// Returns true if the message is allowed, false if rate exceeded.
func (pp *PeerProtection) CheckRate(addr string) bool {
	pp.mu.Lock()
	defer pp.mu.Unlock()
	host := hostOnly(addr)
	now := time.Now()
	rt, ok := pp.rates[host]
	if !ok {
		pp.rates[host] = &peerRateTracker{count: 1, windowEnd: now.Add(RateLimitWindow)}
		return true
	}
	if now.After(rt.windowEnd) {
		// New window.
		rt.count = 1
		rt.windowEnd = now.Add(RateLimitWindow)
		return true
	}
	rt.count++
	return rt.count <= RateLimitPerSecond
}

// RemovePeer cleans up rate tracking state for a disconnected peer.
func (pp *PeerProtection) RemovePeer(addr string) {
	pp.mu.Lock()
	defer pp.mu.Unlock()
	host := hostOnly(addr)
	delete(pp.rates, host)
	// Keep scores and bans — they persist until expiry.
}

// BanCount returns the number of currently banned peers.
func (pp *PeerProtection) BanCount() int {
	pp.mu.Lock()
	defer pp.mu.Unlock()
	return len(pp.banned)
}

// hostOnly strips the port from an address to ban by IP, not IP:port.
// This prevents attackers from reconnecting on a different port.
func hostOnly(addr string) string {
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return addr[:i]
		}
	}
	return addr
}

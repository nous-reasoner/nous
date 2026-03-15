package network

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	mrand "math/rand"
	"net"
	"os"
	"sync"
	"time"
)

// AddrManager bucket constants.
const (
	NewBucketCount   = 1024
	TriedBucketCount = 256
	BucketSize       = 64
	MaxAddrPerSource = 100

	// getAddressesPct is the percentage of known addresses to return on getaddr.
	getAddressesPct = 23

	// maxNewAddrAge is the maximum age of an address before it is considered terrible.
	maxNewAddrAge = 30 * 24 * time.Hour

	// maxAddrAttempts is the max consecutive failures before an address is terrible.
	maxAddrAttempts = 3
)

// KnownAddress is a peer address tracked by the AddrManager with metadata.
type KnownAddress struct {
	Addr        NetAddress
	Source      string    // source IP that told us about this address
	LastSeen    time.Time // last time we heard about this address
	LastAttempt time.Time // last connection attempt
	LastSuccess time.Time // last successful connection
	Attempts    int       // consecutive failure count
	InTried     bool      // whether this address is in a tried bucket

	bucket int // bucket index
	pos    int // position within bucket
}

// isTerrible returns true if the address should be replaceable in its bucket.
func (ka *KnownAddress) isTerrible() bool {
	if time.Since(ka.LastSeen) > maxNewAddrAge {
		return true
	}
	if ka.Attempts >= maxAddrAttempts && ka.LastSuccess.IsZero() {
		return true
	}
	return false
}

// AddrManager implements Bitcoin-style new/tried dual-bucket address management
// with SHA256-based bucket hashing for anti-eclipse attack protection.
type AddrManager struct {
	mu           sync.RWMutex
	addrIndex    map[string]*KnownAddress              // "IP:Port" → KnownAddress
	newBuckets   [NewBucketCount][BucketSize]string     // key or ""
	triedBuckets [TriedBucketCount][BucketSize]string   // key or ""
	nNew         int
	nTried       int
	key          [32]byte // random key for anti-eclipse bucket hashing
	filePath     string
	sourceCount  map[string]int // sourceIP → count of addresses from this source
	seeds        []string
	rng          *mrand.Rand
}

// NewAddrManager creates a new address manager with a random anti-eclipse key.
func NewAddrManager(filePath string, seeds []string) *AddrManager {
	am := &AddrManager{
		addrIndex:   make(map[string]*KnownAddress),
		filePath:    filePath,
		sourceCount: make(map[string]int),
		seeds:       seeds,
		rng:         mrand.New(mrand.NewSource(time.Now().UnixNano())),
	}
	rand.Read(am.key[:])
	return am
}

// addrKey returns the map key for a NetAddress.
func addrKey(addr NetAddress) string {
	return net.JoinHostPort(addr.IP, fmt.Sprintf("%d", addr.Port))
}

// addrGroup returns the /16 subnet grouping for an IP (e.g. "89.168").
func addrGroup(ip string) string {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return ip
	}
	if ip4 := parsed.To4(); ip4 != nil {
		return fmt.Sprintf("%d.%d", ip4[0], ip4[1])
	}
	// IPv6: use first 32 bits.
	return fmt.Sprintf("%02x%02x", parsed[0:2], parsed[2:4])
}

// newBucketIdx computes which new bucket an address belongs to.
// bucket = SHA256(key + sourceGroup + SHA256(key + addr)) % NewBucketCount
func (am *AddrManager) newBucketIdx(addr, source string) int {
	srcGroup := addrGroup(source)

	h1 := sha256.Sum256(append(am.key[:], []byte(addr)...))

	var buf []byte
	buf = append(buf, am.key[:]...)
	buf = append(buf, []byte(srcGroup)...)
	buf = append(buf, h1[:]...)
	h2 := sha256.Sum256(buf)

	return int(binary.LittleEndian.Uint64(h2[:8]) % NewBucketCount)
}

// bucketPos computes the position within a bucket for an address.
// pos = SHA256(key + addr) % BucketSize
func (am *AddrManager) bucketPos(addr string) int {
	h := sha256.Sum256(append(am.key[:], []byte(addr)...))
	return int(binary.LittleEndian.Uint64(h[:8]) % BucketSize)
}

// triedBucketIdx computes which tried bucket an address belongs to.
// bucket = SHA256(key + addrGroup + SHA256(key + addr)) % TriedBucketCount
func (am *AddrManager) triedBucketIdx(addr string) int {
	ip, _, _ := net.SplitHostPort(addr)
	group := addrGroup(ip)

	h1 := sha256.Sum256(append(am.key[:], []byte(addr)...))

	var buf []byte
	buf = append(buf, am.key[:]...)
	buf = append(buf, []byte(group)...)
	buf = append(buf, h1[:]...)
	h2 := sha256.Sum256(buf)

	return int(binary.LittleEndian.Uint64(h2[:8]) % TriedBucketCount)
}

// AddAddresses adds peer addresses received from a source peer.
func (am *AddrManager) AddAddresses(addrs []NetAddress, source net.IP) {
	am.mu.Lock()
	defer am.mu.Unlock()

	sourceStr := source.String()

	for _, addr := range addrs {
		if isPrivateIP(addr.IP) {
			continue
		}

		key := addrKey(addr)

		// Already known: just refresh last-seen.
		if ka, exists := am.addrIndex[key]; exists {
			ka.LastSeen = time.Now()
			continue
		}

		// Enforce per-source limit.
		if am.sourceCount[sourceStr] >= MaxAddrPerSource {
			continue
		}

		bucket := am.newBucketIdx(key, sourceStr)
		pos := am.bucketPos(key)

		// If slot is occupied, only replace if existing entry is terrible.
		if existing := am.newBuckets[bucket][pos]; existing != "" {
			if ek, ok := am.addrIndex[existing]; ok {
				if !ek.isTerrible() {
					continue
				}
				delete(am.addrIndex, existing)
				am.nNew--
			}
			am.newBuckets[bucket][pos] = ""
		}

		ka := &KnownAddress{
			Addr:     addr,
			Source:   sourceStr,
			LastSeen: time.Now(),
			bucket:   bucket,
			pos:      pos,
		}
		am.addrIndex[key] = ka
		am.newBuckets[bucket][pos] = key
		am.nNew++
		am.sourceCount[sourceStr]++
	}
}

// MarkGood marks an address as successfully connected,
// moving it from a new bucket to a tried bucket.
func (am *AddrManager) MarkGood(addr NetAddress) {
	am.mu.Lock()
	defer am.mu.Unlock()

	key := addrKey(addr)
	ka, ok := am.addrIndex[key]
	if !ok {
		return
	}

	ka.LastSuccess = time.Now()
	ka.LastSeen = time.Now()
	ka.Attempts = 0

	if ka.InTried {
		return
	}

	// Remove from new bucket.
	am.newBuckets[ka.bucket][ka.pos] = ""
	am.nNew--

	// Find tried bucket slot.
	bucket := am.triedBucketIdx(key)
	pos := am.bucketPos(key)

	// If tried slot is occupied, evict existing entry back to new.
	if existing := am.triedBuckets[bucket][pos]; existing != "" {
		if ek, ok := am.addrIndex[existing]; ok {
			ek.InTried = false
			am.nTried--

			// Find a new bucket for the evicted entry.
			newBucket := am.newBucketIdx(existing, ek.Source)
			newPos := am.bucketPos(existing)
			if am.newBuckets[newBucket][newPos] == "" {
				ek.bucket = newBucket
				ek.pos = newPos
				am.newBuckets[newBucket][newPos] = existing
				am.nNew++
			} else {
				// Both slots occupied — drop the evicted entry.
				delete(am.addrIndex, existing)
			}
		}
	}

	ka.InTried = true
	ka.bucket = bucket
	ka.pos = pos
	am.triedBuckets[bucket][pos] = key
	am.nTried++
}

// MarkAttempt records a connection attempt for an address.
func (am *AddrManager) MarkAttempt(addr NetAddress) {
	am.mu.Lock()
	defer am.mu.Unlock()

	if ka, ok := am.addrIndex[addrKey(addr)]; ok {
		ka.LastAttempt = time.Now()
		ka.Attempts++
	}
}

// MarkFailed records a connection failure. If the address has too many
// consecutive failures, it is removed.
func (am *AddrManager) MarkFailed(addr NetAddress) {
	am.mu.Lock()
	defer am.mu.Unlock()

	key := addrKey(addr)
	ka, ok := am.addrIndex[key]
	if !ok {
		return
	}

	ka.Attempts++
	if ka.Attempts >= maxAddrAttempts && ka.LastSuccess.IsZero() {
		// Remove terrible address.
		if ka.InTried {
			am.triedBuckets[ka.bucket][ka.pos] = ""
			am.nTried--
		} else {
			am.newBuckets[ka.bucket][ka.pos] = ""
			am.nNew--
		}
		delete(am.addrIndex, key)
	}
}

// SelectForOutbound picks a random address for an outbound connection attempt.
// Alternates between tried and new buckets (50/50).
func (am *AddrManager) SelectForOutbound() *NetAddress {
	am.mu.RLock()
	defer am.mu.RUnlock()

	if am.nNew == 0 && am.nTried == 0 {
		return nil
	}

	// Collect eligible candidates.
	var candidates []*KnownAddress
	for _, ka := range am.addrIndex {
		if !ka.LastAttempt.IsZero() && time.Since(ka.LastAttempt) < 10*time.Minute {
			continue
		}
		candidates = append(candidates, ka)
	}
	if len(candidates) == 0 {
		return nil
	}

	// Prefer tried addresses 50% of the time.
	var tried, fresh []*KnownAddress
	for _, ka := range candidates {
		if ka.InTried {
			tried = append(tried, ka)
		} else {
			fresh = append(fresh, ka)
		}
	}

	var pick *KnownAddress
	useTried := am.rng.Intn(2) == 0
	if useTried && len(tried) > 0 {
		pick = tried[am.rng.Intn(len(tried))]
	} else if len(fresh) > 0 {
		pick = fresh[am.rng.Intn(len(fresh))]
	} else if len(tried) > 0 {
		pick = tried[am.rng.Intn(len(tried))]
	}

	if pick == nil {
		return nil
	}
	addr := pick.Addr
	return &addr
}

// GetAddresses returns a random sample of known addresses for a getaddr response.
// Returns ~23% of all known addresses, up to MaxAddrCount.
func (am *AddrManager) GetAddresses() []NetAddress {
	am.mu.RLock()
	defer am.mu.RUnlock()

	total := len(am.addrIndex)
	if total == 0 {
		return nil
	}

	numAddrs := total * getAddressesPct / 100
	if numAddrs > MaxAddrCount {
		numAddrs = MaxAddrCount
	}
	if numAddrs < 1 {
		numAddrs = 1
	}

	all := make([]NetAddress, 0, total)
	for _, ka := range am.addrIndex {
		all = append(all, ka.Addr)
	}
	am.rng.Shuffle(len(all), func(i, j int) {
		all[i], all[j] = all[j], all[i]
	})

	if numAddrs > len(all) {
		numAddrs = len(all)
	}
	return all[:numAddrs]
}

// Count returns the number of addresses in new and tried buckets.
func (am *AddrManager) Count() (nNew, nTried int) {
	am.mu.RLock()
	defer am.mu.RUnlock()
	return am.nNew, am.nTried
}

// Total returns the total number of known addresses.
func (am *AddrManager) Total() int {
	am.mu.RLock()
	defer am.mu.RUnlock()
	return len(am.addrIndex)
}

// Seeds returns the configured seed node addresses.
func (am *AddrManager) Seeds() []string {
	return am.seeds
}

// --- Persistence ---

type persistedAddr struct {
	IP          string `json:"ip"`
	Port        uint16 `json:"port"`
	Source      string `json:"source"`
	LastSeen    int64  `json:"last_seen"`
	LastAttempt int64  `json:"last_attempt"`
	LastSuccess int64  `json:"last_success"`
	Attempts    int    `json:"attempts"`
	InTried     bool   `json:"in_tried"`
	Bucket      int    `json:"bucket"`
	Pos         int    `json:"pos"`
}

type persistedAddrFile struct {
	Version int             `json:"version"`
	Key     string          `json:"key"`
	Saved   int64           `json:"last_saved"`
	Addrs   []persistedAddr `json:"addresses"`
}

// SaveToFile persists the AddrManager state to disk.
func (am *AddrManager) SaveToFile() error {
	if am.filePath == "" {
		return nil
	}

	am.mu.RLock()
	pf := persistedAddrFile{
		Version: 1,
		Key:     hex.EncodeToString(am.key[:]),
		Saved:   time.Now().Unix(),
	}
	for _, ka := range am.addrIndex {
		pa := persistedAddr{
			IP:          ka.Addr.IP,
			Port:        ka.Addr.Port,
			Source:      ka.Source,
			LastSeen:    ka.LastSeen.Unix(),
			LastAttempt: ka.LastAttempt.Unix(),
			LastSuccess: ka.LastSuccess.Unix(),
			Attempts:    ka.Attempts,
			InTried:     ka.InTried,
			Bucket:      ka.bucket,
			Pos:         ka.pos,
		}
		pf.Addrs = append(pf.Addrs, pa)
	}
	am.mu.RUnlock()

	data, err := json.MarshalIndent(pf, "", "  ")
	if err != nil {
		return err
	}

	tmp := am.filePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, am.filePath)
}

// LoadFromFile loads persisted state from disk. Handles both the new format
// and the legacy AddressBook format (automatic migration).
func (am *AddrManager) LoadFromFile() error {
	if am.filePath == "" {
		return nil
	}

	data, err := os.ReadFile(am.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	// Try new format.
	var pf persistedAddrFile
	if err := json.Unmarshal(data, &pf); err != nil {
		return am.loadLegacy(data)
	}
	if pf.Version == 0 {
		return am.loadLegacy(data)
	}

	am.mu.Lock()
	defer am.mu.Unlock()

	// Restore key.
	if keyBytes, err := hex.DecodeString(pf.Key); err == nil && len(keyBytes) == 32 {
		copy(am.key[:], keyBytes)
	}

	for _, pa := range pf.Addrs {
		key := net.JoinHostPort(pa.IP, fmt.Sprintf("%d", pa.Port))

		ka := &KnownAddress{
			Addr:        NetAddress{IP: pa.IP, Port: pa.Port},
			Source:      pa.Source,
			LastSeen:    time.Unix(pa.LastSeen, 0),
			LastAttempt: time.Unix(pa.LastAttempt, 0),
			LastSuccess: time.Unix(pa.LastSuccess, 0),
			Attempts:    pa.Attempts,
			InTried:     pa.InTried,
			bucket:      pa.Bucket,
			pos:         pa.Pos,
		}

		// Validate bucket/pos ranges.
		if pa.InTried {
			if pa.Bucket >= TriedBucketCount || pa.Pos >= BucketSize {
				continue
			}
			am.triedBuckets[pa.Bucket][pa.Pos] = key
			am.nTried++
		} else {
			if pa.Bucket >= NewBucketCount || pa.Pos >= BucketSize {
				continue
			}
			am.newBuckets[pa.Bucket][pa.Pos] = key
			am.nNew++
		}

		am.addrIndex[key] = ka
		if pa.Source != "" {
			am.sourceCount[pa.Source]++
		}
	}

	return nil
}

// loadLegacy migrates from the old AddressBook JSON format.
func (am *AddrManager) loadLegacy(data []byte) error {
	var legacy struct {
		Addresses []struct {
			Addr struct {
				IP   string `json:"IP"`
				Port uint16 `json:"Port"`
			} `json:"addr"`
			LastSeen time.Time `json:"last_seen"`
		} `json:"addresses"`
	}

	if err := json.Unmarshal(data, &legacy); err != nil {
		return nil // corrupted, start fresh
	}

	am.mu.Lock()
	defer am.mu.Unlock()

	// Generate new key (legacy format didn't have one).
	rand.Read(am.key[:])

	for _, entry := range legacy.Addresses {
		addr := NetAddress{IP: entry.Addr.IP, Port: entry.Addr.Port}
		am.addLocked(addr, "legacy")
	}

	if len(legacy.Addresses) > 0 {
		log.Printf("network: migrated %d addresses from legacy peers.json", len(legacy.Addresses))
	}
	return nil
}

// addLocked adds an address to a new bucket while holding the lock.
func (am *AddrManager) addLocked(addr NetAddress, source string) {
	if isPrivateIP(addr.IP) {
		return
	}

	key := addrKey(addr)
	if _, exists := am.addrIndex[key]; exists {
		return
	}

	bucket := am.newBucketIdx(key, source)
	pos := am.bucketPos(key)

	if am.newBuckets[bucket][pos] != "" {
		return // slot occupied, skip
	}

	ka := &KnownAddress{
		Addr:     addr,
		Source:   source,
		LastSeen: time.Now(),
		bucket:   bucket,
		pos:      pos,
	}
	am.addrIndex[key] = ka
	am.newBuckets[bucket][pos] = key
	am.nNew++
}

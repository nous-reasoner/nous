package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
)

// DeriveInt computes HMAC-SHA256(seed, label) and returns the first 8 bytes as uint64.
// This is the core deterministic derivation primitive used throughout CSP generation.
func DeriveInt(seed Hash, label string) uint64 {
	mac := hmac.New(sha256.New, seed[:])
	mac.Write([]byte(label))
	result := mac.Sum(nil)
	return binary.BigEndian.Uint64(result[:8])
}

// DeriveInts derives count uint64 values using sequential sub-labels: label+"0", label+"1", ...
func DeriveInts(seed Hash, label string, count int) []uint64 {
	result := make([]uint64, count)
	for i := range result {
		result[i] = DeriveInt(seed, fmt.Sprintf("%s%d", label, i))
	}
	return result
}

// DeriveSubset selects count distinct indices from [0, n) using a deterministic
// Fisher-Yates shuffle seeded by HMAC-SHA256.
func DeriveSubset(seed Hash, label string, n int, count int) []int {
	if count > n {
		count = n
	}
	// Build pool of available indices.
	pool := make([]int, n)
	for i := range pool {
		pool[i] = i
	}
	selected := make([]int, count)
	for i := 0; i < count; i++ {
		remaining := len(pool) - i
		idx := DeriveInt(seed, fmt.Sprintf("%s%d", label, i)) % uint64(remaining)
		pos := int(idx) + i
		// Swap selected element to position i.
		pool[i], pool[pos] = pool[pos], pool[i]
		selected[i] = pool[i]
	}
	return selected
}

// DeriveHash computes HMAC-SHA256(seed, label) and returns the full 32-byte result as a Hash.
// Used to derive sub-seeds (e.g., sub-seeds from parent seed).
func DeriveHash(seed Hash, label string) Hash {
	mac := hmac.New(sha256.New, seed[:])
	mac.Write([]byte(label))
	return HashFromBytes(mac.Sum(nil))
}

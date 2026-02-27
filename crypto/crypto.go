// Package crypto provides cryptographic primitives for the NOUS blockchain.
//
// Responsibilities:
//   - Key pair generation (secp256k1)
//   - ECDSA signing and verification
//   - SHA-256 / double-SHA-256 hashing
//   - Address derivation (Base58Check)
package crypto

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"

	"golang.org/x/crypto/ripemd160"
)

// HashSize is the length of a SHA-256 hash in bytes.
const HashSize = 32

// Hash represents a 32-byte SHA-256 hash.
type Hash [HashSize]byte

// Sha256 computes the SHA-256 hash of data.
func Sha256(data []byte) Hash {
	return sha256.Sum256(data)
}

// DoubleSha256 computes SHA-256(SHA-256(data)), used for block and tx hashing.
func DoubleSha256(data []byte) Hash {
	first := sha256.Sum256(data)
	return sha256.Sum256(first[:])
}

// Bytes returns the hash as a byte slice.
func (h Hash) Bytes() []byte {
	return h[:]
}

// String returns the hash as a lowercase hex string.
func (h Hash) String() string {
	return hex.EncodeToString(h[:])
}

// IsZero returns true if the hash is all zeros.
func (h Hash) IsZero() bool {
	for _, b := range h {
		if b != 0 {
			return false
		}
	}
	return true
}

// Compare returns -1, 0, or 1 comparing h to other as big-endian unsigned integers.
// Used for PoW target comparison: h.Compare(target) <= 0 means h meets the target.
func (h Hash) Compare(other Hash) int {
	hInt := new(big.Int).SetBytes(h[:])
	oInt := new(big.Int).SetBytes(other[:])
	return hInt.Cmp(oInt)
}

// HashFromBytes creates a Hash from a byte slice. Panics if len(b) != HashSize.
func HashFromBytes(b []byte) Hash {
	if len(b) != HashSize {
		panic(fmt.Sprintf("crypto: HashFromBytes requires %d bytes, got %d", HashSize, len(b)))
	}
	var h Hash
	copy(h[:], b)
	return h
}

// Hash160 computes RIPEMD160(SHA256(data)), used for P2PKH address derivation.
func Hash160(data []byte) []byte {
	shaHash := sha256.Sum256(data)
	r := ripemd160.New()
	r.Write(shaHash[:])
	return r.Sum(nil)
}

// HashFromHex creates a Hash from a hex string. Returns an error on invalid input.
func HashFromHex(s string) (Hash, error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return Hash{}, fmt.Errorf("crypto: invalid hex: %w", err)
	}
	if len(b) != HashSize {
		return Hash{}, fmt.Errorf("crypto: hex decodes to %d bytes, need %d", len(b), HashSize)
	}
	return HashFromBytes(b), nil
}

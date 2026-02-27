package crypto

import (
	"crypto/sha256"
	"errors"
	"math/big"

	"golang.org/x/crypto/ripemd160"
)

// AddressVersion is the version byte prepended to addresses (0x00 = mainnet).
const AddressVersion = 0x00

// Address is a Base58Check-encoded public key hash.
type Address string

// base58Alphabet is the Bitcoin Base58 character set (no 0, O, I, l).
const base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

// PubKeyToAddress derives a NOUS address: Base58Check(version || RIPEMD160(SHA256(compressedPubKey))).
func PubKeyToAddress(pubKey *PublicKey) Address {
	pubBytes := pubKey.SerializeCompressed()

	// SHA-256 then RIPEMD-160.
	shaHash := sha256.Sum256(pubBytes)
	riper := ripemd160.New()
	riper.Write(shaHash[:])
	pubKeyHash := riper.Sum(nil) // 20 bytes

	// Base58Check: version + payload + checksum.
	return Address(base58CheckEncode(AddressVersion, pubKeyHash))
}

// AddressToPubKeyHash decodes a Base58Check address and returns the 20-byte public key hash.
func AddressToPubKeyHash(addr Address) ([]byte, error) {
	decoded, err := base58Decode(string(addr))
	if err != nil {
		return nil, err
	}
	if len(decoded) < 5 {
		return nil, errors.New("crypto: address too short")
	}
	// Verify checksum.
	payload := decoded[:len(decoded)-4]
	checksum := decoded[len(decoded)-4:]
	hash := DoubleSha256(payload)
	for i := 0; i < 4; i++ {
		if checksum[i] != hash[i] {
			return nil, errors.New("crypto: address checksum mismatch")
		}
	}
	if payload[0] != AddressVersion {
		return nil, errors.New("crypto: unknown address version")
	}
	return payload[1:], nil // 20-byte pubkey hash
}

// base58CheckEncode encodes version + payload with a 4-byte checksum.
func base58CheckEncode(version byte, payload []byte) string {
	versioned := make([]byte, 1+len(payload))
	versioned[0] = version
	copy(versioned[1:], payload)

	checksum := DoubleSha256(versioned)
	full := append(versioned, checksum[:4]...)
	return base58Encode(full)
}

// base58Encode encodes a byte slice to a Base58 string.
func base58Encode(input []byte) string {
	x := new(big.Int).SetBytes(input)
	base := big.NewInt(58)
	zero := big.NewInt(0)
	mod := new(big.Int)

	var result []byte
	for x.Cmp(zero) > 0 {
		x.DivMod(x, base, mod)
		result = append(result, base58Alphabet[mod.Int64()])
	}

	// Preserve leading zeros.
	for _, b := range input {
		if b != 0 {
			break
		}
		result = append(result, base58Alphabet[0])
	}

	// Reverse.
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return string(result)
}

// base58Decode decodes a Base58 string to a byte slice.
func base58Decode(s string) ([]byte, error) {
	result := big.NewInt(0)
	base := big.NewInt(58)

	for _, c := range []byte(s) {
		idx := -1
		for i := 0; i < 58; i++ {
			if base58Alphabet[i] == c {
				idx = i
				break
			}
		}
		if idx < 0 {
			return nil, errors.New("crypto: invalid Base58 character")
		}
		result.Mul(result, base)
		result.Add(result, big.NewInt(int64(idx)))
	}

	// Count leading '1's (which represent leading 0x00 bytes).
	numLeadingZeros := 0
	for _, c := range []byte(s) {
		if c != base58Alphabet[0] {
			break
		}
		numLeadingZeros++
	}

	b := result.Bytes()
	out := make([]byte, numLeadingZeros+len(b))
	copy(out[numLeadingZeros:], b)
	return out, nil
}

package crypto

import (
	"crypto/sha256"
	"errors"
	"math/big"
	"strings"

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

// IsValidAddress returns true if the string is a valid NOUS address
// (either Base58Check or Bech32m with "nous1" prefix).
func IsValidAddress(addr string) bool {
	if strings.HasPrefix(strings.ToLower(addr), AddressHRP+"1") {
		_, _, err := Bech32mAddressToPubKeyHash(addr)
		return err == nil
	}
	_, err := AddressToPubKeyHash(Address(addr))
	return err == nil
}

// AddressToScript converts a NOUS address (Base58Check or Bech32m) to a P2PKH locking script:
// OP_DUP OP_HASH160 <20> <pubKeyHash> OP_EQUALVERIFY OP_CHECKSIG
func AddressToScript(addr string) ([]byte, error) {
	var pubKeyHash []byte

	if strings.HasPrefix(strings.ToLower(addr), AddressHRP+"1") {
		_, hash, err := Bech32mAddressToPubKeyHash(addr)
		if err != nil {
			return nil, err
		}
		pubKeyHash = hash
	} else {
		hash, err := AddressToPubKeyHash(Address(addr))
		if err != nil {
			return nil, err
		}
		pubKeyHash = hash
	}

	// Build P2PKH script: OP_DUP OP_HASH160 <20> <hash> OP_EQUALVERIFY OP_CHECKSIG
	script := make([]byte, 25)
	script[0] = 0x76 // OP_DUP
	script[1] = 0xa9 // OP_HASH160
	script[2] = 0x14 // push 20 bytes
	copy(script[3:23], pubKeyHash)
	script[23] = 0x88 // OP_EQUALVERIFY
	script[24] = 0xac // OP_CHECKSIG
	return script, nil
}

package main

import (
	"crypto/hmac"
	"crypto/sha512"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/tyler-smith/go-bip39"

	"nous/crypto"
)

// BIP44 derivation path for NOUS: m/44'/999'/0'/0/index
const (
	Purpose      = 0x8000002C // 44'
	CoinType     = 0x800003E7 // 999'
	Account      = 0x80000000 // 0'
	Change       = 0          // external chain
	hmacKey      = "Bitcoin seed"
)

var curveOrder = secp256k1.S256().N

// HDKey holds a BIP32 extended key.
type HDKey struct {
	key       [32]byte
	chainCode [32]byte
}

// GenerateMnemonic creates a new BIP39 mnemonic (12 or 24 words).
func GenerateMnemonic(wordCount int) (string, error) {
	var bitSize int
	switch wordCount {
	case 12:
		bitSize = 128
	case 24:
		bitSize = 256
	default:
		return "", errors.New("word count must be 12 or 24")
	}
	entropy, err := bip39.NewEntropy(bitSize)
	if err != nil {
		return "", err
	}
	return bip39.NewMnemonic(entropy)
}

// ValidateMnemonic checks if a mnemonic is valid BIP39.
func ValidateMnemonic(mnemonic string) bool {
	return bip39.IsMnemonicValid(mnemonic)
}

// MnemonicToSeed converts mnemonic + optional passphrase to 64-byte seed.
func MnemonicToSeed(mnemonic, passphrase string) []byte {
	return bip39.NewSeed(mnemonic, passphrase)
}

// MasterKeyFromSeed derives BIP32 master key from seed.
func MasterKeyFromSeed(seed []byte) (*HDKey, error) {
	mac := hmac.New(sha512.New, []byte(hmacKey))
	mac.Write(seed)
	out := mac.Sum(nil)

	key := new(big.Int).SetBytes(out[:32])
	if key.Sign() == 0 || key.Cmp(curveOrder) >= 0 {
		return nil, errors.New("invalid master key")
	}

	hd := &HDKey{}
	copy(hd.key[:], out[:32])
	copy(hd.chainCode[:], out[32:])
	return hd, nil
}

// DeriveChild derives a BIP32 child key at the given index.
func (k *HDKey) DeriveChild(index uint32) (*HDKey, error) {
	mac := hmac.New(sha512.New, k.chainCode[:])
	if index >= 0x80000000 {
		// Hardened: 0x00 || privKey || index
		mac.Write([]byte{0x00})
		mac.Write(k.key[:])
	} else {
		// Normal: pubKey || index
		privKey := secp256k1.PrivKeyFromBytes(k.key[:])
		pubKey := privKey.PubKey().SerializeCompressed()
		mac.Write(pubKey)
	}
	var indexBytes [4]byte
	binary.BigEndian.PutUint32(indexBytes[:], index)
	mac.Write(indexBytes[:])

	out := mac.Sum(nil)
	childKeyInt := new(big.Int).SetBytes(out[:32])
	parentKeyInt := new(big.Int).SetBytes(k.key[:])
	childKeyInt.Add(childKeyInt, parentKeyInt)
	childKeyInt.Mod(childKeyInt, curveOrder)

	if childKeyInt.Sign() == 0 {
		return nil, errors.New("invalid child key")
	}

	child := &HDKey{}
	childBytes := childKeyInt.Bytes()
	// Pad to 32 bytes
	copy(child.key[32-len(childBytes):], childBytes)
	copy(child.chainCode[:], out[32:])
	return child, nil
}

// DerivePath derives a key from a BIP32 path like "m/44'/999'/0'/0/0".
func (k *HDKey) DerivePath(path string) (*HDKey, error) {
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] != "m" {
		return nil, errors.New("path must start with m")
	}

	current := k
	for _, part := range parts[1:] {
		hardened := strings.HasSuffix(part, "'")
		indexStr := strings.TrimSuffix(part, "'")
		var index uint32
		_, err := fmt.Sscanf(indexStr, "%d", &index)
		if err != nil {
			return nil, fmt.Errorf("invalid path component: %s", part)
		}
		if hardened {
			index += 0x80000000
		}
		current, err = current.DeriveChild(index)
		if err != nil {
			return nil, err
		}
	}
	return current, nil
}

// DeriveNOUSKey derives the key at m/44'/999'/0'/0/{index}.
func (k *HDKey) DeriveNOUSKey(index uint32) (*HDKey, error) {
	path := fmt.Sprintf("m/44'/999'/0'/0/%d", index)
	return k.DerivePath(path)
}

// PrivateKey returns the crypto.PrivateKey for this HD key.
func (k *HDKey) PrivateKey() *crypto.PrivateKey {
	priv, _ := crypto.PrivateKeyFromBytes(k.key[:])
	return priv
}

// PublicKey returns the crypto.PublicKey for this HD key.
func (k *HDKey) PublicKey() *crypto.PublicKey {
	return k.PrivateKey().PubKey()
}

// Address returns the bech32m address for this HD key.
func (k *HDKey) Address() string {
	return crypto.PubKeyToBech32mAddress(k.PublicKey())
}

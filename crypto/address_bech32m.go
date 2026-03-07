package crypto

import "errors"

// AddressHRP is the human-readable prefix for NOUS Bech32m addresses.
const AddressHRP = "nous"

// Witness version constants for NOUS addresses.
const (
	WitnessVersion0 = 0x00 // current P2PKH (secp256k1 ECDSA)
	WitnessVersion1 = 0x01 // future SLH-DSA (post-quantum)
)

// PubKeyToBech32mAddress encodes a public key as a Bech32m address: nous1q...
func PubKeyToBech32mAddress(pubKey *PublicKey) string {
	hash := Hash160(pubKey.SerializeCompressed())
	return PubKeyHashToBech32mAddress(hash)
}

// PubKeyHashToBech32mAddress encodes a 20-byte public key hash as a Bech32m address.
func PubKeyHashToBech32mAddress(hash []byte) string {
	data5, _ := convertBits(hash, 8, 5, true)
	payload := make([]byte, 1+len(data5))
	payload[0] = WitnessVersion0
	copy(payload[1:], data5)
	addr, _ := Bech32mEncode(AddressHRP, payload)
	return addr
}

// Bech32mAddressToPubKeyHash decodes a nous1 Bech32m address and returns
// the witness version and 20-byte public key hash.
func Bech32mAddressToPubKeyHash(addr string) (version byte, pubKeyHash []byte, err error) {
	hrp, data, err := Bech32mDecode(addr)
	if err != nil {
		return 0, nil, err
	}
	if hrp != AddressHRP {
		return 0, nil, errors.New("bech32m: wrong HRP, expected \"" + AddressHRP + "\"")
	}
	if len(data) < 1 {
		return 0, nil, errors.New("bech32m: empty data")
	}
	version = data[0]
	if version > 16 {
		return 0, nil, errors.New("bech32m: invalid witness version")
	}

	hash, err := convertBits(data[1:], 5, 8, false)
	if err != nil {
		return 0, nil, err
	}

	// Version 0 requires exactly 20 bytes (Hash160).
	if version == WitnessVersion0 && len(hash) != 20 {
		return 0, nil, errors.New("bech32m: v0 address must be 20 bytes")
	}

	return version, hash, nil
}

// convertBits converts a byte slice between bit groups (e.g., 8-bit to 5-bit).
// This implements the BIP-173 bit conversion algorithm.
func convertBits(data []byte, fromBits, toBits int, pad bool) ([]byte, error) {
	acc := 0
	bits := 0
	maxV := (1 << uint(toBits)) - 1
	var ret []byte

	for _, b := range data {
		if int(b)>>uint(fromBits) != 0 {
			return nil, errors.New("bech32m: invalid data value")
		}
		acc = (acc << uint(fromBits)) | int(b)
		bits += fromBits
		for bits >= toBits {
			bits -= toBits
			ret = append(ret, byte((acc>>uint(bits))&maxV))
		}
	}

	if pad {
		if bits > 0 {
			ret = append(ret, byte((acc<<uint(toBits-bits))&maxV))
		}
	} else {
		if bits >= fromBits {
			return nil, errors.New("bech32m: non-zero padding")
		}
		if (acc<<uint(toBits-bits))&maxV != 0 {
			return nil, errors.New("bech32m: non-zero padding bits")
		}
	}

	return ret, nil
}

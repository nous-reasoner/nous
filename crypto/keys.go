package crypto

import (
	"errors"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	decdsa "github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
)

// PrivateKey wraps a secp256k1 private key.
type PrivateKey struct {
	inner *secp256k1.PrivateKey
}

// PublicKey wraps a secp256k1 public key.
type PublicKey struct {
	inner *secp256k1.PublicKey
}

// Signature wraps a secp256k1 ECDSA signature.
type Signature struct {
	inner *decdsa.Signature
}

// GenerateKeyPair creates a new random secp256k1 key pair.
func GenerateKeyPair() (*PrivateKey, *PublicKey, error) {
	key, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		return nil, nil, err
	}
	priv := &PrivateKey{inner: key}
	pub := &PublicKey{inner: key.PubKey()}
	return priv, pub, nil
}

// PubKey derives the public key from this private key.
func (k *PrivateKey) PubKey() *PublicKey {
	return &PublicKey{inner: k.inner.PubKey()}
}

// Bytes returns the private key as a 32-byte big-endian scalar.
func (k *PrivateKey) Bytes() []byte {
	b := k.inner.Serialize()
	return b
}

// PrivateKeyFromBytes restores a PrivateKey from its 32-byte representation.
func PrivateKeyFromBytes(b []byte) (*PrivateKey, error) {
	if len(b) != 32 {
		return nil, errors.New("crypto: private key must be 32 bytes")
	}
	key := secp256k1.PrivKeyFromBytes(b)
	return &PrivateKey{inner: key}, nil
}

// SerializeCompressed returns the 33-byte SEC compressed encoding of the public key.
func (k *PublicKey) SerializeCompressed() []byte {
	return k.inner.SerializeCompressed()
}

// SerializeUncompressed returns the 65-byte SEC uncompressed encoding.
func (k *PublicKey) SerializeUncompressed() []byte {
	return k.inner.SerializeUncompressed()
}

// ParsePublicKey deserializes a public key from compressed (33-byte) or
// uncompressed (65-byte) SEC format.
func ParsePublicKey(data []byte) (*PublicKey, error) {
	key, err := secp256k1.ParsePubKey(data)
	if err != nil {
		return nil, err
	}
	return &PublicKey{inner: key}, nil
}

// IsEqual returns true if two public keys represent the same point.
func (k *PublicKey) IsEqual(other *PublicKey) bool {
	return k.inner.IsEqual(other.inner)
}

// Sign produces an ECDSA signature of the given hash using the private key.
func Sign(privKey *PrivateKey, hash Hash) (*Signature, error) {
	sig := decdsa.Sign(privKey.inner, hash[:])
	return &Signature{inner: sig}, nil
}

// Verify checks an ECDSA signature against the public key and hash.
func Verify(pubKey *PublicKey, hash Hash, sig *Signature) bool {
	return sig.inner.Verify(hash[:], pubKey.inner)
}

// Bytes serializes the signature in compact R || S format (64 bytes).
func (sig *Signature) Bytes() []byte {
	compact := sig.inner.Serialize()
	// decred's Serialize returns DER encoding. We want compact 64-byte format.
	// Extract R and S from the signature and pack them.
	r, s := extractRS(compact)
	out := make([]byte, 64)
	copy(out[32-len(r):32], r)
	copy(out[64-len(s):64], s)
	return out
}

// extractRS parses R and S from DER-encoded signature.
// DER format: 0x30 <len> 0x02 <rlen> <r> 0x02 <slen> <s>
func extractRS(der []byte) (r, s []byte) {
	// Skip 0x30 <total_len>
	pos := 2
	// Read R
	pos++ // skip 0x02
	rLen := int(der[pos])
	pos++
	r = der[pos : pos+rLen]
	pos += rLen
	// Read S
	pos++ // skip 0x02
	sLen := int(der[pos])
	pos++
	s = der[pos : pos+sLen]

	// Strip leading zero bytes (DER uses signed encoding).
	for len(r) > 1 && r[0] == 0 {
		r = r[1:]
	}
	for len(s) > 1 && s[0] == 0 {
		s = s[1:]
	}
	return
}

// SignatureFromBytes deserializes a 64-byte compact (R || S) signature.
func SignatureFromBytes(b []byte) (*Signature, error) {
	if len(b) != 64 {
		return nil, errors.New("crypto: signature must be 64 bytes")
	}
	r := new(secp256k1.ModNScalar)
	if overflow := r.SetByteSlice(b[:32]); overflow {
		return nil, errors.New("crypto: signature R overflows curve order")
	}
	s := new(secp256k1.ModNScalar)
	if overflow := s.SetByteSlice(b[32:]); overflow {
		return nil, errors.New("crypto: signature S overflows curve order")
	}
	if r.IsZero() || s.IsZero() {
		return nil, errors.New("crypto: invalid signature (zero component)")
	}
	sig := decdsa.NewSignature(r, s)
	return &Signature{inner: sig}, nil
}

package crypto

import (
	"bytes"
	"crypto/sha256"
	"testing"

	"golang.org/x/crypto/ripemd160"
)

func TestSha256(t *testing.T) {
	data := []byte("NOUS blockchain")
	h := Sha256(data)
	if h.IsZero() {
		t.Fatal("SHA-256 should not produce zero hash for non-empty input")
	}
	h2 := Sha256(data)
	if h != h2 {
		t.Fatal("SHA-256 should be deterministic")
	}
}

func TestDoubleSha256(t *testing.T) {
	data := []byte("NOUS blockchain")
	single := Sha256(data)
	double := DoubleSha256(data)
	if single == double {
		t.Fatal("DoubleSha256 should differ from single Sha256")
	}
	expected := Sha256(single[:])
	if double != expected {
		t.Fatal("DoubleSha256 result mismatch")
	}
}

func TestHashFromHex(t *testing.T) {
	data := []byte("test")
	h := Sha256(data)
	hexStr := h.String()

	recovered, err := HashFromHex(hexStr)
	if err != nil {
		t.Fatalf("HashFromHex failed: %v", err)
	}
	if recovered != h {
		t.Fatal("HashFromHex round-trip failed")
	}

	_, err = HashFromHex("not-hex")
	if err == nil {
		t.Fatal("should reject invalid hex")
	}
	_, err = HashFromHex("abcd")
	if err == nil {
		t.Fatal("should reject wrong-length hex")
	}
}

func TestHashCompare(t *testing.T) {
	low := Hash{}
	high := Hash{}
	high[0] = 0xFF

	if low.Compare(high) >= 0 {
		t.Fatal("zero hash should be less than 0xFF... hash")
	}
	if high.Compare(low) <= 0 {
		t.Fatal("0xFF... hash should be greater than zero hash")
	}
	if low.Compare(low) != 0 {
		t.Fatal("same hash should compare equal")
	}
}

func TestGenerateKeyPair(t *testing.T) {
	priv, pub, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}
	if len(priv.Bytes()) != 32 {
		t.Fatal("private key should be 32 bytes")
	}
	compressed := pub.SerializeCompressed()
	if len(compressed) != 33 {
		t.Fatal("compressed pubkey should be 33 bytes")
	}
}

func TestPrivateKeyRoundTrip(t *testing.T) {
	priv, _, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	b := priv.Bytes()
	if len(b) != 32 {
		t.Fatalf("private key bytes should be 32, got %d", len(b))
	}

	restored, err := PrivateKeyFromBytes(b)
	if err != nil {
		t.Fatalf("PrivateKeyFromBytes failed: %v", err)
	}
	if !bytes.Equal(priv.Bytes(), restored.Bytes()) {
		t.Fatal("private key round-trip failed")
	}
}

func TestPublicKeyCompressedRoundTrip(t *testing.T) {
	_, pub, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	compressed := pub.SerializeCompressed()
	if len(compressed) != 33 {
		t.Fatalf("compressed pubkey should be 33 bytes, got %d", len(compressed))
	}
	if compressed[0] != 0x02 && compressed[0] != 0x03 {
		t.Fatalf("compressed pubkey prefix should be 02 or 03, got %02x", compressed[0])
	}

	restored, err := ParsePublicKey(compressed)
	if err != nil {
		t.Fatalf("ParsePublicKey(compressed) failed: %v", err)
	}
	if !pub.IsEqual(restored) {
		t.Fatal("compressed pubkey round-trip failed")
	}
}

func TestPublicKeyUncompressedRoundTrip(t *testing.T) {
	_, pub, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	uncompressed := pub.SerializeUncompressed()
	if len(uncompressed) != 65 {
		t.Fatalf("uncompressed pubkey should be 65 bytes, got %d", len(uncompressed))
	}

	restored, err := ParsePublicKey(uncompressed)
	if err != nil {
		t.Fatalf("ParsePublicKey(uncompressed) failed: %v", err)
	}
	if !pub.IsEqual(restored) {
		t.Fatal("uncompressed pubkey round-trip failed")
	}
}

func TestSignAndVerify(t *testing.T) {
	priv, pub, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	msg := DoubleSha256([]byte("test transaction"))
	sig, err := Sign(priv, msg)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	if !Verify(pub, msg, sig) {
		t.Fatal("valid signature should verify")
	}

	// Tamper with message.
	tampered := msg
	tampered[0] ^= 0xFF
	if Verify(pub, tampered, sig) {
		t.Fatal("signature should not verify with tampered message")
	}

	// Wrong key.
	_, otherPub, _ := GenerateKeyPair()
	if Verify(otherPub, msg, sig) {
		t.Fatal("signature should not verify with wrong public key")
	}
}

func TestSignatureRoundTrip(t *testing.T) {
	priv, pub, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	msg := Sha256([]byte("data"))
	sig, err := Sign(priv, msg)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	b := sig.Bytes()
	if len(b) != 64 {
		t.Fatalf("signature bytes should be 64, got %d", len(b))
	}

	restored, err := SignatureFromBytes(b)
	if err != nil {
		t.Fatalf("SignatureFromBytes failed: %v", err)
	}
	// Verify the restored signature still works.
	if !Verify(pub, msg, restored) {
		t.Fatal("restored signature should still verify")
	}
}

func TestPubKeyToAddress(t *testing.T) {
	_, pub, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	addr := PubKeyToAddress(pub)
	if len(addr) == 0 {
		t.Fatal("address should not be empty")
	}
	if addr[0] != '1' {
		t.Fatalf("address should start with '1', got '%c'", addr[0])
	}
	t.Logf("generated address: %s", addr)
}

func TestAddressRoundTrip(t *testing.T) {
	_, pub, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	addr := PubKeyToAddress(pub)

	pubKeyHash, err := AddressToPubKeyHash(addr)
	if err != nil {
		t.Fatalf("AddressToPubKeyHash failed: %v", err)
	}
	if len(pubKeyHash) != 20 {
		t.Fatalf("pubkey hash should be 20 bytes, got %d", len(pubKeyHash))
	}

	// Re-derive pubkey hash manually and compare.
	compressed := pub.SerializeCompressed()
	shaHash := sha256.Sum256(compressed)
	riper := ripemd160.New()
	riper.Write(shaHash[:])
	expected := riper.Sum(nil)

	if !bytes.Equal(pubKeyHash, expected) {
		t.Fatal("address decode round-trip: pubkey hash mismatch")
	}
}

func TestAddressChecksumValidation(t *testing.T) {
	_, pub, _ := GenerateKeyPair()
	addr := PubKeyToAddress(pub)

	// Flip a character in the middle of the address for reliable corruption.
	chars := []byte(addr)
	mid := len(chars) / 2
	if chars[mid] == 'A' {
		chars[mid] = 'B'
	} else {
		chars[mid] = 'A'
	}
	_, err := AddressToPubKeyHash(Address(string(chars)))
	if err == nil {
		t.Fatal("corrupted address should fail checksum validation")
	}
}

func TestPubKeyDerivedFromPrivKey(t *testing.T) {
	priv, pub, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}
	derived := priv.PubKey()
	if !pub.IsEqual(derived) {
		t.Fatal("PubKey() derived from PrivateKey should match original")
	}
}

func TestDeterministicKeyFromBytes(t *testing.T) {
	keyBytes := Sha256([]byte("NOUS deterministic key seed"))
	priv, err := PrivateKeyFromBytes(keyBytes[:])
	if err != nil {
		t.Fatalf("PrivateKeyFromBytes failed: %v", err)
	}

	pub := priv.PubKey()
	addr := PubKeyToAddress(pub)
	t.Logf("deterministic key -> address: %s", addr)

	priv2, err := PrivateKeyFromBytes(keyBytes[:])
	if err != nil {
		t.Fatalf("PrivateKeyFromBytes (2nd) failed: %v", err)
	}
	addr2 := PubKeyToAddress(priv2.PubKey())
	if addr != addr2 {
		t.Fatal("same seed should produce the same address")
	}
}

func TestBase58RoundTrip(t *testing.T) {
	cases := [][]byte{
		{0x00, 0x00, 0x01},
		{0xFF, 0xFF},
		{0x00},
		{0x01, 0x02, 0x03, 0x04, 0x05},
	}
	for _, tc := range cases {
		encoded := base58Encode(tc)
		decoded, err := base58Decode(encoded)
		if err != nil {
			t.Fatalf("base58Decode(%q) failed: %v", encoded, err)
		}
		if !bytes.Equal(tc, decoded) {
			t.Fatalf("base58 round-trip failed for %x: encoded=%q decoded=%x", tc, encoded, decoded)
		}
	}
}

package crypto

import (
	"bytes"
	"strings"
	"testing"
)

func TestBech32mEncode(t *testing.T) {
	// Encode empty data with HRP "a".
	encoded, err := Bech32mEncode("a", []byte{})
	if err != nil {
		t.Fatal(err)
	}
	// Should be "a1" + 6 checksum characters.
	if !strings.HasPrefix(encoded, "a1") {
		t.Errorf("expected prefix 'a1', got %q", encoded)
	}
	if len(encoded) != 2+6 {
		t.Errorf("expected length 8, got %d", len(encoded))
	}

	// Encode known 5-bit data.
	data := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	encoded, err = Bech32mEncode("test", data)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(encoded, "test1") {
		t.Errorf("expected prefix 'test1', got %q", encoded)
	}
}

func TestBech32mDecode(t *testing.T) {
	// Encode then decode.
	data := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 15, 20, 25, 31}
	encoded, err := Bech32mEncode("nous", data)
	if err != nil {
		t.Fatal(err)
	}
	hrp, decoded, err := Bech32mDecode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if hrp != "nous" {
		t.Errorf("hrp = %q, want %q", hrp, "nous")
	}
	if !bytes.Equal(decoded, data) {
		t.Errorf("decoded data mismatch")
	}
}

func TestBech32mRoundTrip(t *testing.T) {
	hrps := []string{"nous", "test", "bc", "a"}
	for _, hrp := range hrps {
		for length := 0; length <= 40; length += 10 {
			data := make([]byte, length)
			for i := range data {
				data[i] = byte(i % 32)
			}
			encoded, err := Bech32mEncode(hrp, data)
			if err != nil {
				t.Fatalf("encode(%q, len=%d): %v", hrp, length, err)
			}
			gotHRP, gotData, err := Bech32mDecode(encoded)
			if err != nil {
				t.Fatalf("decode(%q): %v", encoded, err)
			}
			if gotHRP != hrp {
				t.Errorf("hrp = %q, want %q", gotHRP, hrp)
			}
			if !bytes.Equal(gotData, data) {
				t.Errorf("data mismatch for hrp=%q len=%d", hrp, length)
			}
		}
	}
}

func TestBech32mInvalidChecksum(t *testing.T) {
	data := []byte{0, 1, 2, 3, 4}
	encoded, err := Bech32mEncode("nous", data)
	if err != nil {
		t.Fatal(err)
	}

	// Tamper the last character.
	tampered := encoded[:len(encoded)-1]
	lastChar := encoded[len(encoded)-1]
	if lastChar == 'q' {
		tampered += "p"
	} else {
		tampered += "q"
	}

	_, _, err = Bech32mDecode(tampered)
	if err == nil {
		t.Error("expected error for tampered checksum, got nil")
	}
}

func TestNousAddress(t *testing.T) {
	for i := 0; i < 10; i++ {
		_, pub, err := GenerateKeyPair()
		if err != nil {
			t.Fatal(err)
		}

		addr := PubKeyToBech32mAddress(pub)
		version, hash, err := Bech32mAddressToPubKeyHash(addr)
		if err != nil {
			t.Fatalf("decode address %q: %v", addr, err)
		}

		if version != WitnessVersion0 {
			t.Errorf("version = %d, want %d", version, WitnessVersion0)
		}

		expectedHash := Hash160(pub.SerializeCompressed())
		if !bytes.Equal(hash, expectedHash) {
			t.Errorf("pubkey hash mismatch for address %q", addr)
		}
	}
}

func TestNousAddressPrefix(t *testing.T) {
	for i := 0; i < 20; i++ {
		_, pub, err := GenerateKeyPair()
		if err != nil {
			t.Fatal(err)
		}
		addr := PubKeyToBech32mAddress(pub)
		if !strings.HasPrefix(addr, "nous1") {
			t.Errorf("address %q does not start with 'nous1'", addr)
		}
	}
}

func TestConvertBits(t *testing.T) {
	testCases := [][]byte{
		{},
		{0x00},
		{0xff},
		{0x00, 0xff, 0x00},
		{0xde, 0xad, 0xbe, 0xef},
		// 20-byte hash (typical Hash160 output).
		{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a,
			0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10, 0x11, 0x12, 0x13, 0x14},
	}

	for _, tc := range testCases {
		// 8 → 5
		data5, err := convertBits(tc, 8, 5, true)
		if err != nil {
			t.Fatalf("8→5 convertBits(%x): %v", tc, err)
		}
		// Verify all values are < 32.
		for _, v := range data5 {
			if v > 31 {
				t.Fatalf("5-bit value %d > 31", v)
			}
		}
		// 5 → 8
		data8, err := convertBits(data5, 5, 8, false)
		if err != nil {
			t.Fatalf("5→8 convertBits: %v", err)
		}
		if !bytes.Equal(data8, tc) {
			t.Errorf("round-trip failed: input=%x, got=%x", tc, data8)
		}
	}
}

func TestIsValidAddress(t *testing.T) {
	_, pub, _ := GenerateKeyPair()

	// Bech32m address.
	bech32mAddr := PubKeyToBech32mAddress(pub)
	if !IsValidAddress(bech32mAddr) {
		t.Errorf("valid bech32m address %q rejected", bech32mAddr)
	}

	// Base58Check address.
	base58Addr := PubKeyToAddress(pub)
	if !IsValidAddress(string(base58Addr)) {
		t.Errorf("valid base58 address %q rejected", base58Addr)
	}

	// Invalid address.
	if IsValidAddress("invalid") {
		t.Error("invalid address accepted")
	}
	if IsValidAddress("nous1invalidchecksum") {
		t.Error("invalid bech32m address accepted")
	}
}

func TestAddressToScript(t *testing.T) {
	_, pub, _ := GenerateKeyPair()
	expectedHash := Hash160(pub.SerializeCompressed())

	// From Bech32m address.
	bech32mAddr := PubKeyToBech32mAddress(pub)
	script, err := AddressToScript(bech32mAddr)
	if err != nil {
		t.Fatal(err)
	}
	// Verify P2PKH script structure.
	if len(script) != 25 {
		t.Fatalf("script length = %d, want 25", len(script))
	}
	if script[0] != 0x76 || script[1] != 0xa9 || script[2] != 0x14 {
		t.Error("wrong script prefix")
	}
	if !bytes.Equal(script[3:23], expectedHash) {
		t.Error("wrong pubkey hash in script")
	}
	if script[23] != 0x88 || script[24] != 0xac {
		t.Error("wrong script suffix")
	}

	// From Base58Check address — should produce same script.
	base58Addr := PubKeyToAddress(pub)
	script2, err := AddressToScript(string(base58Addr))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(script, script2) {
		t.Error("Bech32m and Base58 scripts differ for same key")
	}
}

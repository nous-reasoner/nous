package network

import (
	"bytes"
	"testing"
	"time"
)

// TestBanScoreAccumulation verifies that multiple score additions
// accumulate to 100 and trigger a ban.
func TestBanScoreAccumulationToThreshold(t *testing.T) {
	pp := NewPeerProtection()
	addr := "192.168.1.1:9333"

	// Add 20 points five times = 100 total.
	for i := 0; i < 4; i++ {
		if pp.AddScore(addr, 20) {
			t.Fatalf("score %d should not trigger ban", (i+1)*20)
		}
	}
	if pp.GetScore(addr) != 80 {
		t.Fatalf("score: want 80, got %d", pp.GetScore(addr))
	}
	if pp.IsBanned(addr) {
		t.Fatal("should not be banned at score 80")
	}

	// Fifth addition reaches 100 → banned.
	if !pp.AddScore(addr, 20) {
		t.Fatal("score 100 should trigger ban")
	}
	if !pp.IsBanned(addr) {
		t.Fatal("should be banned at score 100")
	}
	if pp.BanCount() != 1 {
		t.Fatalf("ban count: want 1, got %d", pp.BanCount())
	}
}

// TestBanExpiry verifies that a ban expires after BanDuration (24 hours).
func TestBanExpiry(t *testing.T) {
	pp := NewPeerProtection()
	addr := "10.0.0.5:9333"

	// Ban the peer.
	pp.AddScore(addr, BanScoreThreshold)
	if !pp.IsBanned(addr) {
		t.Fatal("should be banned immediately after reaching threshold")
	}

	// Manually set BannedAt to 24 hours ago to simulate time passing.
	pp.mu.Lock()
	host := hostOnly(addr)
	if entry, ok := pp.banned[host]; ok {
		entry.BannedAt = time.Now().Add(-BanDuration - time.Second)
	}
	pp.mu.Unlock()

	// Should no longer be banned.
	if pp.IsBanned(addr) {
		t.Fatal("ban should have expired after 24 hours")
	}

	// Ban entry and score should be cleaned up.
	if pp.BanCount() != 0 {
		t.Fatalf("ban count after expiry: want 0, got %d", pp.BanCount())
	}
	if pp.GetScore(addr) != 0 {
		t.Fatalf("score after ban expiry: want 0, got %d", pp.GetScore(addr))
	}
}

// TestBannedPeerRejected verifies that a banned IP is rejected
// regardless of port, simulating CheckBanned at connection time.
func TestBannedPeerRejectedAllPorts(t *testing.T) {
	pp := NewPeerProtection()

	// Ban via one port.
	pp.AddScore("172.16.0.1:9333", BanScoreThreshold)

	// Verify the same IP on different ports is rejected.
	ports := []string{"172.16.0.1:9333", "172.16.0.1:12345", "172.16.0.1:80"}
	for _, addr := range ports {
		if !pp.IsBanned(addr) {
			t.Fatalf("banned IP should be rejected on %s", addr)
		}
	}

	// A different IP should NOT be banned.
	if pp.IsBanned("172.16.0.2:9333") {
		t.Fatal("different IP should not be banned")
	}
}

// TestNormalPeerUnaffected verifies that a peer with a low ban score
// is not banned and can continue operating normally.
func TestNormalPeerUnaffected(t *testing.T) {
	pp := NewPeerProtection()
	addr := "10.0.0.10:9333"

	// Add small penalties that stay well under threshold.
	pp.AddScore(addr, BanScorePolicyTx) // +5
	pp.AddScore(addr, BanScoreUnknownCmd) // +10
	pp.AddScore(addr, BanScorePolicyTx) // +5

	totalExpected := BanScorePolicyTx + BanScoreUnknownCmd + BanScorePolicyTx // 20
	if pp.GetScore(addr) != totalExpected {
		t.Fatalf("score: want %d, got %d", totalExpected, pp.GetScore(addr))
	}

	// Should NOT be banned.
	if pp.IsBanned(addr) {
		t.Fatal("peer with low score should not be banned")
	}

	// Rate limiting should still allow messages.
	if !pp.CheckRate(addr) {
		t.Fatal("normal peer should pass rate check")
	}

	// Ban count should be 0.
	if pp.BanCount() != 0 {
		t.Fatalf("ban count: want 0, got %d", pp.BanCount())
	}
}

// TestMessageSizeCheck verifies that messages exceeding MaxPayloadSize (17 MB)
// are rejected at both encode and decode time.
func TestMessageSizeCheck(t *testing.T) {
	// Build a payload that exceeds MaxPayloadSize (17 MB).
	oversized := make([]byte, MaxPayloadSize+1)
	msg := &MsgBlock{Payload: oversized}

	// EncodeMessage should reject the oversized payload.
	_, err := EncodeMessage(MainNetMagic, msg)
	if err == nil {
		t.Fatal("EncodeMessage should reject payload exceeding 17 MB")
	}

	// Also verify that DecodeMessage rejects a crafted header claiming a huge payload.
	var wire bytes.Buffer
	// magic (LE)
	wire.Write([]byte{0x53, 0x55, 0x4F, 0x4E}) // MainNetMagic LE
	// command "block\x00..."
	var cmd [CommandSize]byte
	copy(cmd[:], "block")
	wire.Write(cmd[:])
	// payload length: MaxPayloadSize + 1 (LE)
	tooBig := uint32(MaxPayloadSize + 1)
	wire.Write([]byte{
		byte(tooBig), byte(tooBig >> 8),
		byte(tooBig >> 16), byte(tooBig >> 24),
	})
	// checksum (doesn't matter, size check comes first)
	wire.Write([]byte{0, 0, 0, 0})

	_, err = DecodeMessage(bytes.NewReader(wire.Bytes()), MainNetMagic)
	if err == nil {
		t.Fatal("DecodeMessage should reject payload size exceeding 17 MB")
	}
}

// TestRateLimitCheck verifies that a peer sending more than
// RateLimitPerSecond (100) messages per second is throttled.
func TestRateLimitCheck(t *testing.T) {
	pp := NewPeerProtection()
	addr := "10.0.0.20:9333"

	// First 100 messages should all be allowed.
	for i := 0; i < RateLimitPerSecond; i++ {
		if !pp.CheckRate(addr) {
			t.Fatalf("message %d should be allowed (limit is %d)", i+1, RateLimitPerSecond)
		}
	}

	// Message 101 should be denied.
	if pp.CheckRate(addr) {
		t.Fatal("message beyond rate limit (>100/sec) should be denied")
	}

	// Additional messages should continue to be denied within the same window.
	if pp.CheckRate(addr) {
		t.Fatal("continued messages beyond rate limit should be denied")
	}
}

package network

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestAddrManagerNewBucket(t *testing.T) {
	am := NewAddrManager("", nil)

	// Add addresses from two different sources.
	source1 := net.ParseIP("1.1.0.1")
	source2 := net.ParseIP("2.2.0.1")

	am.AddAddresses([]NetAddress{
		{IP: "8.8.8.8", Port: 8333},
	}, source1)
	am.AddAddresses([]NetAddress{
		{IP: "9.9.9.9", Port: 8333},
	}, source2)

	nNew, nTried := am.Count()
	if nNew != 2 {
		t.Fatalf("expected 2 new addresses, got %d", nNew)
	}
	if nTried != 0 {
		t.Fatalf("expected 0 tried addresses, got %d", nTried)
	}

	// Same address added again should not increase count.
	am.AddAddresses([]NetAddress{
		{IP: "8.8.8.8", Port: 8333},
	}, source1)
	nNew, _ = am.Count()
	if nNew != 2 {
		t.Fatalf("expected 2 after dedup, got %d", nNew)
	}
}

func TestAddrManagerTriedBucket(t *testing.T) {
	am := NewAddrManager("", nil)

	addr := NetAddress{IP: "8.8.8.8", Port: 8333}
	am.AddAddresses([]NetAddress{addr}, net.ParseIP("1.1.0.1"))

	nNew, nTried := am.Count()
	if nNew != 1 || nTried != 0 {
		t.Fatalf("before MarkGood: new=%d tried=%d, want new=1 tried=0", nNew, nTried)
	}

	// MarkGood should move from new to tried.
	am.MarkGood(addr)

	nNew, nTried = am.Count()
	if nNew != 0 || nTried != 1 {
		t.Fatalf("after MarkGood: new=%d tried=%d, want new=0 tried=1", nNew, nTried)
	}
}

func TestAddrManagerMaxPerSource(t *testing.T) {
	am := NewAddrManager("", nil)

	source := net.ParseIP("1.1.0.1")

	// Add MaxAddrPerSource + 50 addresses from the same source.
	addrs := make([]NetAddress, MaxAddrPerSource+50)
	for i := range addrs {
		// Use different /16 subnets to avoid bucket collisions.
		addrs[i] = NetAddress{IP: net.IPv4(byte(i/250+1), byte(i%250+1), 1, 1).String(), Port: 8333}
	}
	am.AddAddresses(addrs, source)

	total := am.Total()
	if total > MaxAddrPerSource {
		t.Fatalf("expected at most %d addresses from one source, got %d", MaxAddrPerSource, total)
	}
}

func TestAddrManagerPersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "peers.json")

	// Create and populate.
	am1 := NewAddrManager(path, []string{"seed1:8333"})
	am1.AddAddresses([]NetAddress{
		{IP: "1.2.3.4", Port: 8333},
		{IP: "5.6.7.8", Port: 8333},
	}, net.ParseIP("99.99.99.99"))

	// Mark one as good (tried).
	am1.MarkGood(NetAddress{IP: "1.2.3.4", Port: 8333})

	if err := am1.SaveToFile(); err != nil {
		t.Fatalf("save: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file missing: %v", err)
	}

	// Load into a new AddrManager.
	am2 := NewAddrManager(path, []string{"seed1:8333"})
	if err := am2.LoadFromFile(); err != nil {
		t.Fatalf("load: %v", err)
	}

	nNew, nTried := am2.Count()
	if nNew != 1 || nTried != 1 {
		t.Fatalf("after load: new=%d tried=%d, want new=1 tried=1", nNew, nTried)
	}
	if len(am2.Seeds()) != 1 || am2.Seeds()[0] != "seed1:8333" {
		t.Fatalf("seeds not preserved")
	}
}

func TestAddrManagerLegacyMigration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "peers.json")

	// Write legacy AddressBook format.
	legacy := `{"addresses":[{"addr":{"IP":"1.2.3.4","Port":8333},"last_seen":"2026-01-01T00:00:00Z","failures":0}]}`
	os.WriteFile(path, []byte(legacy), 0600)

	am := NewAddrManager(path, nil)
	if err := am.LoadFromFile(); err != nil {
		t.Fatalf("load legacy: %v", err)
	}

	if am.Total() != 1 {
		t.Fatalf("expected 1 address after legacy migration, got %d", am.Total())
	}
}

func TestAddrManagerSelectForOutbound(t *testing.T) {
	am := NewAddrManager("", nil)

	// Empty manager should return nil.
	if addr := am.SelectForOutbound(); addr != nil {
		t.Fatalf("expected nil from empty manager, got %v", addr)
	}

	// Add some addresses.
	am.AddAddresses([]NetAddress{
		{IP: "1.2.3.4", Port: 8333},
		{IP: "5.6.7.8", Port: 8333},
	}, net.ParseIP("99.99.99.99"))

	// Should return something.
	addr := am.SelectForOutbound()
	if addr == nil {
		t.Fatal("expected non-nil address")
	}
}

func TestAddrManagerMarkFailed(t *testing.T) {
	am := NewAddrManager("", nil)

	addr := NetAddress{IP: "1.2.3.4", Port: 8333}
	am.AddAddresses([]NetAddress{addr}, net.ParseIP("99.99.99.99"))

	if am.Total() != 1 {
		t.Fatalf("expected 1, got %d", am.Total())
	}

	// Mark failed enough times to remove.
	for i := 0; i < maxAddrAttempts; i++ {
		am.MarkFailed(addr)
	}

	if am.Total() != 0 {
		t.Fatalf("expected 0 after failures, got %d", am.Total())
	}
}

func TestAddrManagerBucketDeterminism(t *testing.T) {
	am := NewAddrManager("", nil)

	addr := "1.2.3.4:8333"
	source := "5.6.7.8"

	b1 := am.newBucketIdx(addr, source)
	b2 := am.newBucketIdx(addr, source)
	if b1 != b2 {
		t.Fatalf("bucket assignment not deterministic: %d vs %d", b1, b2)
	}

	// Different source should (usually) give a different bucket.
	b3 := am.newBucketIdx(addr, "99.99.99.99")
	// Not guaranteed to be different, but with 1024 buckets it's overwhelmingly likely.
	_ = b3
}

func TestAddrManagerPrivateIPsRejected(t *testing.T) {
	am := NewAddrManager("", nil)

	am.AddAddresses([]NetAddress{
		{IP: "127.0.0.1", Port: 8333},
		{IP: "10.0.0.1", Port: 8333},
		{IP: "192.168.1.1", Port: 8333},
		{IP: "8.8.8.8", Port: 8333},
	}, net.ParseIP("99.99.99.99"))

	if am.Total() != 1 {
		t.Fatalf("expected 1 public address, got %d", am.Total())
	}
}

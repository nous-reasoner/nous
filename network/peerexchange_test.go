package network

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAddressBookPersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "peers.json")

	// Create and populate an address book.
	ab := NewAddressBook(nil)
	ab.AddAddress(NetAddress{IP: "1.2.3.4", Port: 9333})
	ab.AddAddress(NetAddress{IP: "5.6.7.8", Port: 9333})
	ab.AddAddress(NetAddress{IP: "9.10.11.12", Port: 9334})

	if ab.Count() != 3 {
		t.Fatalf("expected 3 addresses, got %d", ab.Count())
	}

	// Save to file.
	if err := ab.SaveToFile(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Verify file exists.
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file missing: %v", err)
	}

	// Load from file.
	ab2, err := LoadAddressBook(path, []string{"seed1:9333"})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if ab2.Count() != 3 {
		t.Fatalf("loaded %d addresses, want 3", ab2.Count())
	}
	if len(ab2.Seeds()) != 1 || ab2.Seeds()[0] != "seed1:9333" {
		t.Fatalf("seeds not preserved")
	}
}

func TestGetAddrResponse(t *testing.T) {
	// Set up a server with addresses in its book.
	cfg := ServerConfig{
		ListenAddr: ":0",
		Magic:      TestNetMagic,
	}
	server := NewServer(cfg)
	server.addrBook.AddAddress(NetAddress{IP: "1.2.3.4", Port: 9333})
	server.addrBook.AddAddress(NetAddress{IP: "5.6.7.8", Port: 9333})

	if err := server.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer server.Stop()

	// Connect a client.
	conn, err := net.DialTimeout("tcp", server.ListenAddr(), 5*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Send version.
	ver := &MsgVersion{
		Version:     ProtocolVersion,
		BlockHeight: 0,
		Timestamp:   uint64(time.Now().Unix()),
		Nonce:       12345,
		UserAgent:   "test",
		ListenPort:  9999,
	}
	data, _ := EncodeMessage(TestNetMagic, ver)
	conn.Write(data)

	// Read server's verack.
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	msg, err := DecodeMessage(conn, TestNetMagic)
	if err != nil {
		t.Fatalf("read verack: %v", err)
	}
	if msg.Command() != CmdVerAck {
		t.Fatalf("expected verack, got %s", msg.Command())
	}

	// Read server's version (inbound peer gets version sent).
	msg, err = DecodeMessage(conn, TestNetMagic)
	if err != nil {
		t.Fatalf("read version: %v", err)
	}
	if msg.Command() != CmdVersion {
		t.Fatalf("expected version, got %s", msg.Command())
	}

	// Send verack to complete handshake.
	data, _ = EncodeMessage(TestNetMagic, &MsgVerAck{})
	conn.Write(data)

	// The server sends getaddr after completing the handshake (handleVerAck).
	// We may receive it before our getaddr response. Read messages until we
	// either consume the server's getaddr or get an addr.
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	// Drain the server's getaddr if present.
	msg, err = DecodeMessage(conn, TestNetMagic)
	if err != nil {
		t.Fatalf("read after verack: %v", err)
	}
	if msg.Command() == CmdGetAddr {
		// Server asked us for addresses — expected.
	}

	// Now send getaddr.
	data, _ = EncodeMessage(TestNetMagic, &MsgGetAddr{})
	conn.Write(data)

	// Read addr response.
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	msg, err = DecodeMessage(conn, TestNetMagic)
	if err != nil {
		t.Fatalf("read addr: %v", err)
	}
	addrMsg, ok := msg.(*MsgAddr)
	if !ok {
		t.Fatalf("expected MsgAddr, got %T", msg)
	}
	if len(addrMsg.Addresses) < 2 {
		t.Fatalf("expected at least 2 addresses, got %d", len(addrMsg.Addresses))
	}
}

func TestAddressBookDedup(t *testing.T) {
	ab := NewAddressBook(nil)

	// Add same address multiple times.
	ab.AddAddress(NetAddress{IP: "1.2.3.4", Port: 9333})
	ab.AddAddress(NetAddress{IP: "1.2.3.4", Port: 9333})
	ab.AddAddress(NetAddress{IP: "1.2.3.4", Port: 9333})

	if ab.Count() != 1 {
		t.Fatalf("expected 1 address after dedup, got %d", ab.Count())
	}

	// AddAddresses should also dedup.
	ab.AddAddresses([]NetAddress{
		{IP: "1.2.3.4", Port: 9333},
		{IP: "5.6.7.8", Port: 9333},
		{IP: "5.6.7.8", Port: 9333},
	})

	if ab.Count() != 2 {
		t.Fatalf("expected 2 addresses, got %d", ab.Count())
	}
}

func TestPrivateAddressFiltered(t *testing.T) {
	tests := []struct {
		ip      string
		private bool
	}{
		{"127.0.0.1", true},
		{"10.0.0.1", true},
		{"10.255.255.255", true},
		{"172.16.0.1", true},
		{"172.31.255.255", true},
		{"192.168.0.1", true},
		{"192.168.1.100", true},
		{"0.0.0.0", true},
		{"8.8.8.8", false},
		{"1.2.3.4", false},
		{"172.15.0.1", false},  // just outside 172.16/12
		{"172.32.0.1", false},  // just outside 172.16/12
		{"192.169.0.1", false}, // not 192.168/16
	}

	for _, tt := range tests {
		got := isPrivateIP(tt.ip)
		if got != tt.private {
			t.Errorf("isPrivateIP(%q) = %v, want %v", tt.ip, got, tt.private)
		}
	}

	// Test that filterAddresses removes private IPs.
	cfg := ServerConfig{ListenAddr: ":0", Magic: TestNetMagic}
	s := NewServer(cfg)

	addrs := []NetAddress{
		{IP: "127.0.0.1", Port: 9333},
		{IP: "10.0.0.1", Port: 9333},
		{IP: "8.8.8.8", Port: 9333},
		{IP: "1.2.3.4", Port: 9333},
	}
	filtered := s.filterAddresses(addrs)
	if len(filtered) != 2 {
		t.Fatalf("expected 2 addresses after filtering, got %d", len(filtered))
	}
	for _, addr := range filtered {
		if isPrivateIP(addr.IP) {
			t.Errorf("private address %s not filtered", addr.IP)
		}
	}
}

func TestAutoConnect(t *testing.T) {
	// Create two servers.
	cfg1 := ServerConfig{ListenAddr: ":0", Magic: TestNetMagic}
	s1 := NewServer(cfg1)
	if err := s1.Start(); err != nil {
		t.Fatalf("s1 start: %v", err)
	}
	defer s1.Stop()

	cfg2 := ServerConfig{ListenAddr: ":0", Magic: TestNetMagic}
	s2 := NewServer(cfg2)
	if err := s2.Start(); err != nil {
		t.Fatalf("s2 start: %v", err)
	}
	defer s2.Stop()

	// Add s1's address to s2's address book.
	addr := s1.listener.Addr().(*net.TCPAddr)
	s2.addrBook.AddAddress(NetAddress{IP: addr.IP.String(), Port: uint16(addr.Port)})

	// Trigger auto-connect on s2.
	s2.autoConnect()

	// Wait briefly for connection to establish.
	time.Sleep(500 * time.Millisecond)

	// s2 should have 1 outbound peer.
	if s2.peers.Count() < 1 {
		t.Fatalf("expected at least 1 peer on s2, got %d", s2.peers.Count())
	}
}

func TestAddressBookFailureTracking(t *testing.T) {
	ab := NewAddressBook(nil)
	ab.AddAddress(NetAddress{IP: "1.2.3.4", Port: 9333})

	key := "1.2.3.4:9333"

	// First two failures should not remove.
	ab.RecordFailure(key, 3)
	if ab.Count() != 1 {
		t.Fatal("should still have 1 address after 1 failure")
	}
	ab.RecordFailure(key, 3)
	if ab.Count() != 1 {
		t.Fatal("should still have 1 address after 2 failures")
	}

	// Third failure should remove.
	removed := ab.RecordFailure(key, 3)
	if !removed {
		t.Fatal("expected address to be removed on 3rd failure")
	}
	if ab.Count() != 0 {
		t.Fatalf("expected 0 addresses, got %d", ab.Count())
	}

	// Re-adding should reset failure count.
	ab.AddAddress(NetAddress{IP: "1.2.3.4", Port: 9333})
	addrs := ab.GetGoodAddresses(10, 3)
	if len(addrs) != 1 {
		t.Fatalf("expected 1 good address, got %d", len(addrs))
	}
}

package network

import (
	"encoding/binary"
	"net"
	"testing"
	"time"

	"nous/crypto"
)

func TestGetAddrResponse(t *testing.T) {
	cfg := ServerConfig{
		ListenAddr: ":0",
		Magic:      TestNetMagic,
	}
	server := NewServer(cfg)

	// Add enough addresses so GetAddresses returns at least 1 (23% of n).
	server.addrMgr.AddAddresses([]NetAddress{
		{IP: "1.2.3.4", Port: 9333},
		{IP: "5.6.7.8", Port: 9333},
		{IP: "9.10.11.12", Port: 9333},
		{IP: "13.14.15.16", Port: 9333},
		{IP: "17.18.19.20", Port: 9333},
	}, net.ParseIP("99.99.99.99"))

	if err := server.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer server.Stop()

	conn, err := net.DialTimeout("tcp", server.ListenAddr(), 5*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Send version (v3).
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

	// Read server's version.
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

	// Drain the server's getaddr (sent in handleVerAck to v3 peers).
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	msg, err = DecodeMessage(conn, TestNetMagic)
	if err != nil {
		t.Fatalf("read after verack: %v", err)
	}
	if msg.Command() == CmdGetAddr {
		// Expected: server requests addresses from us.
	}

	// Send getaddr.
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
		t.Fatalf("expected MsgAddr, got %T (%s)", msg, msg.Command())
	}
	if len(addrMsg.Addresses) < 1 {
		t.Fatalf("expected at least 1 address, got %d", len(addrMsg.Addresses))
	}
}

func TestGetAddrOnlyOnce(t *testing.T) {
	cfg := ServerConfig{ListenAddr: ":0", Magic: TestNetMagic}
	server := NewServer(cfg)
	server.addrMgr.AddAddresses([]NetAddress{
		{IP: "1.2.3.4", Port: 9333},
	}, net.ParseIP("99.99.99.99"))

	if err := server.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer server.Stop()

	conn, err := net.DialTimeout("tcp", server.ListenAddr(), 5*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Handshake.
	ver := &MsgVersion{Version: ProtocolVersion, ListenPort: 9999, Nonce: 111, UserAgent: "test"}
	data, _ := EncodeMessage(TestNetMagic, ver)
	conn.Write(data)
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	DecodeMessage(conn, TestNetMagic) // verack
	DecodeMessage(conn, TestNetMagic) // version
	data, _ = EncodeMessage(TestNetMagic, &MsgVerAck{})
	conn.Write(data)
	DecodeMessage(conn, TestNetMagic) // getaddr from server

	// First getaddr should get a response.
	data, _ = EncodeMessage(TestNetMagic, &MsgGetAddr{})
	conn.Write(data)
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	msg, err := DecodeMessage(conn, TestNetMagic)
	if err != nil {
		t.Fatalf("first getaddr response: %v", err)
	}
	if _, ok := msg.(*MsgAddr); !ok {
		t.Fatalf("expected MsgAddr, got %s", msg.Command())
	}

	// Second getaddr should be silently ignored. Send a ping after to verify
	// the connection is still alive (we should get pong back, not another addr).
	data, _ = EncodeMessage(TestNetMagic, &MsgGetAddr{})
	conn.Write(data)
	data, _ = EncodeMessage(TestNetMagic, &MsgPing{Nonce: 42})
	conn.Write(data)
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	msg, err = DecodeMessage(conn, TestNetMagic)
	if err != nil {
		t.Fatalf("after second getaddr: %v", err)
	}
	if msg.Command() != CmdPong {
		t.Fatalf("expected pong after ignored getaddr, got %s", msg.Command())
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
		{"172.15.0.1", false},
		{"172.32.0.1", false},
		{"192.169.0.1", false},
	}

	for _, tt := range tests {
		got := isPrivateIP(tt.ip)
		if got != tt.private {
			t.Errorf("isPrivateIP(%q) = %v, want %v", tt.ip, got, tt.private)
		}
	}

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
}

func TestAutoConnect(t *testing.T) {
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

	// Connect directly (AddrManager rejects private IPs like 127.0.0.1,
	// so we can't add localhost via AddAddresses).
	s1Addr := s1.listener.Addr().String()
	if err := s2.Connect(s1Addr); err != nil {
		t.Fatalf("connect: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	if s2.peers.Count() < 1 {
		t.Fatalf("expected at least 1 peer on s2, got %d", s2.peers.Count())
	}
}

func TestUnknownMessageFallback(t *testing.T) {
	cfg := ServerConfig{ListenAddr: ":0", Magic: TestNetMagic}
	server := NewServer(cfg)
	if err := server.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer server.Stop()

	conn, err := net.DialTimeout("tcp", server.ListenAddr(), 5*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Handshake.
	ver := &MsgVersion{Version: ProtocolVersion, ListenPort: 9999, Nonce: 222, UserAgent: "test"}
	data, _ := EncodeMessage(TestNetMagic, ver)
	conn.Write(data)
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	DecodeMessage(conn, TestNetMagic) // verack
	DecodeMessage(conn, TestNetMagic) // version
	data, _ = EncodeMessage(TestNetMagic, &MsgVerAck{})
	conn.Write(data)
	DecodeMessage(conn, TestNetMagic) // getaddr from server

	// Send an unknown message type. Build a raw message with command "futureXYZ".
	// The payload is arbitrary; the server should read it, verify checksum, and discard.
	fakePayload := []byte("hello future")
	fakeMsg := buildRawMessage(TestNetMagic, "futureXYZ", fakePayload)
	conn.Write(fakeMsg)

	// Send a ping to verify the connection is still alive.
	data, _ = EncodeMessage(TestNetMagic, &MsgPing{Nonce: 77})
	conn.Write(data)

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	msg, err := DecodeMessage(conn, TestNetMagic)
	if err != nil {
		t.Fatalf("connection dead after unknown message: %v", err)
	}
	if msg.Command() != CmdPong {
		t.Fatalf("expected pong, got %s", msg.Command())
	}

	// Verify no ban score was added.
	score := server.protection.GetScore(conn.LocalAddr().String())
	if score > 0 {
		t.Fatalf("expected 0 ban score after unknown message, got %d", score)
	}
}

// buildRawMessage constructs a raw wire message with arbitrary command and payload.
func buildRawMessage(magic uint32, cmd string, payload []byte) []byte {
	var buf [24]byte
	binary.LittleEndian.PutUint32(buf[0:4], magic)
	copy(buf[4:16], cmd)
	binary.LittleEndian.PutUint32(buf[16:20], uint32(len(payload)))
	checksum := crypto.DoubleSha256(payload)
	copy(buf[20:24], checksum[:4])

	result := make([]byte, 24+len(payload))
	copy(result, buf[:])
	copy(result[24:], payload)
	return result
}

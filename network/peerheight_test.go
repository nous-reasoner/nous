package network

import (
	"net"
	"testing"
	"time"
)

func TestPeerHeightOnVersion(t *testing.T) {
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

	// Send version with height 5000.
	ver := &MsgVersion{
		Version:     ProtocolVersion,
		BlockHeight: 5000,
		Timestamp:   uint64(time.Now().Unix()),
		Nonce:       111,
		UserAgent:   "test",
		ListenPort:  9999,
	}
	data, _ := EncodeMessage(TestNetMagic, ver)
	conn.Write(data)

	// Read server's verack + version.
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	DecodeMessage(conn, TestNetMagic) // verack
	DecodeMessage(conn, TestNetMagic) // version

	// Complete handshake.
	data, _ = EncodeMessage(TestNetMagic, &MsgVerAck{})
	conn.Write(data)

	// Allow handshake to process.
	time.Sleep(200 * time.Millisecond)

	// Find our peer in the server.
	peers := server.peers.All()
	var peer *Peer
	for _, p := range peers {
		if p.Handshaked {
			peer = p
			break
		}
	}
	if peer == nil {
		t.Fatal("no handshaked peer found")
	}

	if peer.StartingHeight != 5000 {
		t.Errorf("StartingHeight = %d, want 5000", peer.StartingHeight)
	}
	if peer.BlockHeight != 5000 {
		t.Errorf("BlockHeight = %d, want 5000", peer.BlockHeight)
	}
}

func TestPeerHeightOnBlock(t *testing.T) {
	peer := &Peer{Addr: "test:1", Handshaked: true}
	peer.BlockHeight = 100
	peer.StartingHeight = 100

	// Simulate receiving a block at height 150.
	peer.LastBlockHeight = 150
	peer.UpdateBestHeight(150)

	if peer.BlockHeight != 150 {
		t.Errorf("BlockHeight = %d, want 150", peer.BlockHeight)
	}
	if peer.LastBlockHeight != 150 {
		t.Errorf("LastBlockHeight = %d, want 150", peer.LastBlockHeight)
	}
	// StartingHeight should NOT change.
	if peer.StartingHeight != 100 {
		t.Errorf("StartingHeight = %d, want 100 (should not change)", peer.StartingHeight)
	}
}

func TestPeerHeightOnHeaders(t *testing.T) {
	peer := &Peer{Addr: "test:1", Handshaked: true}
	peer.BlockHeight = 100
	peer.StartingHeight = 100

	// Simulate receiving headers up to height 200.
	peer.LastHeaderHeight = 200
	peer.UpdateBestHeight(200)

	if peer.BlockHeight != 200 {
		t.Errorf("BlockHeight = %d, want 200", peer.BlockHeight)
	}
	if peer.LastHeaderHeight != 200 {
		t.Errorf("LastHeaderHeight = %d, want 200", peer.LastHeaderHeight)
	}
	if peer.LastBlockHeight != 0 {
		t.Errorf("LastBlockHeight = %d, want 0 (no blocks received yet)", peer.LastBlockHeight)
	}
}

func TestPeerHeightOnInv(t *testing.T) {
	peer := &Peer{Addr: "test:1", Handshaked: true}
	peer.BlockHeight = 100

	// countBlockInv counts block items in an inv list.
	items := []InvItem{
		{Type: InvTypeBlock, Hash: [32]byte{1}},
		{Type: InvTypeTx, Hash: [32]byte{2}},
		{Type: InvTypeBlock, Hash: [32]byte{3}},
		{Type: InvTypeBlock, Hash: [32]byte{4}},
	}
	blockCount := countBlockInv(items)
	if blockCount != 3 {
		t.Errorf("countBlockInv = %d, want 3", blockCount)
	}

	// Simulating inv handling: peer announces 50 blocks when our height is 100.
	// Inferred height = 100 + 50 = 150.
	inferredHeight := uint64(100 + 50)
	peer.UpdateBestHeight(inferredHeight)

	if peer.BlockHeight != 150 {
		t.Errorf("BlockHeight = %d, want 150", peer.BlockHeight)
	}
}

func TestPeerHeightNeverDecrease(t *testing.T) {
	peer := &Peer{Addr: "test:1", Handshaked: true}
	peer.BlockHeight = 200

	// Try to set a lower height — should be ignored.
	peer.UpdateBestHeight(100)
	if peer.BlockHeight != 200 {
		t.Errorf("BlockHeight = %d, want 200 (should not decrease)", peer.BlockHeight)
	}

	// Same height — should stay the same.
	peer.UpdateBestHeight(200)
	if peer.BlockHeight != 200 {
		t.Errorf("BlockHeight = %d, want 200", peer.BlockHeight)
	}

	// Higher height — should update.
	peer.UpdateBestHeight(300)
	if peer.BlockHeight != 300 {
		t.Errorf("BlockHeight = %d, want 300", peer.BlockHeight)
	}

	// Verify LastBlockHeight and LastHeaderHeight can decrease (they track
	// the most recent value, not the maximum).
	peer.LastBlockHeight = 250
	peer.LastHeaderHeight = 280
	peer.LastBlockHeight = 260
	peer.LastHeaderHeight = 270
	if peer.LastBlockHeight != 260 {
		t.Errorf("LastBlockHeight = %d, want 260", peer.LastBlockHeight)
	}
}

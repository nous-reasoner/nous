package network

import (
	"bytes"
	"fmt"
	"sync"
	"testing"
	"time"

	"nous/block"
	"nous/crypto"
	"nous/tx"
)

// ============================================================
// 1. Message serialize/deserialize round-trip
// ============================================================

func TestEncodeDecodeRoundTrip(t *testing.T) {
	messages := []Message{
		&MsgVersion{
			Version:     1,
			BlockHeight: 12345,
			Timestamp:   1700000000,
			Nonce:       42,
			UserAgent:   "test/1.0",
			ListenPort:  9333,
		},
		&MsgVerAck{},
		&MsgGetBlocks{
			StartHash: crypto.Sha256([]byte("start")),
			StopHash:  crypto.Hash{},
		},
		&MsgInv{Items: []InvItem{
			{Type: InvTypeBlock, Hash: crypto.Sha256([]byte("block1"))},
			{Type: InvTypeTx, Hash: crypto.Sha256([]byte("tx1"))},
		}},
		&MsgGetData{Items: []InvItem{
			{Type: InvTypeBlock, Hash: crypto.Sha256([]byte("block2"))},
		}},
		&MsgBlock{Payload: []byte{0x01, 0x02, 0x03, 0x04}},
		&MsgTx{Payload: []byte{0x05, 0x06}},
		&MsgPing{Nonce: 9999},
		&MsgPong{Nonce: 9999},
		&MsgAddr{Addresses: []NetAddress{
			{IP: "127.0.0.1", Port: 9333},
			{IP: "192.168.1.1", Port: 9334},
		}},
	}

	for _, msg := range messages {
		data, err := EncodeMessage(MainNetMagic, msg)
		if err != nil {
			t.Fatalf("EncodeMessage(%s): %v", msg.Command(), err)
		}

		reader := bytes.NewReader(data)
		decoded, err := DecodeMessage(reader, MainNetMagic)
		if err != nil {
			t.Fatalf("DecodeMessage(%s): %v", msg.Command(), err)
		}

		if decoded.Command() != msg.Command() {
			t.Fatalf("command mismatch: want %s, got %s", msg.Command(), decoded.Command())
		}
	}
}

// ============================================================
// 2. Per-type encode/decode correctness
// ============================================================

func TestVersionMessageFields(t *testing.T) {
	orig := &MsgVersion{
		Version:     1,
		BlockHeight: 50000,
		Timestamp:   1700000000,
		Nonce:       12345678,
		UserAgent:   "nous/0.1.0",
		ListenPort:  9333,
	}

	data, _ := EncodeMessage(MainNetMagic, orig)
	decoded, err := DecodeMessage(bytes.NewReader(data), MainNetMagic)
	if err != nil {
		t.Fatal(err)
	}

	ver := decoded.(*MsgVersion)
	if ver.Version != orig.Version {
		t.Fatalf("Version: want %d, got %d", orig.Version, ver.Version)
	}
	if ver.BlockHeight != orig.BlockHeight {
		t.Fatalf("BlockHeight: want %d, got %d", orig.BlockHeight, ver.BlockHeight)
	}
	if ver.Timestamp != orig.Timestamp {
		t.Fatalf("Timestamp: want %d, got %d", orig.Timestamp, ver.Timestamp)
	}
	if ver.Nonce != orig.Nonce {
		t.Fatalf("Nonce: want %d, got %d", orig.Nonce, ver.Nonce)
	}
	if ver.UserAgent != orig.UserAgent {
		t.Fatalf("UserAgent: want %q, got %q", orig.UserAgent, ver.UserAgent)
	}
	if ver.ListenPort != orig.ListenPort {
		t.Fatalf("ListenPort: want %d, got %d", orig.ListenPort, ver.ListenPort)
	}
}

func TestPingPongFields(t *testing.T) {
	ping := &MsgPing{Nonce: 0xDEADBEEF}
	data, _ := EncodeMessage(MainNetMagic, ping)
	decoded, _ := DecodeMessage(bytes.NewReader(data), MainNetMagic)
	got := decoded.(*MsgPing)
	if got.Nonce != ping.Nonce {
		t.Fatalf("ping nonce: want %d, got %d", ping.Nonce, got.Nonce)
	}

	pong := &MsgPong{Nonce: 0xCAFEBABE}
	data, _ = EncodeMessage(MainNetMagic, pong)
	decoded, _ = DecodeMessage(bytes.NewReader(data), MainNetMagic)
	gotPong := decoded.(*MsgPong)
	if gotPong.Nonce != pong.Nonce {
		t.Fatalf("pong nonce: want %d, got %d", pong.Nonce, gotPong.Nonce)
	}
}

func TestInvMessageFields(t *testing.T) {
	orig := &MsgInv{Items: []InvItem{
		{Type: InvTypeBlock, Hash: crypto.Sha256([]byte("b1"))},
		{Type: InvTypeTx, Hash: crypto.Sha256([]byte("t1"))},
		{Type: InvTypeBlock, Hash: crypto.Sha256([]byte("b2"))},
	}}

	data, _ := EncodeMessage(MainNetMagic, orig)
	decoded, _ := DecodeMessage(bytes.NewReader(data), MainNetMagic)
	inv := decoded.(*MsgInv)

	if len(inv.Items) != len(orig.Items) {
		t.Fatalf("inv items: want %d, got %d", len(orig.Items), len(inv.Items))
	}
	for i := range orig.Items {
		if inv.Items[i].Type != orig.Items[i].Type {
			t.Fatalf("item %d type mismatch", i)
		}
		if inv.Items[i].Hash != orig.Items[i].Hash {
			t.Fatalf("item %d hash mismatch", i)
		}
	}
}

func TestAddrMessageFields(t *testing.T) {
	orig := &MsgAddr{Addresses: []NetAddress{
		{IP: "10.0.0.1", Port: 9333},
		{IP: "10.0.0.2", Port: 9334},
	}}

	data, _ := EncodeMessage(MainNetMagic, orig)
	decoded, _ := DecodeMessage(bytes.NewReader(data), MainNetMagic)
	addr := decoded.(*MsgAddr)

	if len(addr.Addresses) != 2 {
		t.Fatalf("addr count: want 2, got %d", len(addr.Addresses))
	}
	if addr.Addresses[0].IP != "10.0.0.1" || addr.Addresses[0].Port != 9333 {
		t.Fatalf("addr[0] mismatch: %+v", addr.Addresses[0])
	}
	if addr.Addresses[1].IP != "10.0.0.2" || addr.Addresses[1].Port != 9334 {
		t.Fatalf("addr[1] mismatch: %+v", addr.Addresses[1])
	}
}

func TestGetBlocksFields(t *testing.T) {
	start := crypto.Sha256([]byte("start hash"))
	stop := crypto.Sha256([]byte("stop hash"))
	orig := &MsgGetBlocks{StartHash: start, StopHash: stop}

	data, _ := EncodeMessage(MainNetMagic, orig)
	decoded, _ := DecodeMessage(bytes.NewReader(data), MainNetMagic)
	gb := decoded.(*MsgGetBlocks)

	if gb.StartHash != start {
		t.Fatal("StartHash mismatch")
	}
	if gb.StopHash != stop {
		t.Fatal("StopHash mismatch")
	}
}

func TestBlockTxPayload(t *testing.T) {
	payload := []byte{0xDE, 0xAD, 0xBE, 0xEF, 0x01, 0x02}
	orig := &MsgBlock{Payload: payload}
	data, _ := EncodeMessage(MainNetMagic, orig)
	decoded, _ := DecodeMessage(bytes.NewReader(data), MainNetMagic)
	blk := decoded.(*MsgBlock)
	if !bytes.Equal(blk.Payload, payload) {
		t.Fatal("block payload mismatch")
	}

	txPayload := []byte{0xCA, 0xFE}
	origTx := &MsgTx{Payload: txPayload}
	data, _ = EncodeMessage(MainNetMagic, origTx)
	decoded, _ = DecodeMessage(bytes.NewReader(data), MainNetMagic)
	txMsg := decoded.(*MsgTx)
	if !bytes.Equal(txMsg.Payload, txPayload) {
		t.Fatal("tx payload mismatch")
	}
}

// ============================================================
// 3. Bad magic rejected
// ============================================================

func TestDecodeRejectsBadMagic(t *testing.T) {
	data, _ := EncodeMessage(MainNetMagic, &MsgPing{Nonce: 1})
	_, err := DecodeMessage(bytes.NewReader(data), TestNetMagic)
	if err == nil {
		t.Fatal("should reject mismatched magic")
	}
}

// ============================================================
// 4. Checksum verification
// ============================================================

func TestDecodeRejectsBadChecksum(t *testing.T) {
	data, _ := EncodeMessage(MainNetMagic, &MsgPing{Nonce: 42})
	// Corrupt the checksum (bytes 20-23).
	data[20] ^= 0xFF
	_, err := DecodeMessage(bytes.NewReader(data), MainNetMagic)
	if err == nil {
		t.Fatal("should reject corrupted checksum")
	}
}

// ============================================================
// 5. Mempool: add, query, fee-rate sort, remove
// ============================================================

func TestMempoolAddGetRemove(t *testing.T) {
	mp := NewMempool()

	t1 := &tx.Transaction{
		Version: 1,
		Inputs:  []tx.TxIn{{PrevOut: tx.OutPoint{TxID: crypto.Sha256([]byte("in1")), Index: 0}, Sequence: 0xFFFFFFFF}},
		Outputs: []tx.TxOut{{Amount: 100, PkScript: []byte{0x76}}},
	}
	t2 := &tx.Transaction{
		Version: 1,
		Inputs:  []tx.TxIn{{PrevOut: tx.OutPoint{TxID: crypto.Sha256([]byte("in2")), Index: 0}, Sequence: 0xFFFFFFFF}},
		Outputs: []tx.TxOut{{Amount: 200, PkScript: []byte{0x76}}},
	}

	// Add.
	if !mp.Add(t1) {
		t.Fatal("first add should succeed")
	}
	if !mp.Add(t2) {
		t.Fatal("second add should succeed")
	}
	if mp.Count() != 2 {
		t.Fatalf("count: want 2, got %d", mp.Count())
	}

	// Duplicate add.
	if mp.Add(t1) {
		t.Fatal("duplicate add should return false")
	}

	// Get.
	got := mp.Get(t1.TxID())
	if got == nil {
		t.Fatal("Get should return the transaction")
	}

	// Has.
	if !mp.Has(t1.TxID()) {
		t.Fatal("Has should return true")
	}

	// Remove.
	mp.Remove(t1.TxID())
	if mp.Count() != 1 {
		t.Fatalf("count after remove: want 1, got %d", mp.Count())
	}
	if mp.Has(t1.TxID()) {
		t.Fatal("removed tx should not be present")
	}
}

func TestMempoolFeeRateSort(t *testing.T) {
	mp := NewMempool()

	// Add 3 transactions with different fees.
	t1 := &tx.Transaction{
		Version: 1,
		Inputs:  []tx.TxIn{{PrevOut: tx.OutPoint{TxID: crypto.Sha256([]byte("low")), Index: 0}, Sequence: 0xFFFFFFFF}},
		Outputs: []tx.TxOut{{Amount: 100, PkScript: []byte{0x76}}},
	}
	t2 := &tx.Transaction{
		Version: 1,
		Inputs:  []tx.TxIn{{PrevOut: tx.OutPoint{TxID: crypto.Sha256([]byte("high")), Index: 0}, Sequence: 0xFFFFFFFF}},
		Outputs: []tx.TxOut{{Amount: 50, PkScript: []byte{0x76}}},
	}
	t3 := &tx.Transaction{
		Version: 1,
		Inputs:  []tx.TxIn{{PrevOut: tx.OutPoint{TxID: crypto.Sha256([]byte("mid")), Index: 0}, Sequence: 0xFFFFFFFF}},
		Outputs: []tx.TxOut{{Amount: 75, PkScript: []byte{0x76}}},
	}

	mp.AddWithFee(t1, 10)   // low fee
	mp.AddWithFee(t2, 1000) // high fee
	mp.AddWithFee(t3, 500)  // medium fee

	sorted := mp.GetByFeeRate()
	if len(sorted) != 3 {
		t.Fatalf("sorted: want 3, got %d", len(sorted))
	}

	// Highest fee rate first.
	if sorted[0].TxID != t2.TxID() {
		t.Fatal("first should be highest fee")
	}
	if sorted[1].TxID != t3.TxID() {
		t.Fatal("second should be medium fee")
	}
	if sorted[2].TxID != t1.TxID() {
		t.Fatal("third should be lowest fee")
	}
}

func TestMempoolRemoveConfirmed(t *testing.T) {
	mp := NewMempool()

	t1 := &tx.Transaction{
		Version: 1,
		Inputs:  []tx.TxIn{{PrevOut: tx.OutPoint{TxID: crypto.Sha256([]byte("c1")), Index: 0}, Sequence: 0xFFFFFFFF}},
		Outputs: []tx.TxOut{{Amount: 100, PkScript: []byte{0x76}}},
	}
	t2 := &tx.Transaction{
		Version: 1,
		Inputs:  []tx.TxIn{{PrevOut: tx.OutPoint{TxID: crypto.Sha256([]byte("c2")), Index: 0}, Sequence: 0xFFFFFFFF}},
		Outputs: []tx.TxOut{{Amount: 200, PkScript: []byte{0x76}}},
	}

	mp.Add(t1)
	mp.Add(t2)

	// Simulate block confirming t1.
	mp.RemoveConfirmed([]*tx.Transaction{t1})

	if mp.Has(t1.TxID()) {
		t.Fatal("confirmed tx should be removed")
	}
	if !mp.Has(t2.TxID()) {
		t.Fatal("unconfirmed tx should remain")
	}
}

// ============================================================
// 6. PeerManager: add, remove, best peer
// ============================================================

func TestPeerManagerAddRemove(t *testing.T) {
	pm := NewPeerManager()

	p1 := &Peer{Addr: "10.0.0.1:9333", Inbound: false, LastActive: time.Now()}
	p2 := &Peer{Addr: "10.0.0.2:9333", Inbound: true, LastActive: time.Now()}

	if !pm.Add(p1) {
		t.Fatal("first add should succeed")
	}
	if !pm.Add(p2) {
		t.Fatal("second add should succeed")
	}
	if pm.Count() != 2 {
		t.Fatalf("count: want 2, got %d", pm.Count())
	}

	got := pm.Get("10.0.0.1:9333")
	if got != p1 {
		t.Fatal("Get should return the peer")
	}

	pm.Remove("10.0.0.1:9333")
	if pm.Count() != 1 {
		t.Fatalf("count after remove: want 1, got %d", pm.Count())
	}
	if pm.Get("10.0.0.1:9333") != nil {
		t.Fatal("removed peer should not be present")
	}
}

func TestPeerManagerBestPeer(t *testing.T) {
	pm := NewPeerManager()

	p1 := &Peer{Addr: "a:1", BlockHeight: 100, Handshaked: true, LastActive: time.Now()}
	p2 := &Peer{Addr: "b:1", BlockHeight: 500, Handshaked: true, LastActive: time.Now()}
	p3 := &Peer{Addr: "c:1", BlockHeight: 300, Handshaked: true, LastActive: time.Now()}
	p4 := &Peer{Addr: "d:1", BlockHeight: 900, Handshaked: false, LastActive: time.Now()} // not handshaked

	pm.Add(p1)
	pm.Add(p2)
	pm.Add(p3)
	pm.Add(p4)

	best := pm.BestPeer()
	if best != p2 {
		t.Fatalf("best peer should have height 500, got %d", best.BlockHeight)
	}
}

func TestPeerManagerConnectionLimits(t *testing.T) {
	pm := NewPeerManager()

	// Add max outbound.
	for i := 0; i < MaxOutbound; i++ {
		p := &Peer{Addr: fmt.Sprintf("out:%d", i), Inbound: false, LastActive: time.Now()}
		if !pm.Add(p) {
			t.Fatalf("outbound %d should be allowed", i)
		}
	}

	// Next outbound should be rejected.
	extra := &Peer{Addr: "out:extra", Inbound: false, LastActive: time.Now()}
	if pm.Add(extra) {
		t.Fatal("should reject outbound beyond limit")
	}

	// Inbound should still work.
	inbound := &Peer{Addr: "in:0", Inbound: true, LastActive: time.Now()}
	if !pm.Add(inbound) {
		t.Fatal("inbound should still be allowed")
	}
}

// ============================================================
// 7. Two nodes TCP handshake
// ============================================================

func TestTCPHandshake(t *testing.T) {
	// Server A.
	configA := ServerConfig{
		ListenAddr: "127.0.0.1:0", // ephemeral port
		Magic:      TestNetMagic,
	}
	serverA := NewServer(configA)
	if err := serverA.Start(); err != nil {
		t.Fatal(err)
	}
	defer serverA.Stop()

	// Server B.
	configB := ServerConfig{
		ListenAddr: "127.0.0.1:0",
		Magic:      TestNetMagic,
	}
	serverB := NewServer(configB)
	if err := serverB.Start(); err != nil {
		t.Fatal(err)
	}
	defer serverB.Stop()

	// B connects to A.
	addrA := serverA.ListenAddr()
	if err := serverB.Connect(addrA); err != nil {
		t.Fatalf("connect failed: %v", err)
	}

	// Wait for handshake.
	time.Sleep(200 * time.Millisecond)

	// A should have 1 inbound peer.
	if serverA.Peers().Count() < 1 {
		t.Fatalf("A should have at least 1 peer, got %d", serverA.Peers().Count())
	}

	// B should have 1 outbound peer.
	if serverB.Peers().Count() < 1 {
		t.Fatalf("B should have at least 1 peer, got %d", serverB.Peers().Count())
	}
}

// ============================================================
// 8. Block broadcast: A sends, B receives
// ============================================================

func TestBlockBroadcast(t *testing.T) {
	received := make(chan []byte, 1)

	// Server A.
	configA := ServerConfig{
		ListenAddr: "127.0.0.1:0",
		Magic:      TestNetMagic,
	}
	serverA := NewServer(configA)
	if err := serverA.Start(); err != nil {
		t.Fatal(err)
	}
	defer serverA.Stop()

	// Server B with custom block handler.
	configB := ServerConfig{
		ListenAddr: "127.0.0.1:0",
		Magic:      TestNetMagic,
	}
	serverB := NewServer(configB)
	serverB.OnMessage(CmdBlock, func(peer *Peer, msg Message) {
		blkMsg := msg.(*MsgBlock)
		received <- blkMsg.Payload
	})
	if err := serverB.Start(); err != nil {
		t.Fatal(err)
	}
	defer serverB.Stop()

	// B connects to A.
	if err := serverB.Connect(serverA.ListenAddr()); err != nil {
		t.Fatal(err)
	}

	// Wait for handshake.
	time.Sleep(200 * time.Millisecond)

	// A broadcasts a block.
	blockData := []byte{0xBE, 0xEF, 0xCA, 0xFE, 0x01, 0x02, 0x03}
	serverA.BroadcastMessage(&MsgBlock{Payload: blockData})

	// B should receive it.
	select {
	case got := <-received:
		if !bytes.Equal(got, blockData) {
			t.Fatalf("block payload mismatch: want %x, got %x", blockData, got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for block broadcast")
	}
}

// ============================================================
// mockChain implements ChainAccess for sync tests.
// ============================================================

type mockChain struct {
	mu     sync.RWMutex
	blocks []*block.Block
	hashes []crypto.Hash
}

func newMockChain(genesis *block.Block) *mockChain {
	h := genesis.Header.Hash()
	return &mockChain{
		blocks: []*block.Block{genesis},
		hashes: []crypto.Hash{h},
	}
}

func (mc *mockChain) Height() uint64 {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	return uint64(len(mc.blocks) - 1)
}

func (mc *mockChain) TipHash() crypto.Hash {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	return mc.hashes[len(mc.hashes)-1]
}

func (mc *mockChain) HasBlock(hash crypto.Hash) bool {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	for _, h := range mc.hashes {
		if h == hash {
			return true
		}
	}
	return false
}

func (mc *mockChain) GetBlockByHeight(height uint64) (*block.Block, error) {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	if int(height) >= len(mc.blocks) {
		return nil, fmt.Errorf("block %d not found", height)
	}
	return mc.blocks[height], nil
}

func (mc *mockChain) GetBlockHashByHeight(height uint64) (crypto.Hash, error) {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	if int(height) >= len(mc.hashes) {
		return crypto.Hash{}, fmt.Errorf("block %d not found", height)
	}
	return mc.hashes[height], nil
}

func (mc *mockChain) GetBlockByHash(hash crypto.Hash) (*block.Block, error) {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	for i, h := range mc.hashes {
		if h == hash {
			return mc.blocks[i], nil
		}
	}
	return nil, fmt.Errorf("block %x not found", hash[:8])
}

func (mc *mockChain) AddBlock(blk *block.Block) (uint64, error) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	h := blk.Header.Hash()
	// Check prev hash links to our tip.
	if blk.Header.PrevBlockHash != mc.hashes[len(mc.hashes)-1] {
		return 0, fmt.Errorf("prev hash mismatch")
	}
	mc.blocks = append(mc.blocks, blk)
	mc.hashes = append(mc.hashes, h)
	return uint64(len(mc.blocks) - 1), nil
}

// makeTestBlock creates a simple block linked to the given prev hash.
func makeTestBlock(prevHash crypto.Hash, height uint32) *block.Block {
	coinbase := tx.NewCoinbaseTx(uint64(height), 50_0000_0000, tx.CreateP2PKHLockScript(make([]byte, 20)), tx.ChainIDNous)
	txIDs := []crypto.Hash{coinbase.TxID()}
	merkle := block.ComputeMerkleRoot(txIDs)

	return &block.Block{
		Header: block.Header{
			Version:       1,
			PrevBlockHash: prevHash,
			MerkleRoot:    merkle,
			Timestamp:     1735689600 + height*600,
		},
		Transactions: []*tx.Transaction{coinbase},
	}
}

// ============================================================
// 9. Block sync: A has 5 blocks, B syncs from genesis to 5
// ============================================================

func TestBlockSync(t *testing.T) {
	// Build a genesis block.
	genesis := makeTestBlock(crypto.Hash{}, 0)

	// Chain A: genesis + 5 blocks.
	chainA := newMockChain(genesis)
	for h := uint32(1); h <= 5; h++ {
		tip := chainA.TipHash()
		blk := makeTestBlock(tip, h)
		if _, err := chainA.AddBlock(blk); err != nil {
			t.Fatalf("chainA add block %d: %v", h, err)
		}
	}

	// Chain B: genesis only.
	chainB := newMockChain(genesis)

	// Server A.
	serverA := NewServer(ServerConfig{ListenAddr: "127.0.0.1:0", Magic: TestNetMagic})
	serverA.SetBlockHeight(chainA.Height())
	if err := serverA.Start(); err != nil {
		t.Fatal(err)
	}
	defer serverA.Stop()

	// Server B.
	serverB := NewServer(ServerConfig{ListenAddr: "127.0.0.1:0", Magic: TestNetMagic})
	serverB.SetBlockHeight(chainB.Height())
	if err := serverB.Start(); err != nil {
		t.Fatal(err)
	}
	defer serverB.Stop()

	// Attach syncers.
	syncerA := NewBlockSyncer(serverA, chainA)
	syncerA.Start()
	syncerB := NewBlockSyncer(serverB, chainB)
	syncerB.Start()

	// B connects to A.
	if err := serverB.Connect(serverA.ListenAddr()); err != nil {
		t.Fatalf("connect: %v", err)
	}

	// Wait for handshake.
	time.Sleep(300 * time.Millisecond)

	// Trigger sync on B.
	syncerB.TriggerSync()

	// Wait for B to reach height 5.
	if err := syncerB.WaitForSync(5, 5*time.Second); err != nil {
		t.Fatalf("sync failed: %v (chainB height=%d)", err, chainB.Height())
	}

	if chainB.Height() != 5 {
		t.Fatalf("chainB height: want 5, got %d", chainB.Height())
	}

	// Verify tip hashes match.
	if chainA.TipHash() != chainB.TipHash() {
		t.Fatal("tip hash mismatch after sync")
	}
}

// ============================================================
// 10. Block broadcast: A mines, B and C both receive
// ============================================================

func TestNewBlockBroadcast(t *testing.T) {
	genesis := makeTestBlock(crypto.Hash{}, 0)

	chainA := newMockChain(genesis)
	chainB := newMockChain(genesis)
	chainC := newMockChain(genesis)

	// Create three servers.
	serverA := NewServer(ServerConfig{ListenAddr: "127.0.0.1:0", Magic: TestNetMagic})
	if err := serverA.Start(); err != nil {
		t.Fatal(err)
	}
	defer serverA.Stop()

	serverB := NewServer(ServerConfig{ListenAddr: "127.0.0.1:0", Magic: TestNetMagic})
	if err := serverB.Start(); err != nil {
		t.Fatal(err)
	}
	defer serverB.Stop()

	serverC := NewServer(ServerConfig{ListenAddr: "127.0.0.1:0", Magic: TestNetMagic})
	if err := serverC.Start(); err != nil {
		t.Fatal(err)
	}
	defer serverC.Stop()

	// Attach syncers.
	syncerA := NewBlockSyncer(serverA, chainA)
	syncerA.Start()
	syncerB := NewBlockSyncer(serverB, chainB)
	syncerB.Start()
	syncerC := NewBlockSyncer(serverC, chainC)
	syncerC.Start()

	// B and C connect to A.
	if err := serverB.Connect(serverA.ListenAddr()); err != nil {
		t.Fatal(err)
	}
	if err := serverC.Connect(serverA.ListenAddr()); err != nil {
		t.Fatal(err)
	}

	// Wait for handshakes.
	time.Sleep(300 * time.Millisecond)

	// A mines a new block.
	newBlock := makeTestBlock(chainA.TipHash(), 1)
	if _, err := chainA.AddBlock(newBlock); err != nil {
		t.Fatalf("chainA add block: %v", err)
	}
	serverA.SetBlockHeight(1)

	// A broadcasts the block.
	if err := syncerA.BroadcastBlock(newBlock); err != nil {
		t.Fatalf("broadcast: %v", err)
	}

	// Wait for B and C to receive and process the block.
	if err := syncerB.WaitForSync(1, 3*time.Second); err != nil {
		t.Fatalf("B did not receive block: %v (height=%d)", err, chainB.Height())
	}
	if err := syncerC.WaitForSync(1, 3*time.Second); err != nil {
		t.Fatalf("C did not receive block: %v (height=%d)", err, chainC.Height())
	}

	// All three chains should have the same tip.
	if chainA.TipHash() != chainB.TipHash() {
		t.Fatal("A-B tip hash mismatch")
	}
	if chainA.TipHash() != chainC.TipHash() {
		t.Fatal("A-C tip hash mismatch")
	}
}

// ============================================================
// 11. Mempool rejects transactions beyond MaxMempoolTxCount
// ============================================================

func TestMempoolSizeLimit(t *testing.T) {
	mp := NewMempool()

	// Fill to the limit.
	for i := 0; i < MaxMempoolTxCount; i++ {
		txn := &tx.Transaction{
			Version: 1,
			Inputs:  []tx.TxIn{{PrevOut: tx.OutPoint{TxID: crypto.Sha256([]byte(fmt.Sprintf("tx-%d", i))), Index: 0}, Sequence: 0xFFFFFFFF}},
			Outputs: []tx.TxOut{{Amount: 100, PkScript: []byte{0x76}}},
		}
		if !mp.Add(txn) {
			t.Fatalf("add tx %d should succeed", i)
		}
	}

	if mp.Count() != MaxMempoolTxCount {
		t.Fatalf("count: want %d, got %d", MaxMempoolTxCount, mp.Count())
	}

	// Next add should be rejected.
	overflow := &tx.Transaction{
		Version: 1,
		Inputs:  []tx.TxIn{{PrevOut: tx.OutPoint{TxID: crypto.Sha256([]byte("overflow")), Index: 0}, Sequence: 0xFFFFFFFF}},
		Outputs: []tx.TxOut{{Amount: 100, PkScript: []byte{0x76}}},
	}
	if mp.Add(overflow) {
		t.Fatal("mempool should reject when full")
	}
}

// ============================================================
// 11b. Mempool double-spend protection
// ============================================================

func TestMempoolDoubleSpendRejected(t *testing.T) {
	mp := NewMempool()

	// Shared UTXO that both transactions try to spend.
	sharedOutPoint := tx.OutPoint{TxID: crypto.Sha256([]byte("shared-utxo")), Index: 0}

	txA := &tx.Transaction{
		Version: 1,
		Inputs:  []tx.TxIn{{PrevOut: sharedOutPoint, Sequence: 0xFFFFFFFF}},
		Outputs: []tx.TxOut{{Amount: 100, PkScript: []byte{0xaa}}},
	}
	txB := &tx.Transaction{
		Version: 1,
		Inputs:  []tx.TxIn{{PrevOut: sharedOutPoint, Sequence: 0xFFFFFFFF}},
		Outputs: []tx.TxOut{{Amount: 50, PkScript: []byte{0xbb}}}, // different outputs → different TxID
	}

	// First tx should succeed.
	if !mp.Add(txA) {
		t.Fatal("txA should be accepted")
	}

	// Second tx spending the same UTXO should be rejected.
	if mp.Add(txB) {
		t.Fatal("txB should be rejected (double-spend)")
	}
	if mp.Count() != 1 {
		t.Fatalf("count: want 1, got %d", mp.Count())
	}
}

func TestMempoolDoubleSpendAddWithFee(t *testing.T) {
	mp := NewMempool()

	sharedOutPoint := tx.OutPoint{TxID: crypto.Sha256([]byte("shared-fee")), Index: 0}

	txA := &tx.Transaction{
		Version: 1,
		Inputs:  []tx.TxIn{{PrevOut: sharedOutPoint, Sequence: 0xFFFFFFFF}},
		Outputs: []tx.TxOut{{Amount: 100, PkScript: []byte{0xaa}}},
	}
	txB := &tx.Transaction{
		Version: 1,
		Inputs:  []tx.TxIn{{PrevOut: sharedOutPoint, Sequence: 0xFFFFFFFF}},
		Outputs: []tx.TxOut{{Amount: 50, PkScript: []byte{0xbb}}},
	}

	if !mp.AddWithFee(txA, 100) {
		t.Fatal("txA should be accepted")
	}
	if mp.AddWithFee(txB, 200) {
		t.Fatal("txB should be rejected (double-spend via AddWithFee)")
	}
}

func TestMempoolSpentSetClearedOnRemove(t *testing.T) {
	mp := NewMempool()

	sharedOutPoint := tx.OutPoint{TxID: crypto.Sha256([]byte("removable")), Index: 0}

	txA := &tx.Transaction{
		Version: 1,
		Inputs:  []tx.TxIn{{PrevOut: sharedOutPoint, Sequence: 0xFFFFFFFF}},
		Outputs: []tx.TxOut{{Amount: 100, PkScript: []byte{0xaa}}},
	}
	txB := &tx.Transaction{
		Version: 1,
		Inputs:  []tx.TxIn{{PrevOut: sharedOutPoint, Sequence: 0xFFFFFFFF}},
		Outputs: []tx.TxOut{{Amount: 50, PkScript: []byte{0xbb}}},
	}

	mp.Add(txA)

	// txB rejected while txA is in pool.
	if mp.Add(txB) {
		t.Fatal("txB should be rejected while txA is in pool")
	}

	// Remove txA → spent set should be cleaned.
	mp.Remove(txA.TxID())

	// Now txB should succeed.
	if !mp.Add(txB) {
		t.Fatal("txB should be accepted after txA is removed")
	}
}

func TestMempoolSpentSetClearedOnConfirm(t *testing.T) {
	mp := NewMempool()

	utxoX := tx.OutPoint{TxID: crypto.Sha256([]byte("utxo-x")), Index: 0}
	utxoY := tx.OutPoint{TxID: crypto.Sha256([]byte("utxo-y")), Index: 0}

	// txPool spends utxoX (in mempool).
	txPool := &tx.Transaction{
		Version: 1,
		Inputs:  []tx.TxIn{{PrevOut: utxoX, Sequence: 0xFFFFFFFF}},
		Outputs: []tx.TxOut{{Amount: 100, PkScript: []byte{0xaa}}},
	}
	mp.Add(txPool)

	// Block confirms a DIFFERENT tx spending utxoX.
	txBlock := &tx.Transaction{
		Version: 1,
		Inputs:  []tx.TxIn{{PrevOut: utxoX, Sequence: 0xFFFFFFFF}},
		Outputs: []tx.TxOut{{Amount: 90, PkScript: []byte{0xcc}}},
	}
	mp.RemoveConfirmed([]*tx.Transaction{txBlock})

	// txPool should be evicted (it conflicts with the confirmed block).
	if mp.Has(txPool.TxID()) {
		t.Fatal("txPool should be evicted — its input was confirmed in a block")
	}
	if mp.Count() != 0 {
		t.Fatalf("pool should be empty, got %d", mp.Count())
	}

	// Now a new tx spending utxoY should work fine (spent set is clean).
	txNew := &tx.Transaction{
		Version: 1,
		Inputs:  []tx.TxIn{{PrevOut: utxoY, Sequence: 0xFFFFFFFF}},
		Outputs: []tx.TxOut{{Amount: 50, PkScript: []byte{0xdd}}},
	}
	if !mp.Add(txNew) {
		t.Fatal("txNew should succeed")
	}
}

func TestMempoolMultiInputDoubleSpend(t *testing.T) {
	mp := NewMempool()

	utxo1 := tx.OutPoint{TxID: crypto.Sha256([]byte("multi-1")), Index: 0}
	utxo2 := tx.OutPoint{TxID: crypto.Sha256([]byte("multi-2")), Index: 0}
	utxo3 := tx.OutPoint{TxID: crypto.Sha256([]byte("multi-3")), Index: 0}

	// txA spends utxo1 and utxo2.
	txA := &tx.Transaction{
		Version: 1,
		Inputs: []tx.TxIn{
			{PrevOut: utxo1, Sequence: 0xFFFFFFFF},
			{PrevOut: utxo2, Sequence: 0xFFFFFFFF},
		},
		Outputs: []tx.TxOut{{Amount: 200, PkScript: []byte{0xaa}}},
	}
	mp.Add(txA)

	// txB spends utxo2 (overlap) and utxo3 → rejected.
	txB := &tx.Transaction{
		Version: 1,
		Inputs: []tx.TxIn{
			{PrevOut: utxo2, Sequence: 0xFFFFFFFF},
			{PrevOut: utxo3, Sequence: 0xFFFFFFFF},
		},
		Outputs: []tx.TxOut{{Amount: 150, PkScript: []byte{0xbb}}},
	}
	if mp.Add(txB) {
		t.Fatal("txB should be rejected — shares utxo2 with txA")
	}

	// txC spends only utxo3 (no overlap) → accepted.
	txC := &tx.Transaction{
		Version: 1,
		Inputs:  []tx.TxIn{{PrevOut: utxo3, Sequence: 0xFFFFFFFF}},
		Outputs: []tx.TxOut{{Amount: 100, PkScript: []byte{0xcc}}},
	}
	if !mp.Add(txC) {
		t.Fatal("txC should succeed — no conflict")
	}
}

// ============================================================
// 12. Inv/Addr message count limits
// ============================================================

func TestDecodeRejectsHugeInvCount(t *testing.T) {
	// Build a raw message with a huge inv count.
	var payload bytes.Buffer
	// count = MaxInvItems + 1
	count := uint32(MaxInvItems + 1)
	buf := make([]byte, 4)
	buf[0] = byte(count)
	buf[1] = byte(count >> 8)
	buf[2] = byte(count >> 16)
	buf[3] = byte(count >> 24)
	payload.Write(buf)
	// Don't bother writing actual items — the count check should fail first.

	// Encode into a wire message manually.
	wireMsg := &MsgInv{Items: nil}
	_ = wireMsg // just for command string
	var wire bytes.Buffer
	// magic
	wire.Write([]byte{0x53, 0x55, 0x4F, 0x4E}) // MainNetMagic LE
	// command "inv\x00..."
	var cmd [CommandSize]byte
	copy(cmd[:], "inv")
	wire.Write(cmd[:])
	// payload length
	payloadBytes := payload.Bytes()
	lenBuf := make([]byte, 4)
	lenBuf[0] = byte(len(payloadBytes))
	lenBuf[1] = byte(len(payloadBytes) >> 8)
	wire.Write(lenBuf)
	// checksum
	cksum := crypto.DoubleSha256(payloadBytes)
	wire.Write(cksum[:4])
	// payload
	wire.Write(payloadBytes)

	_, err := DecodeMessage(bytes.NewReader(wire.Bytes()), MainNetMagic)
	if err == nil {
		t.Fatal("huge inv count should be rejected")
	}
}

// ============================================================
// 13. PeerProtection: ban scoring, expiry, rate limiting
// ============================================================

func TestBanScoreAccumulation(t *testing.T) {
	pp := NewPeerProtection()
	addr := "10.0.0.1:9333"

	// Below threshold — not banned.
	if pp.AddScore(addr, 50) {
		t.Fatal("score 50 should not trigger ban")
	}
	if pp.GetScore(addr) != 50 {
		t.Fatalf("score: want 50, got %d", pp.GetScore(addr))
	}
	if pp.IsBanned(addr) {
		t.Fatal("should not be banned at score 50")
	}

	// Reach threshold — banned.
	if !pp.AddScore(addr, 50) {
		t.Fatal("score 100 should trigger ban")
	}
	if !pp.IsBanned(addr) {
		t.Fatal("should be banned at score 100")
	}
	if pp.BanCount() != 1 {
		t.Fatalf("ban count: want 1, got %d", pp.BanCount())
	}
}

func TestBanScoreByIP(t *testing.T) {
	pp := NewPeerProtection()

	// Same IP, different ports — should share ban score.
	pp.AddScore("10.0.0.1:9333", 60)
	pp.AddScore("10.0.0.1:12345", 40)

	if !pp.IsBanned("10.0.0.1:9999") {
		t.Fatal("same IP on different port should be banned")
	}
}

func TestBannedPeerRejected(t *testing.T) {
	pp := NewPeerProtection()
	addr := "10.0.0.1:9333"

	// Ban the peer.
	pp.AddScore(addr, 100)

	// Should be banned regardless of port.
	if !pp.IsBanned("10.0.0.1:5555") {
		t.Fatal("banned IP should be rejected on any port")
	}
}

func TestRateLimiting(t *testing.T) {
	pp := NewPeerProtection()
	addr := "10.0.0.1:9333"

	// First RateLimitPerSecond messages should be allowed.
	for i := 0; i < RateLimitPerSecond; i++ {
		if !pp.CheckRate(addr) {
			t.Fatalf("message %d should be allowed", i)
		}
	}

	// Next message should be denied.
	if pp.CheckRate(addr) {
		t.Fatal("message beyond rate limit should be denied")
	}
}

func TestRateLimitWindowReset(t *testing.T) {
	pp := NewPeerProtection()
	addr := "10.0.0.1:9333"

	// Exhaust rate limit.
	for i := 0; i <= RateLimitPerSecond; i++ {
		pp.CheckRate(addr)
	}

	// Manually reset the window to simulate time passing.
	pp.mu.Lock()
	host := hostOnly(addr)
	if rt, ok := pp.rates[host]; ok {
		rt.windowEnd = time.Now().Add(-time.Second) // expired
	}
	pp.mu.Unlock()

	// Should be allowed again.
	if !pp.CheckRate(addr) {
		t.Fatal("rate limit should reset after window expires")
	}
}

func TestHostOnlyStripsPort(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"10.0.0.1:9333", "10.0.0.1"},
		{"192.168.1.1:12345", "192.168.1.1"},
		{"localhost:80", "localhost"},
		{"noport", "noport"},
	}
	for _, tt := range tests {
		got := hostOnly(tt.input)
		if got != tt.want {
			t.Fatalf("hostOnly(%q): want %q, got %q", tt.input, tt.want, got)
		}
	}
}

func TestRemovePeerCleansRateTracker(t *testing.T) {
	pp := NewPeerProtection()
	addr := "10.0.0.1:9333"

	pp.CheckRate(addr)
	pp.RemovePeer(addr)

	// After removal, rate tracker should be gone — first message in new window allowed.
	if !pp.CheckRate(addr) {
		t.Fatal("rate should be reset after RemovePeer")
	}
}

func TestBanScoreConstants(t *testing.T) {
	// Verify that a single consensus-invalid block triggers an immediate ban.
	if BanScoreInvalidBlock < BanScoreThreshold {
		t.Fatal("invalid block score should trigger immediate ban")
	}

	// Verify that a single consensus-invalid tx does NOT trigger immediate ban.
	if BanScoreConsensusTx >= BanScoreThreshold {
		t.Fatal("single invalid tx should not trigger immediate ban")
	}

	// Verify HandshakeTimeout is reasonable.
	if HandshakeTimeout < time.Second || HandshakeTimeout > time.Minute {
		t.Fatalf("HandshakeTimeout %v out of reasonable range", HandshakeTimeout)
	}
}

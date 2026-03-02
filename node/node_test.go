package node

import (
	"bytes"
	"encoding/json"
	"math/big"
	"net/http"
	"testing"
	"time"

	"nous/block"
	"nous/consensus"
	"nous/crypto"
	"nous/network"
	"nous/storage"
)

// helper: create a test node with the given genesis block.
func setupTestNode(t *testing.T, genesis *block.Block) (*consensus.ChainState, *storage.BlockStore, *network.Server) {
	t.Helper()
	if genesis == nil {
		genesis = block.GenesisBlock(make([]byte, 20), uint32(time.Now().Unix())-60, 0x1d00ffff)
	}
	dir := t.TempDir()
	store, err := storage.NewBlockStore(dir)
	if err != nil {
		t.Fatal(err)
	}

	chain := consensus.NewChainState(genesis)

	if err := store.SaveBlock(genesis, 0); err != nil {
		t.Fatal(err)
	}

	cfg := network.ServerConfig{
		ListenAddr: ":0",
		Magic:      network.TestNetMagic,
	}
	server := network.NewServer(cfg)
	return chain, store, server
}

// easyDifficulty returns difficulty params suitable for fast testing.
func easyDifficulty() *consensus.DifficultyParams {
	maxTarget := new(big.Int).Lsh(big.NewInt(1), 256)
	maxTarget.Sub(maxTarget, big.NewInt(1))
	var target crypto.Hash
	b := maxTarget.Bytes()
	copy(target[32-len(b):], b)

	return &consensus.DifficultyParams{
		PoWTarget: target,
	}
}

// ============================================================
// 1. Genesis block loads correctly from store
// ============================================================

func TestGenesisBlockLoads(t *testing.T) {
	_, store, _ := setupTestNode(t, nil)

	blk, err := store.LoadBlockByHeight(0)
	if err != nil {
		t.Fatalf("load genesis: %v", err)
	}
	if blk.Header.Version != 1 {
		t.Fatalf("genesis version: want 1, got %d", blk.Header.Version)
	}
	if len(blk.Transactions) != 1 {
		t.Fatalf("genesis tx count: want 1, got %d", len(blk.Transactions))
	}
	if !blk.Transactions[0].IsCoinbase() {
		t.Fatal("genesis tx should be coinbase")
	}
	if !blk.Header.PrevBlockHash.IsZero() {
		t.Fatal("genesis prev hash should be zero")
	}
}

// ============================================================
// 2. RPC getblockcount returns 0 after genesis
// ============================================================

func TestRPCGetBlockCount(t *testing.T) {
	chain, store, server := setupTestNode(t, nil)

	rpc := NewRPCServer("127.0.0.1:0", chain, server, store, nil)
	if err := rpc.Start(); err != nil {
		t.Fatal(err)
	}
	defer rpc.Stop()

	time.Sleep(50 * time.Millisecond)

	body := `{"jsonrpc":"2.0","method":"getblockcount","params":[],"id":1}`
	resp, err := http.Post("http://"+rpc.Addr()+"/rpc", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("rpc request: %v", err)
	}
	defer resp.Body.Close()

	var result rpcResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if result.Error != nil {
		t.Fatalf("rpc error: %s", result.Error.Message)
	}

	count, ok := result.Result.(float64)
	if !ok {
		t.Fatalf("result type: want float64, got %T", result.Result)
	}
	if int(count) != 0 {
		t.Fatalf("block count: want 0, got %d", int(count))
	}
}

// ============================================================
// 3. Mining produces a block at height 1
// ============================================================

func TestMiningProducesBlock(t *testing.T) {
	chain, store, server := setupTestNode(t, nil)
	chain.Difficulty = easyDifficulty()
	chain.Anchor.Target = easyDifficulty().PoWTarget

	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	defer server.Stop()

	_, pubKey, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}

	reasoner := NewReasoner(chain, server, store, pubKey)
	reasoner.Start()

	deadline := time.After(120 * time.Second)
	for {
		select {
		case <-deadline:
			reasoner.Stop()
			t.Fatal("timeout waiting for block to be mined")
		default:
		}
		if chain.Height >= 1 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	reasoner.Stop()

	if chain.Height < 1 {
		t.Fatalf("expected height >= 1, got %d", chain.Height)
	}

	blk, err := store.LoadBlockByHeight(1)
	if err != nil {
		t.Fatalf("load block 1: %v", err)
	}
	if len(blk.Transactions) < 1 {
		t.Fatal("block 1 should have at least a coinbase tx")
	}
	if !blk.Transactions[0].IsCoinbase() {
		t.Fatal("first tx should be coinbase")
	}
}

// ============================================================
// 4. Two-node sync: node B receives block from node A
// ============================================================

func TestTwoNodeSync(t *testing.T) {
	genesis := block.GenesisBlock(make([]byte, 20), uint32(time.Now().Unix())-60, 0x1d00ffff)

	chainA, storeA, serverA := setupTestNode(t, genesis)
	chainA.Difficulty = easyDifficulty()
	chainA.Anchor.Target = easyDifficulty().PoWTarget
	if err := serverA.Start(); err != nil {
		t.Fatal(err)
	}
	defer serverA.Stop()

	_, pubKey, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}

	reasonerA := NewReasoner(chainA, serverA, storeA, pubKey)
	reasonerA.Start()

	deadline := time.After(120 * time.Second)
	for {
		select {
		case <-deadline:
			reasonerA.Stop()
			t.Fatal("timeout waiting for node A to mine")
		default:
		}
		if chainA.Height >= 1 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	reasonerA.Stop()

	chainB, storeB, serverB := setupTestNode(t, genesis)
	chainB.Difficulty = easyDifficulty()
	chainB.Anchor.Target = easyDifficulty().PoWTarget
	if err := serverB.Start(); err != nil {
		t.Fatal(err)
	}
	defer serverB.Stop()

	reasonerB := NewReasoner(chainB, serverB, storeB, pubKey)

	blk, err := storeA.LoadBlockByHeight(1)
	if err != nil {
		t.Fatalf("load block from A: %v", err)
	}

	if err := reasonerB.ApplyBlock(blk); err != nil {
		t.Fatalf("apply block to B: %v", err)
	}

	if chainB.Height != 1 {
		t.Fatalf("node B height: want 1, got %d", chainB.Height)
	}

	blkA, err := storeA.LoadBlockByHeight(1)
	if err != nil {
		t.Fatalf("load block 1 from A: %v", err)
	}
	blkB, err := storeB.LoadBlockByHeight(1)
	if err != nil {
		t.Fatalf("load block 1 from B: %v", err)
	}
	hashA := blkA.Header.Hash()
	hashB := blkB.Header.Hash()
	if hashA != hashB {
		t.Fatalf("block 1 hash mismatch: A=%x B=%x", hashA[:8], hashB[:8])
	}
}

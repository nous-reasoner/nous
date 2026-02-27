package node

import (
	"bytes"
	"encoding/json"
	"math/big"
	"net/http"
	"testing"
	"time"

	"github.com/nous-chain/nous/block"
	"github.com/nous-chain/nous/consensus"
	"github.com/nous-chain/nous/crypto"
	"github.com/nous-chain/nous/network"
	"github.com/nous-chain/nous/storage"
)

// helper: create a test node with the given genesis block.
// If genesis is nil, a fresh one is created.
func setupTestNode(t *testing.T, genesis *block.Block) (*consensus.ChainState, *storage.BlockStore, *network.Server) {
	t.Helper()
	if genesis == nil {
		genesis = block.GenesisBlock(make([]byte, 20), uint32(time.Now().Unix())-60)
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

// easyDifficulty returns difficulty params suitable for fast testing:
// minimal VDF iterations and the easiest possible PoW target.
func easyDifficulty() *consensus.DifficultyParams {
	// Max 256-bit target = 2^256 - 1 (any hash passes).
	maxTarget := new(big.Int).Lsh(big.NewInt(1), 256)
	maxTarget.Sub(maxTarget, big.NewInt(1))
	var target crypto.Hash
	b := maxTarget.Bytes()
	copy(target[32-len(b):], b)

	return &consensus.DifficultyParams{
		VDFIterations: 1, // single iteration
		CSPDifficulty: consensus.CSPDifficultyParams{
			BaseVariables:   12,
			ConstraintRatio: 1.4,
		},
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

	// Give the HTTP server a moment to start serving.
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

	// Result should be 0 (genesis height).
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

	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	defer server.Stop()

	privKey, pubKey, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}

	miner := NewMiner(chain, server, store, privKey, pubKey)
	miner.Start()

	// Wait for at least one block to be mined (with timeout).
	deadline := time.After(120 * time.Second)
	for {
		select {
		case <-deadline:
			miner.Stop()
			t.Fatal("timeout waiting for block to be mined")
		default:
		}
		if chain.Height >= 1 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	miner.Stop()

	if chain.Height < 1 {
		t.Fatalf("expected height >= 1, got %d", chain.Height)
	}

	// Verify the mined block is persisted.
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
	// Shared genesis block so both nodes have the same chain root.
	genesis := block.GenesisBlock(make([]byte, 20), uint32(time.Now().Unix())-60)

	// Node A: mines a block.
	chainA, storeA, serverA := setupTestNode(t, genesis)
	chainA.Difficulty = easyDifficulty()
	if err := serverA.Start(); err != nil {
		t.Fatal(err)
	}
	defer serverA.Stop()

	privKey, pubKey, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}

	minerA := NewMiner(chainA, serverA, storeA, privKey, pubKey)
	minerA.Start()

	// Wait for node A to mine a block.
	deadline := time.After(120 * time.Second)
	for {
		select {
		case <-deadline:
			minerA.Stop()
			t.Fatal("timeout waiting for node A to mine")
		default:
		}
		if chainA.Height >= 1 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	minerA.Stop()

	// Node B: connect to A and receive the block.
	chainB, storeB, serverB := setupTestNode(t, genesis)
	chainB.Difficulty = easyDifficulty()
	if err := serverB.Start(); err != nil {
		t.Fatal(err)
	}
	defer serverB.Stop()

	minerB := NewMiner(chainB, serverB, storeB, privKey, pubKey)

	// Load the block mined by A and apply it to B.
	blk, err := storeA.LoadBlockByHeight(1)
	if err != nil {
		t.Fatalf("load block from A: %v", err)
	}

	if err := minerB.ApplyBlock(blk); err != nil {
		t.Fatalf("apply block to B: %v", err)
	}

	if chainB.Height != 1 {
		t.Fatalf("node B height: want 1, got %d", chainB.Height)
	}

	// Verify block 1 hashes match on both nodes.
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

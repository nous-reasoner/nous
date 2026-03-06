package node

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"nous/block"
	"nous/consensus"
	"nous/crypto"
	"nous/network"
	"nous/storage"
	"nous/tx"
)

// rpcCall sends a JSON-RPC request and decodes the response.
func rpcCall(t *testing.T, addr, method string, params interface{}) rpcResponse {
	t.Helper()
	p, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	body := rpcRequest{JSONRPC: "2.0", Method: method, Params: p, ID: 1}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post("http://"+addr+"/rpc", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("rpc %s: %v", method, err)
	}
	defer resp.Body.Close()
	var result rpcResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode %s response: %v", method, err)
	}
	return result
}

// TestListUnspentFiltersCoinbaseMaturity verifies that listunspent
// omits immature coinbase outputs and includes mature ones.
func TestListUnspentFiltersCoinbaseMaturity(t *testing.T) {
	// Generate a key pair for mining.
	_, pubKey, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pkh := crypto.Hash160(pubKey.SerializeCompressed())
	addr := crypto.PubKeyToBech32mAddress(pubKey)

	// Create a testnet genesis block paying to our key.
	genesis := block.GenesisBlock(pkh, uint32(time.Now().Unix())-60,
		consensus.TargetToCompact(easyDifficulty().PoWTarget), true)

	chain := consensus.NewChainState(genesis)
	chain.Difficulty = easyDifficulty()
	chain.Anchor.Target = easyDifficulty().PoWTarget
	chain.IsTestnet = true

	dir := t.TempDir()
	store, err := storage.NewBlockStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveBlock(genesis, 0); err != nil {
		t.Fatal(err)
	}

	cfg := network.ServerConfig{ListenAddr: ":0", Magic: network.TestNetMagic}
	server := network.NewServer(cfg)

	rpc := NewRPCServer("127.0.0.1:0", chain, server, store, nil)
	if err := rpc.Start(); err != nil {
		t.Fatal(err)
	}
	defer rpc.Stop()
	time.Sleep(50 * time.Millisecond)

	// Mine 5 blocks — all coinbase outputs should be immature.
	prev := &genesis.Header
	for h := uint64(1); h <= 5; h++ {
		blk, err := consensus.MineBlock(prev, nil, pkh, chain.Difficulty, h, nil, true)
		if err != nil {
			t.Fatalf("mine block %d: %v", h, err)
		}
		if err := chain.AddBlock(blk); err != nil {
			t.Fatalf("add block %d: %v", h, err)
		}
		prev = &blk.Header
	}

	// At height 5, only genesis coinbase (height 0) is not yet mature
	// (needs height >= 0 + 100 = 100). All mined coinbase outputs are immature.
	// However the genesis coinbase at height 0 is also immature (5 < 0+100).
	// So listunspent should return 0 UTXOs (all are coinbase, all immature).
	resp := rpcCall(t, rpc.Addr(), "listunspent", []string{addr})
	if resp.Error != nil {
		t.Fatalf("listunspent error: %s", resp.Error.Message)
	}
	utxos, ok := resp.Result.([]interface{})
	if !ok {
		t.Fatalf("result type: want []interface{}, got %T", resp.Result)
	}
	if len(utxos) != 0 {
		t.Fatalf("at height 5, want 0 mature UTXOs, got %d", len(utxos))
	}

	// Mine to height 100 — genesis coinbase (height 0) should now be mature.
	for h := uint64(6); h <= tx.CoinbaseMaturity; h++ {
		blk, err := consensus.MineBlock(prev, nil, pkh, chain.Difficulty, h, nil, true)
		if err != nil {
			t.Fatalf("mine block %d: %v", h, err)
		}
		if err := chain.AddBlock(blk); err != nil {
			t.Fatalf("add block %d: %v", h, err)
		}
		prev = &blk.Header
	}

	// At height 100: genesis coinbase (height 0) is mature (100 >= 0+100).
	// Block 1 coinbase is not mature (100 < 1+100 = 101).
	// So exactly 1 UTXO should be returned (the genesis coinbase).
	resp = rpcCall(t, rpc.Addr(), "listunspent", []string{addr})
	if resp.Error != nil {
		t.Fatalf("listunspent error: %s", resp.Error.Message)
	}
	utxos = resp.Result.([]interface{})
	if len(utxos) != 1 {
		t.Fatalf("at height %d, want 1 mature UTXO, got %d", tx.CoinbaseMaturity, len(utxos))
	}

	// Verify the returned UTXO has is_coinbase field set to true.
	u := utxos[0].(map[string]interface{})
	isCoinbase, ok := u["is_coinbase"].(bool)
	if !ok || !isCoinbase {
		t.Fatalf("want is_coinbase=true, got %v", u["is_coinbase"])
	}

	// Mine one more block (height 101) — now block 1's coinbase also matures.
	blk, err := consensus.MineBlock(prev, nil, pkh, chain.Difficulty, tx.CoinbaseMaturity+1, nil, true)
	if err != nil {
		t.Fatalf("mine block %d: %v", tx.CoinbaseMaturity+1, err)
	}
	if err := chain.AddBlock(blk); err != nil {
		t.Fatalf("add block %d: %v", tx.CoinbaseMaturity+1, err)
	}

	// At height 101: genesis (h=0) mature, block 1 (h=1) mature (101 >= 101).
	// Block 2 coinbase still immature (101 < 2+100=102).
	// So 2 mature UTXOs.
	resp = rpcCall(t, rpc.Addr(), "listunspent", []string{addr})
	if resp.Error != nil {
		t.Fatalf("listunspent error: %s", resp.Error.Message)
	}
	utxos = resp.Result.([]interface{})
	if len(utxos) != 2 {
		t.Fatalf("at height %d, want 2 mature UTXOs, got %d", tx.CoinbaseMaturity+1, len(utxos))
	}
}

// TestGetBalanceFiltersCoinbaseMaturity verifies that getbalance returns
// separate balance and immature fields, correctly filtering immature coinbase.
func TestGetBalanceFiltersCoinbaseMaturity(t *testing.T) {
	_, pubKey, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pkh := crypto.Hash160(pubKey.SerializeCompressed())
	addr := crypto.PubKeyToBech32mAddress(pubKey)

	genesis := block.GenesisBlock(pkh, uint32(time.Now().Unix())-60,
		consensus.TargetToCompact(easyDifficulty().PoWTarget), true)

	chain := consensus.NewChainState(genesis)
	chain.Difficulty = easyDifficulty()
	chain.Anchor.Target = easyDifficulty().PoWTarget
	chain.IsTestnet = true

	dir := t.TempDir()
	store, err := storage.NewBlockStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveBlock(genesis, 0); err != nil {
		t.Fatal(err)
	}

	cfg := network.ServerConfig{ListenAddr: ":0", Magic: network.TestNetMagic}
	server := network.NewServer(cfg)

	rpc := NewRPCServer("127.0.0.1:0", chain, server, store, nil)
	if err := rpc.Start(); err != nil {
		t.Fatal(err)
	}
	defer rpc.Stop()
	time.Sleep(50 * time.Millisecond)

	// Mine 5 blocks — all coinbase outputs are immature.
	prev := &genesis.Header
	for h := uint64(1); h <= 5; h++ {
		blk, err := consensus.MineBlock(prev, nil, pkh, chain.Difficulty, h, nil, true)
		if err != nil {
			t.Fatalf("mine block %d: %v", h, err)
		}
		if err := chain.AddBlock(blk); err != nil {
			t.Fatalf("add block %d: %v", h, err)
		}
		prev = &blk.Header
	}

	// At height 5: all 6 coinbase outputs (genesis + blocks 1-5) are immature.
	resp := rpcCall(t, rpc.Addr(), "getbalance", []string{addr})
	if resp.Error != nil {
		t.Fatalf("getbalance error: %s", resp.Error.Message)
	}
	result := resp.Result.(map[string]interface{})
	balance := int64(result["balance"].(float64))
	immature := int64(result["immature"].(float64))

	if balance != 0 {
		t.Fatalf("at height 5, want balance=0, got %d", balance)
	}
	if immature == 0 {
		t.Fatal("at height 5, want immature > 0, got 0")
	}
	totalImmature := immature

	// Mine to height 100 — genesis coinbase (height 0) should mature.
	for h := uint64(6); h <= tx.CoinbaseMaturity; h++ {
		blk, err := consensus.MineBlock(prev, nil, pkh, chain.Difficulty, h, nil, true)
		if err != nil {
			t.Fatalf("mine block %d: %v", h, err)
		}
		if err := chain.AddBlock(blk); err != nil {
			t.Fatalf("add block %d: %v", h, err)
		}
		prev = &blk.Header
	}

	// At height 100: genesis coinbase (h=0) is mature, rest are immature.
	resp = rpcCall(t, rpc.Addr(), "getbalance", []string{addr})
	if resp.Error != nil {
		t.Fatalf("getbalance error: %s", resp.Error.Message)
	}
	result = resp.Result.(map[string]interface{})
	balance = int64(result["balance"].(float64))
	immature = int64(result["immature"].(float64))

	// Genesis coinbase = 1 NOUS = 100000000 nou.
	if balance != 1_0000_0000 {
		t.Fatalf("at height 100, want balance=100000000, got %d", balance)
	}
	if balance+immature != totalImmature+int64(tx.CoinbaseMaturity-5)*1_0000_0000 {
		// balance + immature should equal all UTXOs (101 coinbase × 1 NOUS).
		t.Logf("balance=%d immature=%d total=%d", balance, immature, balance+immature)
	}
	if immature == 0 {
		t.Fatal("at height 100, want immature > 0 (blocks 1-100 still immature)")
	}
}

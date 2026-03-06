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

// newTestRPC creates a testnet chain, store, and RPC server for testing.
// Returns the chain, rpc server address, pkh, bech32m address, and genesis header.
func newTestRPC(t *testing.T) (*consensus.ChainState, string, []byte, string, *block.Header) {
	t.Helper()
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
	t.Cleanup(func() { rpc.Stop() })
	time.Sleep(50 * time.Millisecond)

	return chain, rpc.Addr(), pkh, addr, &genesis.Header
}

// mineBlocks mines blocks from startHeight to endHeight inclusive.
func mineBlocks(t *testing.T, chain *consensus.ChainState, prev *block.Header, pkh []byte, from, to uint64) *block.Header {
	t.Helper()
	for h := from; h <= to; h++ {
		blk, err := consensus.MineBlock(prev, nil, pkh, chain.Difficulty, h, nil, true)
		if err != nil {
			t.Fatalf("mine block %d: %v", h, err)
		}
		if err := chain.AddBlock(blk); err != nil {
			t.Fatalf("add block %d: %v", h, err)
		}
		prev = &blk.Header
	}
	return prev
}

// TestListUnspentFiltersCoinbaseMaturity verifies that listunspent
// omits immature coinbase outputs and includes mature ones.
func TestListUnspentFiltersCoinbaseMaturity(t *testing.T) {
	maturity := tx.TestnetCoinbaseMaturity // 10 for testnet
	chain, rpcAddr, pkh, addr, genHeader := newTestRPC(t)

	// Mine 5 blocks — all coinbase outputs should be immature (5 < 10).
	prev := mineBlocks(t, chain, genHeader, pkh, 1, 5)

	resp := rpcCall(t, rpcAddr, "listunspent", []string{addr})
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

	// Mine to height=maturity — genesis coinbase (height 0) should now be mature.
	prev = mineBlocks(t, chain, prev, pkh, 6, maturity)

	// At height=maturity: genesis coinbase (h=0) mature, block 1 not yet.
	resp = rpcCall(t, rpcAddr, "listunspent", []string{addr})
	if resp.Error != nil {
		t.Fatalf("listunspent error: %s", resp.Error.Message)
	}
	utxos = resp.Result.([]interface{})
	if len(utxos) != 1 {
		t.Fatalf("at height %d, want 1 mature UTXO, got %d", maturity, len(utxos))
	}

	// Verify the returned UTXO has is_coinbase field set to true.
	u := utxos[0].(map[string]interface{})
	isCoinbase, ok := u["is_coinbase"].(bool)
	if !ok || !isCoinbase {
		t.Fatalf("want is_coinbase=true, got %v", u["is_coinbase"])
	}

	// Mine one more block — now block 1's coinbase also matures.
	mineBlocks(t, chain, prev, pkh, maturity+1, maturity+1)

	// 2 mature UTXOs: genesis (h=0) and block 1 (h=1).
	resp = rpcCall(t, rpcAddr, "listunspent", []string{addr})
	if resp.Error != nil {
		t.Fatalf("listunspent error: %s", resp.Error.Message)
	}
	utxos = resp.Result.([]interface{})
	if len(utxos) != 2 {
		t.Fatalf("at height %d, want 2 mature UTXOs, got %d", maturity+1, len(utxos))
	}
}

// TestGetBalanceFiltersCoinbaseMaturity verifies that getbalance returns
// separate balance and immature fields, correctly filtering immature coinbase.
func TestGetBalanceFiltersCoinbaseMaturity(t *testing.T) {
	maturity := tx.TestnetCoinbaseMaturity // 10 for testnet
	chain, rpcAddr, pkh, addr, genHeader := newTestRPC(t)

	// Mine 5 blocks — all coinbase outputs are immature.
	prev := mineBlocks(t, chain, genHeader, pkh, 1, 5)

	// At height 5: all 6 coinbase outputs (genesis + blocks 1-5) are immature.
	resp := rpcCall(t, rpcAddr, "getbalance", []string{addr})
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

	// Mine to height=maturity — genesis coinbase (height 0) should mature.
	mineBlocks(t, chain, prev, pkh, 6, maturity)

	resp = rpcCall(t, rpcAddr, "getbalance", []string{addr})
	if resp.Error != nil {
		t.Fatalf("getbalance error: %s", resp.Error.Message)
	}
	result = resp.Result.(map[string]interface{})
	balance = int64(result["balance"].(float64))
	immature = int64(result["immature"].(float64))

	// Genesis coinbase = 1 NOUS = 100000000 nou.
	if balance != 1_0000_0000 {
		t.Fatalf("at height %d, want balance=100000000, got %d", maturity, balance)
	}
	if immature == 0 {
		t.Fatalf("at height %d, want immature > 0 (blocks 1-%d still immature)", maturity, maturity)
	}
}

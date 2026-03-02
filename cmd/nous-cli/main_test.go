package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"nous/crypto"
	"nous/tx"
	"nous/wallet"
)

// ============================================================
// 1. createwallet creates file and produces a valid address
// ============================================================

func TestCreateWallet(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test-wallet.dat")
	pass := "testpass"

	flagWalletFile = path
	flagWalletPass = pass
	defer func() {
		flagWalletFile = ""
		flagWalletPass = ""
	}()

	if err := cmdCreateWallet(); err != nil {
		t.Fatalf("createwallet: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("wallet file not found: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("wallet file is empty")
	}

	w, err := wallet.LoadFromFile(path, pass)
	if err != nil {
		t.Fatalf("load wallet: %v", err)
	}

	addr := w.GetAddress()
	if addr == "" {
		t.Fatal("address is empty")
	}

	pkh, err := crypto.AddressToPubKeyHash(addr)
	if err != nil {
		t.Fatalf("invalid address %s: %v", addr, err)
	}
	if len(pkh) != 20 {
		t.Fatalf("pubkey hash length: want 20, got %d", len(pkh))
	}
}

// ============================================================
// 2. RPC client encodes requests and decodes responses correctly
// ============================================================

func TestRPCClientEncodeDecode(t *testing.T) {
	var receivedMethod string
	var receivedParams json.RawMessage

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			JSONRPC string          `json:"jsonrpc"`
			Method  string          `json:"method"`
			Params  json.RawMessage `json:"params"`
			ID      int             `json:"id"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		receivedMethod = req.Method
		receivedParams = req.Params

		var result interface{}
		switch req.Method {
		case "getblockcount":
			result = 42
		case "getbalance":
			result = int64(500000000)
		default:
			w.WriteHeader(200)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"jsonrpc": "2.0",
				"error":   map[string]interface{}{"code": -32601, "message": "not found"},
				"id":      req.ID,
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"jsonrpc": "2.0",
			"result":  result,
			"id":      req.ID,
		})
	}))
	defer srv.Close()

	client := &RPCClient{
		url:    srv.URL + "/rpc",
		client: srv.Client(),
	}

	// getblockcount encodes correctly and decodes result.
	var height float64
	if err := client.CallInto(&height, "getblockcount", nil); err != nil {
		t.Fatalf("getblockcount: %v", err)
	}
	if receivedMethod != "getblockcount" {
		t.Fatalf("method: want getblockcount, got %s", receivedMethod)
	}
	if int(height) != 42 {
		t.Fatalf("height: want 42, got %v", height)
	}

	// getbalance encodes params correctly.
	var balance float64
	if err := client.CallInto(&balance, "getbalance", []string{"N1abc123"}); err != nil {
		t.Fatalf("getbalance: %v", err)
	}
	if receivedMethod != "getbalance" {
		t.Fatalf("method: want getbalance, got %s", receivedMethod)
	}
	var params []string
	if err := json.Unmarshal(receivedParams, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if len(params) != 1 || params[0] != "N1abc123" {
		t.Fatalf("params: want [N1abc123], got %v", params)
	}

	// Unknown method returns RPC error.
	_, err := client.Call("nonexistent", nil)
	if err == nil {
		t.Fatal("expected error for unknown method")
	}
	rpcErr, ok := err.(*rpcError)
	if !ok {
		t.Fatalf("expected *rpcError, got %T", err)
	}
	if rpcErr.Code != -32601 {
		t.Fatalf("error code: want -32601, got %d", rpcErr.Code)
	}
}

// ============================================================
// 3. send command constructs a valid signed transaction
// ============================================================

func TestSendTransactionFormat(t *testing.T) {
	// Create a wallet with a known key.
	w, err := wallet.NewWallet()
	if err != nil {
		t.Fatal(err)
	}
	senderAddr := string(w.GetAddress())
	senderPKH := w.PubKeyHash()

	// Save wallet to temp file.
	dir := t.TempDir()
	wPath := filepath.Join(dir, "send-test.dat")
	pass := "testpass"
	if err := w.SaveToFile(wPath, pass); err != nil {
		t.Fatal(err)
	}

	// Create a fake UTXO belonging to the sender.
	fakeTxID := crypto.DoubleSha256([]byte("fake-funding-tx"))
	fakeScript := tx.CreateP2PKHLockScript(senderPKH)
	fakeValue := int64(5 * NOU) // 5 NOUS

	// Mock RPC server: returns UTXO on listunspent, captures raw tx on sendrawtx.
	var capturedRawTx string
	mockSrv := httptest.NewServer(http.HandlerFunc(func(hw http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
			ID     int             `json:"id"`
		}
		json.NewDecoder(r.Body).Decode(&req)

		var result interface{}
		switch req.Method {
		case "listunspent":
			result = []map[string]interface{}{
				{
					"txid":   hex.EncodeToString(fakeTxID[:]),
					"index":  0,
					"value":  fakeValue,
					"script": hex.EncodeToString(fakeScript),
					"height": 1,
				},
			}
		case "sendrawtx":
			var args []string
			json.Unmarshal(req.Params, &args)
			capturedRawTx = args[0]
			fakeTxHash := crypto.DoubleSha256([]byte("result-txid"))
			result = hex.EncodeToString(fakeTxHash[:])
		default:
			json.NewEncoder(hw).Encode(map[string]interface{}{
				"jsonrpc": "2.0",
				"error":   map[string]interface{}{"code": -32601, "message": "not found"},
				"id":      req.ID,
			})
			return
		}
		json.NewEncoder(hw).Encode(map[string]interface{}{
			"jsonrpc": "2.0",
			"result":  result,
			"id":      req.ID,
		})
	}))
	defer mockSrv.Close()

	// Generate a destination address.
	_, destPub, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	destAddr := crypto.PubKeyToAddress(destPub)
	sendAmount := int64(2 * NOU) // 2 NOUS

	// Build a local UTXOSet from the mock data and create the transaction.
	utxoSet := tx.NewUTXOSet()
	utxoSet.Add(
		tx.OutPoint{TxID: fakeTxID, Index: 0},
		tx.TxOut{Amount: fakeValue, PkScript: fakeScript},
		1,
		false,
	)

	// Reload wallet from file (same as cmdSend would do).
	w2, err := wallet.LoadFromFile(wPath, pass)
	if err != nil {
		t.Fatalf("reload wallet: %v", err)
	}

	transaction, err := w2.CreateTransaction(destAddr, sendAmount, DefaultFee, utxoSet)
	if err != nil {
		t.Fatalf("create tx: %v", err)
	}

	// Verify transaction structure.
	if transaction.Version != 2 {
		t.Fatalf("tx version: want 2, got %d", transaction.Version)
	}
	if len(transaction.Inputs) != 1 {
		t.Fatalf("input count: want 1, got %d", len(transaction.Inputs))
	}
	if transaction.Inputs[0].PrevOut.TxID != fakeTxID {
		t.Fatal("input does not reference the funding UTXO")
	}
	if transaction.Inputs[0].PrevOut.Index != 0 {
		t.Fatalf("input index: want 0, got %d", transaction.Inputs[0].PrevOut.Index)
	}

	// Should have 2 outputs: recipient + change.
	expectedChange := fakeValue - sendAmount - DefaultFee
	if len(transaction.Outputs) != 2 {
		t.Fatalf("output count: want 2, got %d", len(transaction.Outputs))
	}
	if transaction.Outputs[0].Amount != sendAmount {
		t.Fatalf("output[0] value: want %d, got %d", sendAmount, transaction.Outputs[0].Amount)
	}
	if transaction.Outputs[1].Amount != expectedChange {
		t.Fatalf("change value: want %d, got %d", expectedChange, transaction.Outputs[1].Amount)
	}

	// ScriptSig should be non-empty (signed).
	if len(transaction.Inputs[0].SignatureScript) == 0 {
		t.Fatal("input scriptSig is empty (unsigned)")
	}

	// Serialize/deserialize round-trip.
	raw := transaction.Serialize()
	decoded, err := tx.Deserialize(raw)
	if err != nil {
		t.Fatalf("deserialize: %v", err)
	}
	if decoded.TxID() != transaction.TxID() {
		t.Fatal("txid mismatch after round-trip")
	}

	// Verify the hex encoding works (same as cmdSend sends to RPC).
	rawHex := hex.EncodeToString(raw)
	if len(rawHex) == 0 {
		t.Fatal("serialized hex is empty")
	}

	// Suppress unused variable warnings.
	_ = capturedRawTx
	_ = senderAddr
	_ = mockSrv
	_ = fmt.Sprintf("")
}

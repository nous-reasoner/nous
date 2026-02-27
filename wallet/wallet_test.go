package wallet

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nous-chain/nous/crypto"
	"github.com/nous-chain/nous/tx"
)

// ============================================================
// 1. Create wallet, address format correct
// ============================================================

func TestNewWalletAddressFormat(t *testing.T) {
	w, err := NewWallet()
	if err != nil {
		t.Fatal(err)
	}

	addr := w.GetAddress()
	if len(addr) == 0 {
		t.Fatal("address should not be empty")
	}

	// Address should be valid Base58Check — round-trip decode must succeed.
	pkh, err := crypto.AddressToPubKeyHash(addr)
	if err != nil {
		t.Fatalf("address decode failed: %v", err)
	}
	if len(pkh) != 20 {
		t.Fatalf("pubkey hash should be 20 bytes, got %d", len(pkh))
	}

	// PubKeyHash helper should match.
	if got := w.PubKeyHash(); len(got) != 20 {
		t.Fatalf("PubKeyHash should be 20 bytes, got %d", len(got))
	}
}

// ============================================================
// 2. Export then import private key, address matches
// ============================================================

func TestExportImportPrivateKey(t *testing.T) {
	w1, err := NewWallet()
	if err != nil {
		t.Fatal(err)
	}

	exported := w1.ExportPrivateKey()
	if len(exported) != 32 {
		t.Fatalf("exported key should be 32 bytes, got %d", len(exported))
	}

	w2, err := NewWallet()
	if err != nil {
		t.Fatal(err)
	}
	idx, err := w2.ImportPrivateKey(exported)
	if err != nil {
		t.Fatal(err)
	}

	// The imported key should produce the same address.
	if w2.Keys[idx].Address != w1.GetAddress() {
		t.Fatalf("imported address mismatch: want %s, got %s", w1.GetAddress(), w2.Keys[idx].Address)
	}
}

// ============================================================
// 3. Save to file then load, keys match
// ============================================================

func TestSaveLoadFile(t *testing.T) {
	w1, err := NewWallet()
	if err != nil {
		t.Fatal(err)
	}
	// Add a second key.
	w1.GenerateNewKey()

	dir := t.TempDir()
	path := filepath.Join(dir, "wallet.dat")
	password := "test-password-123"

	if err := w1.SaveToFile(path, password); err != nil {
		t.Fatalf("save: %v", err)
	}

	// File should exist.
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not created: %v", err)
	}

	w2, err := LoadFromFile(path, password)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if len(w2.Keys) != len(w1.Keys) {
		t.Fatalf("key count: want %d, got %d", len(w1.Keys), len(w2.Keys))
	}
	if w2.Primary != w1.Primary {
		t.Fatalf("primary: want %d, got %d", w1.Primary, w2.Primary)
	}
	for i := range w1.Keys {
		if w2.Keys[i].Address != w1.Keys[i].Address {
			t.Fatalf("key %d address mismatch", i)
		}
	}
}

// ============================================================
// 4. Wrong password fails
// ============================================================

func TestLoadWrongPassword(t *testing.T) {
	w, err := NewWallet()
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "wallet.dat")

	if err := w.SaveToFile(path, "correct-password"); err != nil {
		t.Fatal(err)
	}

	_, err = LoadFromFile(path, "wrong-password")
	if err == nil {
		t.Fatal("loading with wrong password should fail")
	}
}

// ============================================================
// Helper: fund a wallet with a coinbase UTXO
// ============================================================

func fundWallet(t *testing.T, w *Wallet, amount int64) *tx.UTXOSet {
	t.Helper()
	utxoSet := tx.NewUTXOSet()
	pkh := w.PubKeyHash()
	coinbase := tx.NewCoinbase(1, amount, pkh, "test")
	utxoSet.AddTransaction(coinbase, 1)
	return utxoSet
}

// ============================================================
// 5. Create transaction: amounts, change, valid signatures
// ============================================================

func TestCreateTransaction(t *testing.T) {
	sender, err := NewWallet()
	if err != nil {
		t.Fatal(err)
	}
	receiver, err := NewWallet()
	if err != nil {
		t.Fatal(err)
	}

	// Fund sender with 100 NOUS.
	utxoSet := fundWallet(t, sender, 100_0000_0000)

	// Send 30 NOUS with 1 NOUS fee.
	amount := int64(30_0000_0000)
	fee := int64(1_0000_0000)
	transaction, err := sender.CreateTransaction(receiver.GetAddress(), amount, fee, utxoSet)
	if err != nil {
		t.Fatalf("CreateTransaction: %v", err)
	}

	// Should have 1 input (the coinbase UTXO).
	if len(transaction.Inputs) != 1 {
		t.Fatalf("inputs: want 1, got %d", len(transaction.Inputs))
	}

	// Should have 2 outputs: recipient + change.
	if len(transaction.Outputs) != 2 {
		t.Fatalf("outputs: want 2, got %d", len(transaction.Outputs))
	}

	// Output 0: recipient gets amount.
	if transaction.Outputs[0].Value != amount {
		t.Fatalf("recipient value: want %d, got %d", amount, transaction.Outputs[0].Value)
	}

	// Output 1: change = 100 - 30 - 1 = 69 NOUS.
	expectedChange := int64(69_0000_0000)
	if transaction.Outputs[1].Value != expectedChange {
		t.Fatalf("change value: want %d, got %d", expectedChange, transaction.Outputs[1].Value)
	}

	// Verify the signature is valid via script execution.
	// The input spends a P2PKH output belonging to sender.
	senderPKH := sender.PubKeyHash()
	lockScript := tx.CreateP2PKHLockScript(senderPKH)
	ok := tx.ExecuteScript(transaction.Inputs[0].ScriptSig, lockScript, transaction, 0)
	if !ok {
		t.Fatal("script execution should succeed for signed input")
	}
}

// ============================================================
// 6. Insufficient balance returns error
// ============================================================

func TestCreateTransactionInsufficientFunds(t *testing.T) {
	w, err := NewWallet()
	if err != nil {
		t.Fatal(err)
	}

	receiver, err := NewWallet()
	if err != nil {
		t.Fatal(err)
	}

	// Fund with only 10 NOUS.
	utxoSet := fundWallet(t, w, 10_0000_0000)

	// Try to send 50 NOUS.
	_, err = w.CreateTransaction(receiver.GetAddress(), 50_0000_0000, 1_0000_0000, utxoSet)
	if err == nil {
		t.Fatal("should fail with insufficient funds")
	}
}

// ============================================================
// 7. Balance and UTXO queries
// ============================================================

func TestGetBalanceAndUTXOs(t *testing.T) {
	w, err := NewWallet()
	if err != nil {
		t.Fatal(err)
	}

	utxoSet := fundWallet(t, w, 50_0000_0000)

	bal := w.GetBalance(utxoSet)
	if bal != 50_0000_0000 {
		t.Fatalf("balance: want 5000000000, got %d", bal)
	}

	utxos := w.GetUTXOs(utxoSet)
	if len(utxos) != 1 {
		t.Fatalf("utxo count: want 1, got %d", len(utxos))
	}
}

// ============================================================
// 8. Exact amount (no change output)
// ============================================================

func TestCreateTransactionNoChange(t *testing.T) {
	sender, err := NewWallet()
	if err != nil {
		t.Fatal(err)
	}
	receiver, err := NewWallet()
	if err != nil {
		t.Fatal(err)
	}

	utxoSet := fundWallet(t, sender, 10_0000_0000)

	// Send exactly 9 NOUS + 1 NOUS fee = 10 NOUS total (no change).
	transaction, err := sender.CreateTransaction(receiver.GetAddress(), 9_0000_0000, 1_0000_0000, utxoSet)
	if err != nil {
		t.Fatalf("CreateTransaction: %v", err)
	}

	// Should have only 1 output (no change).
	if len(transaction.Outputs) != 1 {
		t.Fatalf("outputs: want 1 (no change), got %d", len(transaction.Outputs))
	}
	if transaction.Outputs[0].Value != 9_0000_0000 {
		t.Fatalf("recipient value: want 900000000, got %d", transaction.Outputs[0].Value)
	}
}

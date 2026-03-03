package tx

import (
	"os"
	"path/filepath"
	"testing"

	"nous/crypto"
)

func tempBoltUTXO(t *testing.T) (*BoltUTXOSet, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "utxo.db")
	s, err := NewBoltUTXOSet(path)
	if err != nil {
		t.Fatalf("NewBoltUTXOSet: %v", err)
	}
	return s, path
}

func testOutPoint(idx uint32) OutPoint {
	var txid crypto.Hash
	txid[0] = byte(idx)
	txid[1] = byte(idx >> 8)
	return OutPoint{TxID: txid, Index: idx}
}

func testTxOut(amount int64) TxOut {
	return TxOut{Amount: amount, PkScript: []byte{OpDup, OpHash160, 20,
		1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20,
		OpEqualVerify, OpCheckSig}}
}

// Test 1: Basic add/get/spend.
func TestBoltUTXOBasicCRUD(t *testing.T) {
	s, _ := tempBoltUTXO(t)
	defer s.Close()

	op1 := testOutPoint(1)
	op2 := testOutPoint(2)
	op3 := testOutPoint(3)

	// Add 3 UTXOs.
	s.Add(op1, testTxOut(100), 10, false)
	s.Add(op2, testTxOut(200), 20, true)
	s.Add(op3, testTxOut(300), 30, false)

	if s.Count() != 3 {
		t.Fatalf("expected 3, got %d", s.Count())
	}

	// Get all 3.
	u1 := s.Get(op1)
	if u1 == nil || u1.Output.Amount != 100 || u1.Height != 10 || u1.IsCoinbase {
		t.Fatalf("op1 mismatch: %+v", u1)
	}
	u2 := s.Get(op2)
	if u2 == nil || u2.Output.Amount != 200 || u2.Height != 20 || !u2.IsCoinbase {
		t.Fatalf("op2 mismatch: %+v", u2)
	}

	// Spend op2.
	if !s.Spend(op2) {
		t.Fatal("Spend op2 should return true")
	}
	if s.Get(op2) != nil {
		t.Fatal("op2 should be gone after Spend")
	}
	if s.Count() != 2 {
		t.Fatalf("expected 2, got %d", s.Count())
	}
}

// Test 2: Persistence across close/reopen.
func TestBoltUTXOPersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "utxo.db")

	// Write.
	s1, err := NewBoltUTXOSet(path)
	if err != nil {
		t.Fatal(err)
	}
	op := testOutPoint(42)
	s1.Add(op, testTxOut(999), 5, true)
	s1.Close()

	// Reopen.
	s2, err := NewBoltUTXOSet(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()

	u := s2.Get(op)
	if u == nil {
		t.Fatal("UTXO not found after reopen")
	}
	if u.Output.Amount != 999 || u.Height != 5 || !u.IsCoinbase {
		t.Fatalf("mismatch after reopen: %+v", u)
	}
}

// Test 3: ApplyBlockWithUndo atomicity.
func TestBoltUTXOApplyBlockWithUndo(t *testing.T) {
	s, _ := tempBoltUTXO(t)
	defer s.Close()

	// Setup: create a UTXO that will be spent.
	op0 := testOutPoint(0)
	s.Add(op0, testTxOut(500), 0, true)

	// Build a transaction that spends op0 and creates 2 new outputs.
	spendTx := &Transaction{
		Version: 2,
		Inputs:  []TxIn{{PrevOut: op0, SignatureScript: []byte{1}}},
		Outputs: []TxOut{
			{Amount: 300, PkScript: []byte{0xaa}},
			{Amount: 200, PkScript: []byte{0xbb}},
		},
	}
	coinbase := NewCoinbaseTx(1, 100_000_000, []byte{0xcc}, ChainIDNous)

	undo := s.ApplyBlockWithUndo([]*Transaction{coinbase, spendTx}, 1)

	// op0 should be spent.
	if s.Get(op0) != nil {
		t.Fatal("op0 should be spent")
	}

	// Coinbase output should exist.
	cbTxID := coinbase.TxID()
	cbOp := OutPoint{TxID: cbTxID, Index: 0}
	if s.Get(cbOp) == nil {
		t.Fatal("coinbase output missing")
	}

	// spendTx outputs should exist.
	spendTxID := spendTx.TxID()
	if s.Get(OutPoint{TxID: spendTxID, Index: 0}) == nil {
		t.Fatal("spendTx output 0 missing")
	}
	if s.Get(OutPoint{TxID: spendTxID, Index: 1}) == nil {
		t.Fatal("spendTx output 1 missing")
	}

	// Undo should record the spent UTXO.
	if len(undo.SpentUTXOs) != 1 {
		t.Fatalf("expected 1 spent UTXO in undo, got %d", len(undo.SpentUTXOs))
	}
	if undo.SpentUTXOs[0].SpentUTXO.Output.Amount != 500 {
		t.Fatal("undo amount mismatch")
	}
	if len(undo.CreatedTxs) != 2 {
		t.Fatalf("expected 2 created txs, got %d", len(undo.CreatedTxs))
	}
}

// Test 4: Chain reorg — apply 3 blocks, rollback 2, verify state.
func TestBoltUTXORollbackChainReorg(t *testing.T) {
	s, _ := tempBoltUTXO(t)
	defer s.Close()

	// Block 0: genesis coinbase.
	cb0 := NewCoinbaseTx(0, 100_000_000, []byte{0xaa}, ChainIDNous)
	undo0 := s.ApplyBlockWithUndo([]*Transaction{cb0}, 0)

	// Block 1: coinbase only.
	cb1 := NewCoinbaseTx(1, 100_000_000, []byte{0xbb}, ChainIDNous)
	undo1 := s.ApplyBlockWithUndo([]*Transaction{cb1}, 1)

	// Block 2: coinbase only.
	cb2 := NewCoinbaseTx(2, 100_000_000, []byte{0xcc}, ChainIDNous)
	undo2 := s.ApplyBlockWithUndo([]*Transaction{cb2}, 2)

	// Should have 3 coinbase UTXOs.
	if s.Count() != 3 {
		t.Fatalf("expected 3, got %d", s.Count())
	}

	// Rollback block 2.
	if err := s.RollbackBlock(undo2); err != nil {
		t.Fatalf("rollback block 2: %v", err)
	}
	if s.Count() != 2 {
		t.Fatalf("after rollback 2: expected 2, got %d", s.Count())
	}

	// Rollback block 1.
	if err := s.RollbackBlock(undo1); err != nil {
		t.Fatalf("rollback block 1: %v", err)
	}
	if s.Count() != 1 {
		t.Fatalf("after rollback 1: expected 1, got %d", s.Count())
	}

	// Only block 0's coinbase should remain.
	cb0TxID := cb0.TxID()
	u := s.Get(OutPoint{TxID: cb0TxID, Index: 0})
	if u == nil {
		t.Fatal("block 0 coinbase should still exist")
	}

	// Block 1 and 2 coinbases should be gone.
	if s.Get(OutPoint{TxID: cb1.TxID(), Index: 0}) != nil {
		t.Fatal("block 1 coinbase should be gone")
	}
	if s.Get(OutPoint{TxID: cb2.TxID(), Index: 0}) != nil {
		t.Fatal("block 2 coinbase should be gone")
	}

	_ = undo0 // not rolled back
}

// Test 5: Empty database — Get returns nil, no panic.
func TestBoltUTXOEmptyGet(t *testing.T) {
	s, _ := tempBoltUTXO(t)
	defer s.Close()

	u := s.Get(testOutPoint(99))
	if u != nil {
		t.Fatal("expected nil from empty db")
	}
	if s.Count() != 0 {
		t.Fatalf("expected 0 count, got %d", s.Count())
	}
}

// Test 6: Duplicate spend returns false.
func TestBoltUTXODuplicateSpend(t *testing.T) {
	s, _ := tempBoltUTXO(t)
	defer s.Close()

	op := testOutPoint(1)
	s.Add(op, testTxOut(100), 0, false)

	if !s.Spend(op) {
		t.Fatal("first Spend should succeed")
	}
	if s.Spend(op) {
		t.Fatal("second Spend should return false")
	}
}

// Test: FindByPubKeyHash and GetBalance.
func TestBoltUTXOFindAndBalance(t *testing.T) {
	s, _ := tempBoltUTXO(t)
	defer s.Close()

	pkh := []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}
	script := CreateP2PKHLockScript(pkh)

	s.Add(testOutPoint(1), TxOut{Amount: 100, PkScript: script}, 0, false)
	s.Add(testOutPoint(2), TxOut{Amount: 200, PkScript: script}, 1, false)
	s.Add(testOutPoint(3), TxOut{Amount: 50, PkScript: []byte{0xff}}, 2, false) // different script

	utxos := s.FindByPubKeyHash(pkh)
	if len(utxos) != 2 {
		t.Fatalf("expected 2 matching UTXOs, got %d", len(utxos))
	}

	bal := s.GetBalance(pkh)
	if bal != 300 {
		t.Fatalf("expected balance 300, got %d", bal)
	}
}

// Test: RebuildFromBlocks.
func TestBoltUTXORebuildFromBlocks(t *testing.T) {
	s, _ := tempBoltUTXO(t)
	defer s.Close()

	// Pre-populate some junk.
	s.Add(testOutPoint(99), testTxOut(999), 0, false)

	// Build 3 blocks worth of coinbase transactions.
	blocks := make([][]*Transaction, 3)
	for h := 0; h < 3; h++ {
		cb := NewCoinbaseTx(uint64(h), 100_000_000, []byte{byte(h)}, ChainIDNous)
		blocks[h] = []*Transaction{cb}
	}

	loadBlock := func(height uint64) ([]*Transaction, error) {
		return blocks[height], nil
	}

	if err := s.RebuildFromBlocks(loadBlock, 2); err != nil {
		t.Fatalf("RebuildFromBlocks: %v", err)
	}

	// Junk should be gone.
	if s.Get(testOutPoint(99)) != nil {
		t.Fatal("junk UTXO should be cleared")
	}

	// Should have exactly 3 coinbase UTXOs.
	if s.Count() != 3 {
		t.Fatalf("expected 3 UTXOs, got %d", s.Count())
	}
}

// Test: file removed between runs triggers rebuild path.
func TestBoltUTXOFreshFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "utxo.db")

	// Doesn't exist yet — should create.
	s, err := NewBoltUTXOSet(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.Count() != 0 {
		t.Fatal("fresh db should be empty")
	}
	s.Close()

	// File should exist.
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("utxo.db not created: %v", err)
	}
}

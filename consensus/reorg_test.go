package consensus

import (
	"math/big"
	"testing"
	"time"

	"github.com/nous-chain/nous/block"
	"github.com/nous-chain/nous/crypto"
	"github.com/nous-chain/nous/tx"
)

// easyReorgParams returns difficulty params with trivially easy PoW and tiny VDF T.
func easyReorgParams() *DifficultyParams {
	var target crypto.Hash
	for i := range target {
		target[i] = 0xFF
	}
	return &DifficultyParams{
		VDFIterations: 100,
		CSPDifficulty: CSPDifficultyParams{
			BaseVariables:   5,
			ConstraintRatio: 1.0,
		},
		PoWTarget: target,
	}
}

// mineReorgBlock mines a single block using the test helpers.
func mineReorgBlock(
	t *testing.T,
	prevHeader *block.Header,
	priv *crypto.PrivateKey,
	pub *crypto.PublicKey,
	params *DifficultyParams,
	height uint64,
) *block.Block {
	t.Helper()
	blk, err := MineBlock(prevHeader, nil, priv, pub, params, height, nil)
	if err != nil {
		t.Fatalf("mine block at height %d: %v", height, err)
	}
	return blk
}

// buildChain mines a chain of blocks starting from prevHeader at startHeight.
// Returns the blocks in order.
func buildChain(
	t *testing.T,
	prevHeader *block.Header,
	startHeight uint64,
	count int,
	priv *crypto.PrivateKey,
	pub *crypto.PublicKey,
	params *DifficultyParams,
) []*block.Block {
	t.Helper()
	blocks := make([]*block.Block, count)
	hdr := prevHeader
	for i := 0; i < count; i++ {
		h := startHeight + uint64(i)
		blk := mineReorgBlock(t, hdr, priv, pub, params, h)
		blocks[i] = blk
		hdr = &blk.Header
	}
	return blocks
}

// ============================================================
// TestReorg: heavier fork triggers chain switch
// ============================================================

func TestReorg(t *testing.T) {
	// Generate two miners (so they produce different VDF outputs → different blocks).
	privA, pubA, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	privB, pubB, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pkhA := crypto.Hash160(pubA.SerializeCompressed())

	params := easyReorgParams()

	// Create genesis and chain state.
	genesis := block.GenesisBlock(pkhA, uint32(time.Now().Unix())-120)
	cs := NewChainState(genesis)
	cs.Difficulty = params

	// Build main chain A: 10 blocks (heights 1-10) by miner A.
	chainA := buildChain(t, &genesis.Header, 1, 10, privA, pubA, params)
	for _, blk := range chainA {
		if err := cs.AddBlock(blk); err != nil {
			t.Fatalf("add chain A block: %v", err)
		}
	}

	if cs.Height != 10 {
		t.Fatalf("expected height 10, got %d", cs.Height)
	}
	tipA := cs.Tip.Hash()
	t.Logf("Chain A tip at height 10: %x", tipA[:8])

	// Record coinbase UTXOs from chain A blocks 6-10.
	aCoinbaseTxIDs := make([]crypto.Hash, 5)
	for i := 5; i < 10; i++ {
		aCoinbaseTxIDs[i-5] = chainA[i].Transactions[0].TxID()
	}

	// Build fork chain B: from block 5's header, 8 blocks (heights 6-13) by miner B.
	// To make B heavier, use a harder difficulty bits value in the blocks.
	// We'll build them with the same easy params but manually boost chain work.
	forkPoint := chainA[4] // block at height 5
	chainB := buildChain(t, &forkPoint.Header, 6, 8, privB, pubB, params)

	// Track disconnected blocks during reorg.
	var disconnected []*block.Block
	cs.OnReorg = func(blk *block.Block) {
		disconnected = append(disconnected, blk)
	}

	// To trigger reorg, chain B must have higher ChainWork than chain A.
	// Chain A: genesis + 10 blocks of work.
	// Chain B will diverge at height 5 (5 shared blocks) + 8 new blocks.
	// With same difficulty, B has 13 blocks total vs A's 10, so B is heavier.
	// But AddBlock for side-chain blocks needs to handle UTXO validation.
	// We add the B blocks one by one.
	for i, blk := range chainB {
		err := cs.AddBlock(blk)
		if err != nil {
			t.Logf("add chain B block %d (height %d): %v", i, 6+i, err)
			// Side chain blocks may fail main-chain UTXO validation but
			// still get stored and trigger reorg.
		}
	}

	// After adding all B blocks, the heavier chain should be active.
	tipAfter := cs.Tip.Hash()
	t.Logf("After reorg: height=%d tip=%x", cs.Height, tipAfter[:8])

	// Verify: tip is the last block of chain B (height 13).
	if cs.Height != 13 {
		t.Fatalf("expected height 13 after reorg, got %d", cs.Height)
	}
	expectedTipB := chainB[len(chainB)-1].Header.Hash()
	if cs.Tip.Hash() != expectedTipB {
		t.Fatalf("tip should be chain B's last block")
	}

	// Verify: chain A's blocks 6-10 coinbase UTXOs should NOT exist.
	for i, txid := range aCoinbaseTxIDs {
		op := tx.OutPoint{TxID: txid, Index: 0}
		if cs.UTXOSet.Get(op) != nil {
			t.Errorf("chain A coinbase at height %d should have been rolled back", 6+i)
		}
	}

	// Verify: chain B's blocks 6-13 coinbase UTXOs should exist.
	for i, blk := range chainB {
		txid := blk.Transactions[0].TxID()
		op := tx.OutPoint{TxID: txid, Index: 0}
		if cs.UTXOSet.Get(op) == nil {
			t.Errorf("chain B coinbase at height %d should exist in UTXO set", 6+i)
		}
	}

	// Verify: some blocks were disconnected.
	if len(disconnected) == 0 {
		t.Error("expected disconnected blocks from reorg callback")
	}
	t.Logf("Disconnected %d blocks during reorg", len(disconnected))
}

// ============================================================
// TestNoReorgWhenShorter: lighter fork does not trigger switch
// ============================================================

func TestNoReorgWhenShorter(t *testing.T) {
	privA, pubA, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	privB, pubB, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pkhA := crypto.Hash160(pubA.SerializeCompressed())

	params := easyReorgParams()

	genesis := block.GenesisBlock(pkhA, uint32(time.Now().Unix())-120)
	cs := NewChainState(genesis)
	cs.Difficulty = params

	// Build main chain A: 10 blocks.
	chainA := buildChain(t, &genesis.Header, 1, 10, privA, pubA, params)
	for _, blk := range chainA {
		if err := cs.AddBlock(blk); err != nil {
			t.Fatalf("add chain A block: %v", err)
		}
	}

	tipBeforeReorg := cs.Tip.Hash()
	heightBefore := cs.Height
	t.Logf("Chain A: height=%d tip=%x", heightBefore, tipBeforeReorg[:8])

	// Build shorter fork B: 3 blocks from height 5 (heights 6-8).
	forkPoint := chainA[4]
	chainB := buildChain(t, &forkPoint.Header, 6, 3, privB, pubB, params)

	for _, blk := range chainB {
		_ = cs.AddBlock(blk) // may fail UTXO validation; that's OK
	}

	// Verify: tip should NOT have changed.
	if cs.Height != heightBefore {
		t.Fatalf("height should remain %d, got %d", heightBefore, cs.Height)
	}
	if cs.Tip.Hash() != tipBeforeReorg {
		t.Fatal("tip should not change for lighter fork")
	}
	t.Logf("No reorg: tip still at height %d", cs.Height)
}

// ============================================================
// TestUTXORollback: basic undo/rollback correctness
// ============================================================

func TestUTXORollback(t *testing.T) {
	// Create a simple UTXO set and apply/rollback a block.
	utxos := tx.NewUTXOSet()

	// Create a coinbase tx.
	pkh := make([]byte, 20)
	pkh[0] = 0x42
	cb := tx.NewCoinbase(0, 1000, pkh, "test")
	cbTxID := cb.TxID()

	// Apply genesis.
	undo0 := utxos.ApplyBlockWithUndo([]*tx.Transaction{cb}, 0)
	if utxos.Count() != 1 {
		t.Fatalf("expected 1 UTXO after genesis, got %d", utxos.Count())
	}

	// Create a spending tx.
	spendTx := &tx.Transaction{
		Version: 1,
		Inputs: []tx.TxInput{
			{PrevOut: tx.OutPoint{TxID: cbTxID, Index: 0}, Sequence: 0xFFFFFFFF},
		},
		Outputs: []tx.TxOutput{
			{Value: 500, ScriptPubKey: tx.CreateP2PKHLockScript(pkh)},
			{Value: 500, ScriptPubKey: tx.CreateP2PKHLockScript(pkh)},
		},
	}

	// Apply block 1.
	cb1 := tx.NewCoinbase(1, 1000, pkh, "")
	undo1 := utxos.ApplyBlockWithUndo([]*tx.Transaction{cb1, spendTx}, 1)

	// Should have 3 UTXOs: 2 from spendTx + 1 from cb1 (original cb spent).
	if utxos.Count() != 3 {
		t.Fatalf("expected 3 UTXOs after block 1, got %d", utxos.Count())
	}

	// Original coinbase should be spent.
	if utxos.Get(tx.OutPoint{TxID: cbTxID, Index: 0}) != nil {
		t.Fatal("original coinbase should be spent")
	}

	// Rollback block 1.
	if err := utxos.RollbackBlock(undo1); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	// Should be back to 1 UTXO (the original coinbase).
	if utxos.Count() != 1 {
		t.Fatalf("expected 1 UTXO after rollback, got %d", utxos.Count())
	}
	if utxos.Get(tx.OutPoint{TxID: cbTxID, Index: 0}) == nil {
		t.Fatal("original coinbase should be restored after rollback")
	}

	// Rollback genesis.
	if err := utxos.RollbackBlock(undo0); err != nil {
		t.Fatalf("rollback genesis: %v", err)
	}
	if utxos.Count() != 0 {
		t.Fatalf("expected 0 UTXOs after full rollback, got %d", utxos.Count())
	}
}

// ============================================================
// TestFindForkPoint
// ============================================================

func TestFindForkPoint(t *testing.T) {
	// Build a tree:  root → a1 → a2 → a3
	//                     ↘ b1 → b2
	root := &blockNode{Height: 0, ChainWork: big.NewInt(1)}
	a1 := &blockNode{Height: 1, Parent: root, ChainWork: big.NewInt(2)}
	a2 := &blockNode{Height: 2, Parent: a1, ChainWork: big.NewInt(3)}
	a3 := &blockNode{Height: 3, Parent: a2, ChainWork: big.NewInt(4)}
	b1 := &blockNode{Height: 1, Parent: root, ChainWork: big.NewInt(2)}
	b2 := &blockNode{Height: 2, Parent: b1, ChainWork: big.NewInt(3)}

	fp := findForkPoint(a3, b2)
	if fp != root {
		t.Fatal("fork point should be root")
	}

	fp2 := findForkPoint(a3, a2)
	if fp2 != a2 {
		t.Fatal("fork point of a3 and a2 should be a2")
	}

	fp3 := findForkPoint(a1, a1)
	if fp3 != a1 {
		t.Fatal("fork point of same node should be itself")
	}
}

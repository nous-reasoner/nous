package consensus

import (
	"math/big"
	"testing"
	"time"

	"nous/block"
	"nous/crypto"
	"nous/sat"
	"nous/tx"
)

// testParams returns difficulty params with trivially easy PoW.
func testParams() *DifficultyParams {
	var easyTarget crypto.Hash
	for i := range easyTarget {
		easyTarget[i] = 0xFF
	}
	return &DifficultyParams{
		PoWTarget: easyTarget,
	}
}

// makeGenesis creates a genesis block for testing.
// Uses a timestamp 60 seconds in the past so that MineBlock (which uses
// time.Now()) always produces a strictly later timestamp.
func makeGenesis(pubKeyHash []byte) *block.Block {
	return block.GenesisBlock(pubKeyHash, uint32(time.Now().Unix())-60, 0x1d00ffff, false)
}

// mineTestBlock is a helper that mines a block with easy PoW.
func mineTestBlock(
	t *testing.T,
	prevHeader *block.Header,
	pubKeyHash []byte,
	params *DifficultyParams,
	height uint64,
) *block.Block {
	t.Helper()
	blk, err := MineBlock(prevHeader, nil, pubKeyHash, params, height, nil, false)
	if err != nil {
		t.Fatalf("MineBlock failed at height %d: %v", height, err)
	}
	return blk
}

// ============================================================
// 1. BlockReward: constant 1 NOUS (infinite supply)
// ============================================================

func TestBlockRewardConstantEmission(t *testing.T) {
	tests := []struct {
		height uint64
		reward int64
	}{
		{0, 1_00000000},
		{1, 1_00000000},
		{1_050_000, 1_00000000},
		{2_100_000_000, 1_00000000},
		{10_000_000_000, 1_00000000},
	}
	for _, tc := range tests {
		got := BlockReward(tc.height)
		if got != tc.reward {
			t.Errorf("BlockReward(%d): want %d, got %d", tc.height, tc.reward, got)
		}
	}
}

// ============================================================
// 2. ASERT: blocks too fast → target decreases (harder)
// ============================================================

func TestASERTBlocksTooFast(t *testing.T) {
	anchor := &ASERTAnchor{
		Height:    0,
		Timestamp: 1000000,
		Target:    DefaultDifficultyParams().PoWTarget,
	}

	// Block at height 100, but timestamp is only 50s after genesis.
	// Expected timestamp = 1000000 + 150*100 = 1015000.
	// Actual = 1000050 → timeDiff = -14950 (blocks way too fast).
	target := AdjustDifficultyASERT(anchor, 100, 1000050)

	if target.Compare(anchor.Target) >= 0 {
		t.Fatal("target should decrease (harder) when blocks are too fast")
	}
}

// ============================================================
// 3. ASERT: blocks too slow → target increases (easier)
// ============================================================

func TestASERTBlocksTooSlow(t *testing.T) {
	anchor := &ASERTAnchor{
		Height:    0,
		Timestamp: 1000000,
		Target:    DefaultDifficultyParams().PoWTarget,
	}

	// Block at height 100, but timestamp is 2x the expected time.
	// Expected timestamp = 1000000 + 150*100 = 1015000.
	// Use 1030000 → timeDiff = +15000 (blocks way too slow).
	target := AdjustDifficultyASERT(anchor, 100, 1030000)

	if target.Compare(anchor.Target) <= 0 {
		t.Fatal("target should increase (easier) when blocks are too slow")
	}
}

// ============================================================
// 4. ASERT: on-schedule blocks produce near-anchor target
// ============================================================

func TestASERTOnSchedule(t *testing.T) {
	anchor := &ASERTAnchor{
		Height:    0,
		Timestamp: 1000000,
		Target:    DefaultDifficultyParams().PoWTarget,
	}

	// Block at height 100, exactly on schedule.
	// Expected = 1000000 + 150*100 = 1015000.
	target := AdjustDifficultyASERT(anchor, 100, 1015000)

	// Target should equal the anchor target (timeDiff = 0).
	if target != anchor.Target {
		t.Fatalf("on-schedule target should equal anchor target, got %x vs %x",
			target[:8], anchor.Target[:8])
	}
}

// ============================================================
// 5. ASERT: halflife doubling/halving
// ============================================================

func TestASERTHalflife(t *testing.T) {
	anchor := &ASERTAnchor{
		Height:    0,
		Timestamp: 1000000,
		Target:    DefaultDifficultyParams().PoWTarget,
	}

	// If timestamp is exactly one halflife ahead of schedule,
	// target should approximately double.
	// Expected for height 1 = 1000000 + 150 = 1000150.
	// Set timestamp = 1000150 + 43200 = 1043350.
	target := AdjustDifficultyASERT(anchor, 1, 1043350)

	anchorBig := new(big.Int).SetBytes(anchor.Target[:])
	targetBig := new(big.Int).SetBytes(target[:])

	// ratio = target / anchor should be ~2.0.
	// Use fixed-point: ratio*1000 = target*1000/anchor.
	ratio1000 := new(big.Int).Mul(targetBig, big.NewInt(1000))
	ratio1000.Div(ratio1000, anchorBig)

	// Allow 1% tolerance: 1980..2020.
	r := ratio1000.Int64()
	if r < 1980 || r > 2020 {
		t.Fatalf("expected ~2.0x ratio after +1 halflife, got %.3f", float64(r)/1000)
	}
}

// ============================================================
// 6. Full mining: MineBlock → ValidateBlock
// ============================================================

func TestMineBlockAndValidate(t *testing.T) {
	_, pub, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pubKeyHash := crypto.Hash160(pub.SerializeCompressed())
	genesis := makeGenesis(pubKeyHash)
	params := testParams()

	blk := mineTestBlock(t, &genesis.Header, pubKeyHash, params, 1)

	utxos := tx.NewUTXOSet()
	utxos.ApplyBlock(genesis.Transactions, 0)

	if err := ValidateBlock(blk, &genesis.Header, params, utxos, 1, false); err != nil {
		t.Fatalf("ValidateBlock failed: %v", err)
	}
}

// ============================================================
// 7. Validation rejects tampered SAT solution
// ============================================================

func TestValidateRejectsTamperedSAT(t *testing.T) {
	_, pub, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pubKeyHash := crypto.Hash160(pub.SerializeCompressed())
	genesis := makeGenesis(pubKeyHash)
	params := testParams()

	blk := mineTestBlock(t, &genesis.Header, pubKeyHash, params, 1)

	// Tamper with SAT solution.
	if len(blk.SATSolution) > 0 {
		blk.SATSolution[0] = !blk.SATSolution[0]
	}

	utxos := tx.NewUTXOSet()
	utxos.ApplyBlock(genesis.Transactions, 0)

	err = ValidateBlock(blk, &genesis.Header, params, utxos, 1, false)
	if err == nil {
		t.Fatal("should reject tampered SAT solution")
	}
}

// ============================================================
// 8. Validation rejects bad PoW (hash doesn't meet target)
// ============================================================

func TestValidateRejectsBadPoW(t *testing.T) {
	_, pub, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pubKeyHash := crypto.Hash160(pub.SerializeCompressed())
	genesis := makeGenesis(pubKeyHash)

	// Use a very hard target (practically impossible to satisfy randomly).
	params := testParams()
	var hardTarget crypto.Hash
	hardTarget[0] = 0x00
	hardTarget[1] = 0x00
	hardTarget[2] = 0x01 // very small target
	params.PoWTarget = hardTarget

	// Mine with easy target first, then validate with hard.
	easyParams := testParams()
	blk := mineTestBlock(t, &genesis.Header, pubKeyHash, easyParams, 1)

	utxos := tx.NewUTXOSet()
	utxos.ApplyBlock(genesis.Transactions, 0)

	err = ValidateBlock(blk, &genesis.Header, params, utxos, 1, false)
	if err == nil {
		t.Fatal("should reject block that doesn't meet hard PoW target")
	}
}

// ============================================================
// 9. Validation rejects excess coinbase reward
// ============================================================

func TestValidateRejectsExcessCoinbase(t *testing.T) {
	_, pub, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pubKeyHash := crypto.Hash160(pub.SerializeCompressed())
	genesis := makeGenesis(pubKeyHash)
	params := testParams()

	blk := mineTestBlock(t, &genesis.Header, pubKeyHash, params, 1)

	// Inflate coinbase reward.
	blk.Transactions[0].Outputs[0].Amount = 2_00000000 // 2 NOUS instead of 1

	utxos := tx.NewUTXOSet()
	utxos.ApplyBlock(genesis.Transactions, 0)

	err = ValidateBlock(blk, &genesis.Header, params, utxos, 1, false)
	if err == nil {
		t.Fatal("should reject inflated coinbase reward")
	}
}

// ============================================================
// Helper: verify compact target round-trip
// ============================================================

func TestCompactTargetRoundTrip(t *testing.T) {
	bits := uint32(0x1d00ffff)
	target := CompactToTarget(bits)
	if target.IsZero() {
		t.Fatal("target should not be zero")
	}

	back := TargetToCompact(target)
	target2 := CompactToTarget(back)
	if target != target2 {
		t.Fatalf("round-trip mismatch: %s vs %s", target, target2)
	}
}

// ============================================================
// SAT solution is valid and verifiable
// ============================================================

func TestMineBlockProducesValidSAT(t *testing.T) {
	_, pub, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pubKeyHash := crypto.Hash160(pub.SerializeCompressed())
	genesis := makeGenesis(pubKeyHash)
	params := testParams()

	blk := mineTestBlock(t, &genesis.Header, pubKeyHash, params, 1)

	// Block must contain a SAT solution.
	if len(blk.SATSolution) == 0 {
		t.Fatal("block has no SAT solution")
	}

	// Regenerate the formula and verify.
	prevHash := genesis.Header.Hash()
	satSeed := MakeSATSeed(prevHash, blk.Header.Seed)
	formula := sat.GenerateFormula(satSeed, SATVariables, SATClausesRatio)

	if !sat.Verify(formula, blk.SATSolution) {
		t.Fatal("SAT solution does not verify against regenerated formula")
	}

	// Verify solution hash in header matches.
	solBytes := sat.SerializeAssignment(blk.SATSolution)
	solHash := crypto.Sha256(solBytes)
	if solHash != blk.Header.SATSolutionHash {
		t.Fatalf("SAT solution hash mismatch: header=%x computed=%x",
			blk.Header.SATSolutionHash[:8], solHash[:8])
	}
}

// ============================================================
// Cross-tx double-spend within a single block is rejected
// ============================================================

func TestValidateRejectsCrossTxDoubleSpend(t *testing.T) {
	privA, pubA, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pkhA := crypto.Hash160(pubA.SerializeCompressed())

	// Mine block 1 so miner A has a spendable UTXO.
	genesis := makeGenesis(pkhA)
	params := testParams()
	blk1 := mineTestBlock(t, &genesis.Header, pkhA, params, 1)

	utxos := tx.NewUTXOSet()
	utxos.ApplyBlock(genesis.Transactions, 0)
	utxos.ApplyBlock(blk1.Transactions, 1)

	// Identify the coinbase UTXO from block 1.
	cb1ID := blk1.Transactions[0].TxID()
	cbOutPoint := tx.OutPoint{TxID: cb1ID, Index: 0}

	// Build two transactions that spend the same coinbase UTXO.
	buildSpendTx := func(value int64) *tx.Transaction {
		spendTx := &tx.Transaction{
			Version: 2,
			ChainID: tx.ChainIDNous,
			Inputs: []tx.TxIn{
				{PrevOut: cbOutPoint, Sequence: 0xFFFFFFFF},
			},
			Outputs: []tx.TxOut{
				{Amount: value, PkScript: tx.CreateP2PKHLockScript(pkhA)},
			},
		}
		subscript := blk1.Transactions[0].Outputs[0].PkScript
		sigHash := spendTx.SigHash(0, subscript)
		sig, _ := crypto.Sign(privA, sigHash)
		spendTx.Inputs[0].SignatureScript = tx.CreateP2PKHUnlockScript(
			sig.Bytes(), pubA.SerializeCompressed())
		return spendTx
	}

	spendA := buildSpendTx(5000_0000)
	spendB := buildSpendTx(4000_0000)

	// Mine block 2 with both txs included.
	blk2 := mineTestBlock(t, &blk1.Header, pkhA, params, 2)

	// Replace block 2's transactions: keep coinbase, add two double-spends.
	blk2.Transactions = append(blk2.Transactions[:1], spendA, spendB)

	// Recompute merkle root to match the tampered tx list.
	txIDs := make([]crypto.Hash, len(blk2.Transactions))
	for i, t := range blk2.Transactions {
		txIDs[i] = t.TxID()
	}
	blk2.Header.MerkleRoot = block.ComputeMerkleRoot(txIDs)

	err = ValidateBlock(blk2, &blk1.Header, params, utxos, 2, false)
	if err == nil {
		t.Fatal("block with cross-tx double-spend should be rejected")
	}
}

// ============================================================
// Block size limit
// ============================================================

func TestValidateRejectsOversizedBlock(t *testing.T) {
	genesis := makeGenesis(make([]byte, 20))
	chain := NewChainState(genesis)
	params := DefaultDifficultyParams()

	bigScript := make([]byte, block.MaxBlockSize+1)
	coinbase := tx.NewCoinbaseTx(1, 1_00000000, tx.CreateP2PKHLockScript(make([]byte, 20)), tx.ChainIDNous)
	coinbase.Outputs = append(coinbase.Outputs, tx.TxOut{
		Amount:   0,
		PkScript: bigScript,
	})

	blk := &block.Block{
		Header: block.Header{
			Version:       1,
			PrevBlockHash: genesis.Header.Hash(),
			Timestamp:     uint32(time.Now().Unix()),
		},
		Transactions: []*tx.Transaction{coinbase},
	}

	err := ValidateBlock(blk, &genesis.Header, params, chain.UTXOSet, 1, false)
	if err == nil {
		t.Fatal("oversized block should be rejected")
	}
}

// ============================================================
// Block with too many transactions rejected
// ============================================================

func TestValidateRejectsTooManyTransactions(t *testing.T) {
	genesis := makeGenesis(make([]byte, 20))
	chain := NewChainState(genesis)
	params := testParams()

	coinbase := tx.NewCoinbaseTx(1, 1_00000000, tx.CreateP2PKHLockScript(make([]byte, 20)), tx.ChainIDNous)
	txs := make([]*tx.Transaction, block.MaxBlockTransactions+1)
	txs[0] = coinbase
	for i := 1; i < len(txs); i++ {
		txs[i] = &tx.Transaction{Version: 1}
	}

	blk := &block.Block{
		Header: block.Header{
			Version:       1,
			PrevBlockHash: genesis.Header.Hash(),
			Timestamp:     uint32(time.Now().Unix()),
		},
		Transactions: txs,
	}

	err := ValidateBlock(blk, &genesis.Header, params, chain.UTXOSet, 1, false)
	if err == nil {
		t.Fatal("block with too many transactions should be rejected")
	}
}

package consensus

import (
	"testing"
	"time"

	"github.com/nous-chain/nous/block"
	"github.com/nous-chain/nous/crypto"
	"github.com/nous-chain/nous/csp"
	"github.com/nous-chain/nous/tx"
	"github.com/nous-chain/nous/vdf"
)

// testVDFT is a small T value for fast tests (VDF is slow at real values).
const testVDFT = 100

// testParams returns difficulty params with very easy PoW and small VDF T.
func testParams() *DifficultyParams {
	// All-0xFF target = trivially easy PoW.
	var easyTarget crypto.Hash
	for i := range easyTarget {
		easyTarget[i] = 0xFF
	}
	return &DifficultyParams{
		VDFIterations: testVDFT,
		CSPDifficulty: CSPDifficultyParams{
			BaseVariables:   5,
			ConstraintRatio: 1.2,
		},
		PoWTarget: easyTarget,
	}
}

// makeGenesis creates a genesis block for testing.
// Uses a timestamp 60 seconds in the past so that MineBlock (which uses
// time.Now()) always produces a strictly later timestamp.
func makeGenesis(pubKeyHash []byte) *block.Block {
	return block.GenesisBlock(pubKeyHash, uint32(time.Now().Unix())-60)
}

// mineTestBlock is a helper that mines a block with small VDF T and easy PoW.
func mineTestBlock(
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
		t.Fatalf("MineBlock failed at height %d: %v", height, err)
	}
	return blk
}

// ============================================================
// 1. BlockReward: constant emission until MaxTotalSupply
// ============================================================

func TestBlockRewardConstantEmission(t *testing.T) {
	tests := []struct {
		height uint64
		reward int64
	}{
		{0, 10_0000_0000},                 // first block
		{1, 10_0000_0000},                 // second block
		{1_050_000, 10_0000_0000},         // still constant (no halving)
		{2_099_999_999, 10_0000_0000},     // last full reward block
		{2_100_000_000, 0},                // supply exhausted
		{2_100_000_001, 0},                // well past cap
		{10_000_000_000, 0},               // far future
	}
	for _, tc := range tests {
		got := BlockReward(tc.height)
		if got != tc.reward {
			t.Errorf("BlockReward(%d): want %d, got %d", tc.height, tc.reward, got)
		}
	}
}

// ============================================================
// 2. Difficulty adjustment: blocks too fast → difficulty up
// ============================================================

func TestAdjustDifficultyBlocksTooFast(t *testing.T) {
	params := DefaultDifficultyParams()

	// Simulate 144 blocks in half the expected time.
	chain := make([]BlockInfo, 145)
	expectedSpan := 144 * TargetBlockTime
	actualSpan := expectedSpan / 2 // 2x too fast
	for i := range chain {
		chain[i] = BlockInfo{
			Timestamp: uint32(1000000 + uint64(i)*actualSpan/144),
		}
	}

	next := AdjustDifficulty(params, chain, 144)

	// VDF iterations should increase (blocks too fast → need more work).
	if next.VDFIterations <= params.VDFIterations {
		t.Fatalf("VDF iterations should increase: was %d, got %d",
			params.VDFIterations, next.VDFIterations)
	}

	// PoW target should decrease (harder).
	if next.PoWTarget.Compare(params.PoWTarget) >= 0 {
		t.Fatal("PoW target should decrease (harder) when blocks are too fast")
	}
}

// ============================================================
// 3. Difficulty adjustment: blocks too slow → difficulty down
// ============================================================

func TestAdjustDifficultyBlocksTooSlow(t *testing.T) {
	params := DefaultDifficultyParams()

	// Simulate 144 blocks in double the expected time.
	chain := make([]BlockInfo, 145)
	expectedSpan := 144 * TargetBlockTime
	actualSpan := expectedSpan * 2 // 2x too slow
	for i := range chain {
		chain[i] = BlockInfo{
			Timestamp: uint32(1000000 + uint64(i)*actualSpan/144),
		}
	}

	next := AdjustDifficulty(params, chain, 144)

	// VDF iterations should decrease.
	if next.VDFIterations >= params.VDFIterations {
		t.Fatalf("VDF iterations should decrease: was %d, got %d",
			params.VDFIterations, next.VDFIterations)
	}

	// PoW target should increase (easier).
	if next.PoWTarget.Compare(params.PoWTarget) <= 0 {
		t.Fatal("PoW target should increase (easier) when blocks are too slow")
	}
}

// ============================================================
// 4. Extreme case triggers 50% reduction
// ============================================================

func TestAdjustDifficultyExtreme(t *testing.T) {
	params := DefaultDifficultyParams()

	// Simulate 144 blocks in 15x the expected time (extreme).
	chain := make([]BlockInfo, 145)
	expectedSpan := 144 * TargetBlockTime
	actualSpan := expectedSpan * 15
	for i := range chain {
		chain[i] = BlockInfo{
			Timestamp: uint32(1000000 + uint64(i)*actualSpan/144),
		}
	}

	next := AdjustDifficulty(params, chain, 144)

	// With 15x slowdown, PoW target increase should exceed 25% (normal cap).
	// The extreme cap allows up to 2x (100% increase).
	targetRatio := float64(0)
	// Compare as big ints.
	import_big := func(h crypto.Hash) float64 {
		var sum float64
		for _, b := range h {
			sum = sum*256 + float64(b)
		}
		return sum
	}
	oldT := import_big(params.PoWTarget)
	newT := import_big(next.PoWTarget)
	if oldT > 0 {
		targetRatio = newT / oldT
	}

	// Should be more than 1.25 (normal cap) but at most 2.0.
	if targetRatio <= 1.25 {
		t.Fatalf("extreme case should exceed 25%% cap: ratio = %.2f", targetRatio)
	}
	if targetRatio > 2.05 { // small tolerance for rounding
		t.Fatalf("extreme case should not exceed 2x: ratio = %.2f", targetRatio)
	}
}

// ============================================================
// 5. ±25% cap enforced
// ============================================================

func TestAdjustDifficulty25PercentCap(t *testing.T) {
	params := DefaultDifficultyParams()

	// 5x too fast → ratio=0.2, adjustment would be 5x, but capped at 1.25.
	chain := make([]BlockInfo, 145)
	expectedSpan := 144 * TargetBlockTime
	actualSpan := expectedSpan / 5
	for i := range chain {
		chain[i] = BlockInfo{
			Timestamp: uint32(1000000 + uint64(i)*actualSpan/144),
		}
	}

	next := AdjustDifficulty(params, chain, 144)

	// VDF increase should be capped at 25%.
	maxExpected := float64(params.VDFIterations) * 1.26 // slight tolerance
	if float64(next.VDFIterations) > maxExpected {
		t.Fatalf("VDF increase should be capped at ~25%%: was %d, got %d (max ~%.0f)",
			params.VDFIterations, next.VDFIterations, maxExpected)
	}
}

// ============================================================
// 5. VDF minimum floor: extreme reduction cannot go below MinVDFIterations
// ============================================================

func TestAdjustVDFMinimumFloor(t *testing.T) {
	// Start with a small VDF iteration count (2000).
	params := DefaultDifficultyParams()
	params.VDFIterations = 2000

	// Simulate extreme slowdown: 15x expected time.
	// adjustment = 1/15 ≈ 0.067, so 2000 * 0.067 ≈ 133 < MinVDFIterations.
	chain := make([]BlockInfo, 145)
	expectedSpan := 144 * TargetBlockTime
	actualSpan := expectedSpan * 15
	for i := range chain {
		chain[i] = BlockInfo{
			Timestamp: uint32(1000000 + uint64(i)*actualSpan/144),
		}
	}

	next := AdjustDifficulty(params, chain, 144)

	if next.VDFIterations < MinVDFIterations {
		t.Fatalf("VDF iterations fell below MinVDFIterations: got %d, min %d",
			next.VDFIterations, MinVDFIterations)
	}
	if next.VDFIterations != MinVDFIterations {
		t.Fatalf("VDF iterations should be clamped to MinVDFIterations: got %d, want %d",
			next.VDFIterations, MinVDFIterations)
	}
	t.Logf("VDF floor enforced: 2000 → %d (min=%d)", next.VDFIterations, MinVDFIterations)
}

// ============================================================
// 6. Full mining: MineBlock → ValidateBlock
// ============================================================

func TestMineBlockAndValidate(t *testing.T) {
	priv, pub, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pubKeyHash := crypto.Hash160(pub.SerializeCompressed())
	genesis := makeGenesis(pubKeyHash)
	params := testParams()

	blk := mineTestBlock(t, &genesis.Header, priv, pub, params, 1)

	utxos := tx.NewUTXOSet()
	utxos.ApplyBlock(genesis.Transactions, 0)

	if err := ValidateBlock(blk, &genesis.Header, params, utxos, 1); err != nil {
		t.Fatalf("ValidateBlock failed: %v", err)
	}
}

// ============================================================
// 7. Validation rejects tampered VDF proof
// ============================================================

func TestValidateRejectsTamperedVDF(t *testing.T) {
	priv, pub, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pubKeyHash := crypto.Hash160(pub.SerializeCompressed())
	genesis := makeGenesis(pubKeyHash)
	params := testParams()

	blk := mineTestBlock(t, &genesis.Header, priv, pub, params, 1)

	// Tamper with VDF proof.
	blk.Header.VDFProof[len(blk.Header.VDFProof)/2] ^= 0x01

	utxos := tx.NewUTXOSet()
	utxos.ApplyBlock(genesis.Transactions, 0)

	err = ValidateBlock(blk, &genesis.Header, params, utxos, 1)
	if err == nil {
		t.Fatal("should reject tampered VDF proof")
	}
}

// ============================================================
// 8. Validation rejects tampered CSP solution
// ============================================================

func TestValidateRejectsTamperedCSP(t *testing.T) {
	priv, pub, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pubKeyHash := crypto.Hash160(pub.SerializeCompressed())
	genesis := makeGenesis(pubKeyHash)
	params := testParams()

	blk := mineTestBlock(t, &genesis.Header, priv, pub, params, 1)

	// Tamper with CSP solution (change a value).
	if len(blk.CSPSolution.Values) > 0 {
		blk.CSPSolution.Values[0] += 999
	}

	utxos := tx.NewUTXOSet()
	utxos.ApplyBlock(genesis.Transactions, 0)

	err = ValidateBlock(blk, &genesis.Header, params, utxos, 1)
	if err == nil {
		t.Fatal("should reject tampered CSP solution")
	}
}

// ============================================================
// 9. Validation rejects bad nonce (PoW doesn't meet target)
// ============================================================

func TestValidateRejectsBadNonce(t *testing.T) {
	priv, pub, err := crypto.GenerateKeyPair()
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

	// Mine with easy target first (to get a valid block), then validate with hard.
	easyParams := testParams()
	blk := mineTestBlock(t, &genesis.Header, priv, pub, easyParams, 1)

	utxos := tx.NewUTXOSet()
	utxos.ApplyBlock(genesis.Transactions, 0)

	// The block was mined with easy target; validating with hard target should fail.
	err = ValidateBlock(blk, &genesis.Header, params, utxos, 1)
	if err == nil {
		t.Fatal("should reject block that doesn't meet hard PoW target")
	}
}

// ============================================================
// 10. Validation rejects excess coinbase reward
// ============================================================

func TestValidateRejectsExcessCoinbase(t *testing.T) {
	priv, pub, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pubKeyHash := crypto.Hash160(pub.SerializeCompressed())
	genesis := makeGenesis(pubKeyHash)
	params := testParams()

	blk := mineTestBlock(t, &genesis.Header, priv, pub, params, 1)

	// Inflate coinbase reward.
	blk.Transactions[0].Outputs[0].Value = 20_0000_0000 // 20 NOUS instead of 10

	utxos := tx.NewUTXOSet()
	utxos.ApplyBlock(genesis.Transactions, 0)

	err = ValidateBlock(blk, &genesis.Header, params, utxos, 1)
	if err == nil {
		t.Fatal("should reject inflated coinbase reward")
	}
}

// ============================================================
// Helper: verify compact target round-trip
// ============================================================

func TestCompactTargetRoundTrip(t *testing.T) {
	// Standard Bitcoin genesis target.
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
// Helper: HashSolutionValues deterministic
// ============================================================

func TestHashSolutionDeterministic(t *testing.T) {
	vals := []int{1, 2, 3, 4, 5}
	h1 := HashSolutionValues(vals)
	h2 := HashSolutionValues(vals)
	if h1 != h2 {
		t.Fatal("HashSolutionValues should be deterministic")
	}
	if h1.IsZero() {
		t.Fatal("hash should not be zero")
	}
}

// ============================================================
// Helper: BruteForceSolve works for generated problems
// ============================================================

func TestBruteForceSolve(t *testing.T) {
	// Create a tiny hand-crafted problem that brute force can handle.
	prob := &csp.Problem{
		Variables: []csp.Variable{
			{Name: "x0", Lower: 1, Upper: 3},
			{Name: "x1", Lower: 1, Upper: 3},
		},
		Constraints: []csp.Constraint{
			// 1*X + 1*Y = 4 → (1,3), (2,2), (3,1)
			{Type: csp.CtLinear, Vars: []int{0, 1}, Params: []int{1, 1, 4}},
		},
		Level: csp.Standard,
	}

	sol := BruteForceSolve(prob)
	if sol == nil {
		t.Fatal("BruteForceSolve should find a solution for a tiny problem")
	}
	if !csp.VerifySolution(prob, sol) {
		t.Fatal("brute force solution should verify")
	}
	if sol.Values[0]+sol.Values[1] != 4 {
		t.Fatalf("expected sum 4, got %d+%d=%d", sol.Values[0], sol.Values[1], sol.Values[0]+sol.Values[1])
	}
}

// ============================================================
// Integration: VDF + CSP pipeline
// ============================================================

func TestVDFCSPPipeline(t *testing.T) {
	// Simulate the mining pipeline with small T.
	_, pub, _ := crypto.GenerateKeyPair()
	prevHash := crypto.Sha256([]byte("prev"))
	input := vdf.MakeInput(prevHash, pub)

	vdfParams := vdf.NewParams(testVDFT)
	output, err := vdf.Evaluate(vdfParams, input)
	if err != nil {
		t.Fatal(err)
	}

	// Generate CSP from VDF output.
	seed := crypto.Sha256(output.Y)
	prob, sol := csp.GenerateProblem(seed, csp.Standard)

	if !csp.VerifySolution(prob, sol) {
		t.Fatal("candidate solution should verify")
	}

	// Solution hash should be deterministic.
	h1 := HashSolutionValues(sol.Values)
	h2 := HashSolutionValues(sol.Values)
	if h1 != h2 {
		t.Fatal("solution hash should be deterministic")
	}
}

// ============================================================
// 14. Cross-tx double-spend within a single block is rejected
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
	blk1 := mineTestBlock(t, &genesis.Header, privA, pubA, params, 1)

	utxos := tx.NewUTXOSet()
	utxos.ApplyBlock(genesis.Transactions, 0)
	utxos.ApplyBlock(blk1.Transactions, 1)

	// Identify the coinbase UTXO from block 1.
	cb1ID := blk1.Transactions[0].TxID()
	cbOutPoint := tx.OutPoint{TxID: cb1ID, Index: 0}

	// Build two transactions that spend the same coinbase UTXO.
	buildSpendTx := func(value int64) *tx.Transaction {
		spendTx := &tx.Transaction{
			Version: 1,
			Inputs: []tx.TxInput{
				{PrevOut: cbOutPoint, Sequence: 0xFFFFFFFF},
			},
			Outputs: []tx.TxOutput{
				{Value: value, ScriptPubKey: tx.CreateP2PKHLockScript(pkhA)},
			},
		}
		subscript := blk1.Transactions[0].Outputs[0].ScriptPubKey
		sigHash := spendTx.SigHash(0, subscript)
		sig, _ := crypto.Sign(privA, sigHash)
		spendTx.Inputs[0].ScriptSig = tx.CreateP2PKHUnlockScript(
			sig.Bytes(), pubA.SerializeCompressed())
		return spendTx
	}

	spendA := buildSpendTx(5_0000_0000)
	spendB := buildSpendTx(4_0000_0000)

	// Mine block 2 with both txs included. We inject them manually
	// by mining a valid block first, then replacing transactions.
	blk2 := mineTestBlock(t, &blk1.Header, privA, pubA, params, 2)

	// Replace block 2's transactions: keep coinbase, add two double-spends.
	blk2.Transactions = append(blk2.Transactions[:1], spendA, spendB)

	// Recompute merkle root to match the tampered tx list.
	txIDs := make([]crypto.Hash, len(blk2.Transactions))
	for i, t := range blk2.Transactions {
		txIDs[i] = t.TxID()
	}
	blk2.Header.MerkleRoot = block.ComputeMerkleRoot(txIDs)

	err = ValidateBlock(blk2, &blk1.Header, params, utxos, 2)
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
	params.VDFIterations = testVDFT

	// Create a block with an output large enough to exceed MaxBlockSize.
	// We don't need it to pass other validations — just check the size gate.
	bigScript := make([]byte, block.MaxBlockSize+1)
	coinbase := tx.NewCoinbase(1, 10_0000_0000, make([]byte, 20), "test")
	coinbase.Outputs = append(coinbase.Outputs, tx.TxOutput{
		Value:        0,
		ScriptPubKey: bigScript,
	})

	blk := &block.Block{
		Header: block.Header{
			Version:       1,
			PrevBlockHash: genesis.Header.Hash(),
			Timestamp:     uint32(time.Now().Unix()),
			VDFOutput:     []byte{0x01},
			VDFProof:      []byte{0x01},
			MinerPubKey:   []byte{0x01},
		},
		Transactions: []*tx.Transaction{coinbase},
	}

	err := ValidateBlock(blk, &genesis.Header, params, chain.UTXOSet, 1)
	if err == nil {
		t.Fatal("oversized block should be rejected")
	}
}

// ============================================================
// VDF iterations mismatch rejected
// ============================================================

func TestValidateRejectsWrongVDFIterations(t *testing.T) {
	genesis := makeGenesis(make([]byte, 20))
	chain := NewChainState(genesis)
	params := testParams()

	privKey, pubKey, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}

	blk, err := MineBlock(&genesis.Header, nil, privKey, pubKey, params, 1, nil)
	if err != nil {
		t.Fatalf("mine block: %v", err)
	}

	// Tamper: set header VDFIterations to a different value.
	blk.Header.VDFIterations = params.VDFIterations + 1

	err = ValidateBlock(blk, &genesis.Header, params, chain.UTXOSet, 1)
	if err == nil {
		t.Fatal("mismatched VDFIterations should be rejected")
	}
}

// ============================================================
// Block with too many transactions rejected
// ============================================================

func TestValidateRejectsTooManyTransactions(t *testing.T) {
	genesis := makeGenesis(make([]byte, 20))
	chain := NewChainState(genesis)
	params := testParams()

	// Create a block with MaxBlockTransactions + 1 transactions.
	coinbase := tx.NewCoinbase(1, 10_0000_0000, make([]byte, 20), "test")
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
			VDFOutput:     []byte{0x01},
			VDFProof:      []byte{0x01},
			MinerPubKey:   []byte{0x01},
		},
		Transactions: txs,
	}

	err := ValidateBlock(blk, &genesis.Header, params, chain.UTXOSet, 1)
	if err == nil {
		t.Fatal("block with too many transactions should be rejected")
	}
}

// ============================================================
// Print CSP difficulty params at key heights
// ============================================================

func TestCSPParamsAtKeyHeights(t *testing.T) {
	heights := []uint64{0, 525_000, 1_050_000, 2_100_000, 3_150_000, 5_250_000, 10_500_000, 21_000_000}
	for _, h := range heights {
		p := CSPParamsForHeight(h)
		t.Logf("height %10d | base_variables = %2d | constraint_ratio = %.1f", h, p.BaseVariables, p.ConstraintRatio)
	}
}

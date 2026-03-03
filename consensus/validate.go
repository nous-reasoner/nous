package consensus

import (
	"errors"
	"fmt"
	"math"
	"time"

	"nous/block"
	"nous/crypto"
	"nous/sat"
	"nous/tx"
)

// MaxFutureSeconds is the maximum number of seconds (5 minutes) a block
// timestamp may be ahead of the node's wall-clock time.
const MaxFutureSeconds = 300

// safeAddInt64 returns a + b or an error on overflow.
func safeAddInt64(a, b int64) (int64, error) {
	if b > 0 && a > math.MaxInt64-b {
		return 0, errors.New("integer overflow")
	}
	if b < 0 && a < math.MinInt64-b {
		return 0, errors.New("integer underflow")
	}
	return a + b, nil
}

// ValidateBlockHeader performs context-free block validation: size, header
// format, SAT solution, PoW, difficulty bits, and merkle root.
func ValidateBlockHeader(
	blk *block.Block,
	prevHeader *block.Header,
	params *DifficultyParams,
) error {
	hdr := &blk.Header

	// Step 0: Block size check.
	if blk.WireSize() > block.MaxBlockSize {
		return fmt.Errorf("step 0 (block size): %d exceeds max %d", blk.WireSize(), block.MaxBlockSize)
	}

	// Step 1: Header format checks.
	if hdr.Version == 0 {
		return errors.New("step 1 (header format): version must be > 0")
	}
	expectedPrev := prevHeader.Hash()
	if hdr.PrevBlockHash != expectedPrev {
		return errors.New("step 1 (header format): prevBlockHash mismatch")
	}
	if hdr.Timestamp <= prevHeader.Timestamp {
		return fmt.Errorf("step 1 (header format): timestamp %d not after parent timestamp %d",
			hdr.Timestamp, prevHeader.Timestamp)
	}
	maxTime := uint32(time.Now().Unix()) + MaxFutureSeconds
	if hdr.Timestamp > maxTime {
		return fmt.Errorf("step 1 (header format): timestamp %d exceeds max allowed %d (now + %ds)",
			hdr.Timestamp, maxTime, MaxFutureSeconds)
	}

	// Step 2: Regenerate SAT formula and verify solution.
	prevHash := prevHeader.Hash()
	satSeed := makeSATSeed(prevHash, hdr.Seed)
	formula := sat.GenerateFormula(satSeed, SATVariables, SATClausesRatio)

	if len(blk.SATSolution) == 0 {
		return errors.New("step 2: missing SAT solution")
	}
	if !sat.Verify(formula, blk.SATSolution) {
		return errors.New("step 2: SAT solution does not satisfy formula")
	}

	// Step 3: Verify solution hash matches header.
	solBytes := sat.SerializeAssignment(blk.SATSolution)
	solHash := crypto.Sha256(solBytes)
	if solHash != hdr.SATSolutionHash {
		return errors.New("step 3: SAT solution hash mismatch")
	}

	// Step 4: Verify PoW.
	blockHash := hdr.Hash()
	if blockHash.Compare(params.PoWTarget) > 0 {
		return fmt.Errorf("step 4: PoW hash %s exceeds target", blockHash)
	}
	expectedBits := TargetToCompact(params.PoWTarget)
	if hdr.DifficultyBits != expectedBits {
		return fmt.Errorf("step 4: block difficulty bits %x != expected %x", hdr.DifficultyBits, expectedBits)
	}

	// Step 7: Verify merkle root.
	txIDs := make([]crypto.Hash, len(blk.Transactions))
	for i, t := range blk.Transactions {
		txIDs[i] = t.TxID()
	}
	expectedMerkle := block.ComputeMerkleRoot(txIDs)
	if hdr.MerkleRoot != expectedMerkle {
		return errors.New("step 7: merkle root mismatch")
	}

	return nil
}

// ValidateBlockTxs performs UTXO-dependent transaction and coinbase validation.
func ValidateBlockTxs(
	blk *block.Block,
	utxoSet tx.UTXOStore,
	height uint64,
) error {
	// Step 5: Validate all transactions.
	blockSpent := make(map[tx.OutPoint]bool)

	if len(blk.Transactions) == 0 {
		return errors.New("step 5: block has no transactions")
	}
	if len(blk.Transactions) > block.MaxBlockTransactions {
		return fmt.Errorf("step 5: tx count %d exceeds max %d",
			len(blk.Transactions), block.MaxBlockTransactions)
	}
	if !blk.Transactions[0].IsCoinbase() {
		return errors.New("step 5: first transaction must be coinbase")
	}
	// Check for duplicate transaction IDs within this block.
	txIDSeen := make(map[crypto.Hash]bool, len(blk.Transactions))
	for i, t := range blk.Transactions {
		id := t.TxID()
		if txIDSeen[id] {
			return fmt.Errorf("step 5: duplicate TxID at index %d", i)
		}
		txIDSeen[id] = true
	}
	for i := 1; i < len(blk.Transactions); i++ {
		if blk.Transactions[i].IsCoinbase() {
			return fmt.Errorf("step 5: transaction %d is coinbase (only first allowed)", i)
		}
		for _, in := range blk.Transactions[i].Inputs {
			if blockSpent[in.PrevOut] {
				return fmt.Errorf("step 5: transaction %d double-spends UTXO %s:%d (already spent in this block)",
					i, in.PrevOut.TxID, in.PrevOut.Index)
			}
		}
		if err := tx.ValidateTransaction(blk.Transactions[i], utxoSet, height); err != nil {
			return fmt.Errorf("step 5: transaction %d invalid: %w", i, err)
		}
		for _, in := range blk.Transactions[i].Inputs {
			blockSpent[in.PrevOut] = true
		}
	}

	// Step 6: Verify coinbase reward.
	expectedReward := BlockReward(height)
	var coinbaseTotal int64
	for i, out := range blk.Transactions[0].Outputs {
		if out.Amount < 0 || out.Amount > tx.MaxMoney {
			return fmt.Errorf("step 6: coinbase output %d value %d out of range", i, out.Amount)
		}
		sum, err := safeAddInt64(coinbaseTotal, out.Amount)
		if err != nil {
			return fmt.Errorf("step 6: coinbase output sum overflow")
		}
		coinbaseTotal = sum
	}
	var totalFees int64
	for i := 1; i < len(blk.Transactions); i++ {
		var inputSum, outputSum int64
		for _, in := range blk.Transactions[i].Inputs {
			u := utxoSet.Get(in.PrevOut)
			if u == nil {
				return fmt.Errorf("step 6: tx %d input references missing UTXO %s:%d",
					i, in.PrevOut.TxID, in.PrevOut.Index)
			}
			sum, err := safeAddInt64(inputSum, u.Output.Amount)
			if err != nil {
				return fmt.Errorf("step 6: fee input sum overflow in tx %d", i)
			}
			inputSum = sum
		}
		for _, out := range blk.Transactions[i].Outputs {
			sum, err := safeAddInt64(outputSum, out.Amount)
			if err != nil {
				return fmt.Errorf("step 6: fee output sum overflow in tx %d", i)
			}
			outputSum = sum
		}
		fee, err := safeAddInt64(totalFees, inputSum-outputSum)
		if err != nil {
			return fmt.Errorf("step 6: total fees overflow at tx %d", i)
		}
		totalFees = fee
	}
	maxCoinbase, err := safeAddInt64(expectedReward, totalFees)
	if err != nil {
		return fmt.Errorf("step 6: maxCoinbase overflow")
	}
	if coinbaseTotal > maxCoinbase {
		return fmt.Errorf("step 6: coinbase %d exceeds max %d (reward %d + fees %d)",
			coinbaseTotal, maxCoinbase, expectedReward, totalFees)
	}

	return nil
}

// ValidateBlock performs the full Cogito Consensus block validation by running
// both header validation and transaction validation.
func ValidateBlock(
	blk *block.Block,
	prevHeader *block.Header,
	params *DifficultyParams,
	utxoSet tx.UTXOStore,
	height uint64,
) error {
	if err := ValidateBlockHeader(blk, prevHeader, params); err != nil {
		return err
	}
	return ValidateBlockTxs(blk, utxoSet, height)
}

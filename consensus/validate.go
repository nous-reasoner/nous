package consensus

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/nous-chain/nous/block"
	"github.com/nous-chain/nous/crypto"
	"github.com/nous-chain/nous/csp"
	"github.com/nous-chain/nous/tx"
	"github.com/nous-chain/nous/vdf"
)

// MaxFutureSeconds is the maximum number of seconds a block timestamp
// may be ahead of the node's wall-clock time.
const MaxFutureSeconds = 120

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

// ValidateBlock performs the full block validation.
//
//  1. Check header format and size
//  2. Verify VDF proof
//  3. Regenerate CSP from VDF output
//  4. Check standard-tier solution
//  5. Verify solution hash matches header
//  6. Verify PoW meets difficulty
//  7. Validate all transaction signatures
//  8. Verify coinbase reward amount
//  9. Verify all UTXO references are valid
func ValidateBlock(
	blk *block.Block,
	prevHeader *block.Header,
	params *DifficultyParams,
	utxoSet *tx.UTXOSet,
	height uint64,
) error {
	hdr := &blk.Header

	// Step 0: Block size check.
	if blk.WireSize() > block.MaxBlockSize {
		return fmt.Errorf("step 0 (block size): %d exceeds max %d", blk.WireSize(), block.MaxBlockSize)
	}

	// Step 1: Header format checks.
	if err := validateHeaderFormat(hdr, prevHeader); err != nil {
		return fmt.Errorf("step 1 (header format): %w", err)
	}

	// Step 2: Verify VDF proof.
	if hdr.VDFIterations != params.VDFIterations {
		return fmt.Errorf("step 2 (VDF): header VDFIterations %d != expected %d",
			hdr.VDFIterations, params.VDFIterations)
	}
	if err := validateVDF(hdr, prevHeader, params); err != nil {
		return fmt.Errorf("step 2 (VDF): %w", err)
	}

	// Step 3: Regenerate CSP from VDF output.
	seed := crypto.Sha256(hdr.VDFOutput)
	cspParams := CSPParamsForHeight(height)
	numConstraints := ceilConstraints(cspParams.BaseVariables, cspParams.ConstraintRatio)
	stdProblem, _ := csp.GenerateProblemWithParams(seed, cspParams.BaseVariables, numConstraints)

	// Step 4: Check standard-tier solution.
	if blk.CSPSolution == nil {
		return errors.New("step 4: missing standard CSP solution")
	}
	if !csp.VerifySolution(stdProblem, blk.CSPSolution) {
		return errors.New("step 4: standard CSP solution invalid")
	}

	// Step 5: Verify solution hash matches header.
	stdHash := HashSolutionValues(blk.CSPSolution.Values)
	if stdHash != hdr.CSPSolutionHash {
		return errors.New("step 5: standard solution hash mismatch")
	}

	// Step 6: Verify PoW.
	blockHash := hdr.Hash()
	if blockHash.Compare(params.PoWTarget) > 0 {
		return fmt.Errorf("step 6: PoW hash %s exceeds target", blockHash)
	}

	// Step 7: Validate all transactions.
	// Track UTXOs spent within this block to detect cross-tx double-spends.
	blockSpent := make(map[tx.OutPoint]bool)

	if len(blk.Transactions) == 0 {
		return errors.New("step 7: block has no transactions")
	}
	if len(blk.Transactions) > block.MaxBlockTransactions {
		return fmt.Errorf("step 7: tx count %d exceeds max %d",
			len(blk.Transactions), block.MaxBlockTransactions)
	}
	if !blk.Transactions[0].IsCoinbase() {
		return errors.New("step 7: first transaction must be coinbase")
	}
	// Check for duplicate transaction IDs within this block.
	txIDSeen := make(map[crypto.Hash]bool, len(blk.Transactions))
	for i, t := range blk.Transactions {
		id := t.TxID()
		if txIDSeen[id] {
			return fmt.Errorf("step 7: duplicate TxID at index %d", i)
		}
		txIDSeen[id] = true
	}
	for i := 1; i < len(blk.Transactions); i++ {
		if blk.Transactions[i].IsCoinbase() {
			return fmt.Errorf("step 7: transaction %d is coinbase (only first allowed)", i)
		}
		// Check for cross-tx double-spend within this block.
		for _, in := range blk.Transactions[i].Inputs {
			if blockSpent[in.PrevOut] {
				return fmt.Errorf("step 7: transaction %d double-spends UTXO %s:%d (already spent in this block)",
					i, in.PrevOut.TxID, in.PrevOut.Index)
			}
		}
		if err := tx.ValidateTransaction(blk.Transactions[i], utxoSet, height); err != nil {
			return fmt.Errorf("step 7: transaction %d invalid: %w", i, err)
		}
		// Mark all inputs as spent within this block.
		for _, in := range blk.Transactions[i].Inputs {
			blockSpent[in.PrevOut] = true
		}
	}

	// Step 8: Verify coinbase reward.
	expectedReward := BlockReward(height)
	var coinbaseTotal int64
	for i, out := range blk.Transactions[0].Outputs {
		if out.Value < 0 || out.Value > tx.MaxMoney {
			return fmt.Errorf("step 8: coinbase output %d value %d out of range", i, out.Value)
		}
		sum, err := safeAddInt64(coinbaseTotal, out.Value)
		if err != nil {
			return fmt.Errorf("step 8: coinbase output sum overflow")
		}
		coinbaseTotal = sum
	}
	// Coinbase may also collect fees; for now just check it doesn't exceed reward + fees.
	var totalFees int64
	for i := 1; i < len(blk.Transactions); i++ {
		var inputSum, outputSum int64
		for _, in := range blk.Transactions[i].Inputs {
			u := utxoSet.Get(in.PrevOut)
			if u != nil {
				sum, err := safeAddInt64(inputSum, u.Output.Value)
				if err != nil {
					return fmt.Errorf("step 8: fee input sum overflow in tx %d", i)
				}
				inputSum = sum
			}
		}
		for _, out := range blk.Transactions[i].Outputs {
			sum, err := safeAddInt64(outputSum, out.Value)
			if err != nil {
				return fmt.Errorf("step 8: fee output sum overflow in tx %d", i)
			}
			outputSum = sum
		}
		fee, err := safeAddInt64(totalFees, inputSum-outputSum)
		if err != nil {
			return fmt.Errorf("step 8: total fees overflow at tx %d", i)
		}
		totalFees = fee
	}
	maxCoinbase, err := safeAddInt64(expectedReward, totalFees)
	if err != nil {
		return fmt.Errorf("step 8: maxCoinbase overflow")
	}
	if coinbaseTotal > maxCoinbase {
		return fmt.Errorf("step 8: coinbase %d exceeds max %d (reward %d + fees %d)",
			coinbaseTotal, maxCoinbase, expectedReward, totalFees)
	}

	// Step 9: Verify merkle root.
	txIDs := make([]crypto.Hash, len(blk.Transactions))
	for i, t := range blk.Transactions {
		txIDs[i] = t.TxID()
	}
	expectedMerkle := block.ComputeMerkleRoot(txIDs)
	if hdr.MerkleRoot != expectedMerkle {
		return errors.New("step 10: merkle root mismatch")
	}

	return nil
}

func validateHeaderFormat(hdr *block.Header, prevHeader *block.Header) error {
	if hdr.Version == 0 {
		return errors.New("version must be > 0")
	}
	expectedPrev := prevHeader.Hash()
	if hdr.PrevBlockHash != expectedPrev {
		return errors.New("prevBlockHash mismatch")
	}

	// Timestamp must strictly advance beyond the parent block.
	if hdr.Timestamp <= prevHeader.Timestamp {
		return fmt.Errorf("timestamp %d not after parent timestamp %d",
			hdr.Timestamp, prevHeader.Timestamp)
	}
	// Timestamp must not be too far in the future.
	maxTime := uint32(time.Now().Unix()) + MaxFutureSeconds
	if hdr.Timestamp > maxTime {
		return fmt.Errorf("timestamp %d exceeds max allowed %d (now + %ds)",
			hdr.Timestamp, maxTime, MaxFutureSeconds)
	}

	if len(hdr.MinerPubKey) == 0 {
		return errors.New("missing miner public key")
	}
	if len(hdr.VDFOutput) == 0 {
		return errors.New("missing VDF output")
	}
	if len(hdr.VDFProof) == 0 {
		return errors.New("missing VDF proof")
	}
	return nil
}

func validateVDF(hdr *block.Header, prevHeader *block.Header, params *DifficultyParams) error {
	minerPub, err := crypto.ParsePublicKey(hdr.MinerPubKey)
	if err != nil {
		return fmt.Errorf("invalid miner pubkey: %w", err)
	}

	prevHash := prevHeader.Hash()
	vdfInput := vdf.MakeInput(prevHash, minerPub)
	vdfParams := vdf.NewParams(params.VDFIterations)
	vdfOut := &vdf.Output{
		Y:     hdr.VDFOutput,
		Proof: hdr.VDFProof,
	}
	if !vdf.Verify(vdfParams, vdfInput, vdfOut) {
		return errors.New("VDF proof verification failed")
	}
	return nil
}

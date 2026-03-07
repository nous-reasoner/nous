package consensus

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"time"

	"nous/block"
	"nous/crypto"
	"nous/sat"
	"nous/tx"
)

// SATSolveTimeout is the timeout for each SAT solve attempt during mining.
const SATSolveTimeout = 100 * time.Millisecond

// DefaultMineTimeout is the maximum wall-clock time MineBlock will spend
// searching for a valid block before returning an error.
const DefaultMineTimeout = 10 * time.Minute

// MineBlock performs the complete Cogito Consensus mining flow and returns a valid block.
//
// Steps:
//  1. Create coinbase (block reward + transaction fees)
//  2. Compute Merkle root
//  3. Iterate seed from 0:
//     a. Generate SAT formula from makeSATSeed(prevHash, seed)
//     b. Solve with ProbSAT
//     c. Check if block hash < target
//  4. Return complete block
func MineBlock(
	prevHeader *block.Header,
	txs []*tx.Transaction,
	pubKeyHash []byte,
	params *DifficultyParams,
	height uint64,
	utxoSet tx.UTXOStore,
	isTestnet bool,
) (*block.Block, error) {
	// Step 0: Create coinbase (reward + fees).
	reward := BlockReward(height)
	var totalFees int64
	if utxoSet != nil {
		for _, t := range txs {
			var inputSum, outputSum int64
			for _, in := range t.Inputs {
				u := utxoSet.Get(in.PrevOut)
				if u != nil {
					s, err := safeAddInt64(inputSum, u.Output.Amount)
					if err != nil {
						return nil, errors.New("consensus: input sum overflow in fee calculation")
					}
					inputSum = s
				}
			}
			for _, out := range t.Outputs {
				s, err := safeAddInt64(outputSum, out.Amount)
				if err != nil {
					return nil, errors.New("consensus: output sum overflow in fee calculation")
				}
				outputSum = s
			}
			if inputSum > outputSum {
				s, err := safeAddInt64(totalFees, inputSum-outputSum)
				if err != nil {
					return nil, errors.New("consensus: total fees overflow")
				}
				totalFees = s
			}
		}
	}
	coinbaseAmount, err := safeAddInt64(reward, totalFees)
	if err != nil {
		return nil, errors.New("consensus: reward + fees overflow")
	}
	coinbase := tx.NewCoinbaseTx(height, coinbaseAmount, tx.CreateP2PKHLockScript(pubKeyHash), tx.ChainIDFor(isTestnet))
	allTxs := make([]*tx.Transaction, 0, 1+len(txs))
	allTxs = append(allTxs, coinbase)
	allTxs = append(allTxs, txs...)

	// Merkle root.
	txIDs := make([]crypto.Hash, len(allTxs))
	for i, t := range allTxs {
		txIDs[i] = t.TxID()
	}
	merkleRoot := block.ComputeMerkleRoot(txIDs)

	prevHash := prevHeader.Hash()

	// Timestamp.
	now := uint32(time.Now().Unix())
	if now <= prevHeader.Timestamp {
		now = prevHeader.Timestamp + 1
	}
	// If we've drifted too far into the future (fast mining), wait until the
	// timestamp is within the MaxFutureSeconds validation window.
	maxAllowed := uint32(time.Now().Unix()) + MaxFutureSeconds - 30
	if now > maxAllowed {
		wait := time.Duration(now-maxAllowed) * time.Second
		log.Printf("mine: timestamp %d exceeds safe window, sleeping %v", now, wait)
		time.Sleep(wait)
		// Recompute now in case wall-clock caught up.
		now = uint32(time.Now().Unix())
		if now <= prevHeader.Timestamp {
			now = prevHeader.Timestamp + 1
		}
	}

	// UTXO set hash (not yet implemented; zero for now).
	var utxoSetHash crypto.Hash

	// Mining loop: iterate seed values.
	startTime := time.Now()
	deadline := startTime.Add(DefaultMineTimeout)
	for seed := uint64(0); ; seed++ {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("mining timeout after %v", DefaultMineTimeout)
		}
		satSeed := MakeSATSeed(prevHash, seed)
		formula := sat.GenerateFormula(satSeed, SATVariables, SATClausesRatio)

		solution, err := sat.ProbSATSolve(formula, SATVariables, SATSolveTimeout)
		if err != nil {
			// SAT solve failed (timeout), try next seed.
			continue
		}

		// Compute solution hash.
		solBytes := sat.SerializeAssignment(solution)
		solHash := crypto.Sha256(solBytes)

		// Build header.
		hdr := block.Header{
			Version:         1,
			PrevBlockHash:   prevHash,
			MerkleRoot:      merkleRoot,
			Timestamp:       now,
			DifficultyBits:  TargetToCompact(params.PoWTarget),
			Seed:            seed,
			SATSolutionHash: solHash,
			UTXOSetHash:     utxoSetHash,
		}

		// Check PoW: hash < target.
		blockHash := hdr.Hash()
		if blockHash.Compare(params.PoWTarget) <= 0 {
			log.Printf("mine: block found at seed=%d, attempts=%d, elapsed=%v", seed, seed+1, time.Since(startTime))
			blk := &block.Block{
				Header:       hdr,
				Transactions: allTxs,
				SATSolution:  solution,
			}
			return blk, nil
		}

		// PoW not met, try next seed.
	}
}

// MakeSATSeed derives a deterministic 32-byte seed for SAT formula generation.
// seed = SHA256(prevHash || seed_le_bytes)
func MakeSATSeed(prevHash crypto.Hash, seed uint64) [32]byte {
	var buf [40]byte
	copy(buf[:32], prevHash[:])
	binary.LittleEndian.PutUint64(buf[32:], seed)
	return crypto.Sha256(buf[:])
}

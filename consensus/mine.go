package consensus

import (
	"encoding/binary"
	"errors"
	"time"

	"github.com/nous-chain/nous/block"
	"github.com/nous-chain/nous/crypto"
	"github.com/nous-chain/nous/sat"
	"github.com/nous-chain/nous/tx"
)

// SATSolveTimeout is the timeout for each SAT solve attempt during mining.
const SATSolveTimeout = 10 * time.Second

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
	utxoSet *tx.UTXOSet,
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
					s, err := safeAddInt64(inputSum, u.Output.Value)
					if err != nil {
						return nil, errors.New("consensus: input sum overflow in fee calculation")
					}
					inputSum = s
				}
			}
			for _, out := range t.Outputs {
				s, err := safeAddInt64(outputSum, out.Value)
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
	coinbase := tx.NewCoinbase(uint32(height), reward+totalFees, pubKeyHash, "")
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

	// UTXO set hash (not yet implemented; zero for now).
	var utxoSetHash crypto.Hash

	// Mining loop: iterate seed values.
	for seed := uint64(0); ; seed++ {
		satSeed := makeSATSeed(prevHash, seed)
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

// makeSATSeed derives a deterministic 32-byte seed for SAT formula generation.
// seed = SHA256(prevHash || seed_le_bytes)
func makeSATSeed(prevHash crypto.Hash, seed uint64) [32]byte {
	var buf [40]byte
	copy(buf[:32], prevHash[:])
	binary.LittleEndian.PutUint64(buf[32:], seed)
	return crypto.Sha256(buf[:])
}

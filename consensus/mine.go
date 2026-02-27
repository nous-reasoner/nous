package consensus

import (
	"errors"
	"math"
	"time"

	"github.com/nous-chain/nous/block"
	"github.com/nous-chain/nous/crypto"
	"github.com/nous-chain/nous/csp"
	"github.com/nous-chain/nous/tx"
	"github.com/nous-chain/nous/vdf"
)

// MaxNonce is the upper bound for the PoW nonce search.
const MaxNonce = uint32(math.MaxUint32)

// CSPSolver is the interface for solving CSP problems during mining.
// When nil is passed to MineBlock, the built-in BruteForceSolve is used.
type CSPSolver interface {
	Solve(problem *csp.Problem, timeout time.Duration) (*csp.Solution, error)
}

// DefaultSolveTimeout is the default timeout for CSP solving during mining.
const DefaultSolveTimeout = 30 * time.Second

// MineBlock performs the complete mining flow and returns a valid block.
//
// Steps:
//  1. Compute VDF input = SHA256(prevBlock.Hash || minerPubKey)
//  2. Run VDF_Evaluate(input, T)
//  3. Generate standard-tier CSP from VDF output
//  4. Solve standard-tier CSP via solver (or brute-force fallback)
//  5. Build block header
//  6. PoW nonce search
//  7. Return complete block
func MineBlock(
	prevHeader *block.Header,
	txs []*tx.Transaction,
	minerPriv *crypto.PrivateKey,
	minerPub *crypto.PublicKey,
	params *DifficultyParams,
	height uint64,
	solver CSPSolver,
	utxoSet ...*tx.UTXOSet,
) (*block.Block, error) {
	pubKeyHash := crypto.Hash160(minerPub.SerializeCompressed())

	// Step 0: Create coinbase (reward + fees).
	reward := BlockReward(height)
	var totalFees int64
	if len(utxoSet) > 0 && utxoSet[0] != nil {
		for _, t := range txs {
			var inputSum, outputSum int64
			for _, in := range t.Inputs {
				u := utxoSet[0].Get(in.PrevOut)
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

	// Step 1: VDF input.
	prevHash := prevHeader.Hash()
	vdfInput := vdf.MakeInput(prevHash, minerPub)

	// Step 2: VDF evaluate.
	vdfParams := vdf.NewParams(params.VDFIterations)
	vdfOutput, err := vdf.Evaluate(vdfParams, vdfInput)
	if err != nil {
		return nil, err
	}

	// Step 3: Generate standard CSP from VDF output seed.
	seed := crypto.Sha256(vdfOutput.Y)
	stdProblem, stdCandidate := csp.GenerateProblem(seed, csp.Standard)

	// Step 4: Solve standard CSP.
	stdSolution, err := solveCSP(solver, stdProblem, stdCandidate)
	if err != nil {
		return nil, errors.New("consensus: failed to solve standard CSP")
	}
	stdSolHash := HashSolutionValues(stdSolution.Values)

	// Step 5: Build header.
	now := uint32(time.Now().Unix())
	// Ensure timestamp is strictly after the parent (required by validation).
	if now <= prevHeader.Timestamp {
		now = prevHeader.Timestamp + 1
	}
	hdr := block.Header{
		Version:         1,
		PrevBlockHash:   prevHash,
		MerkleRoot:      merkleRoot,
		Timestamp:       now,
		DifficultyBits:  TargetToCompact(params.PoWTarget),
		VDFOutput:       vdfOutput.Y,
		VDFProof:        vdfOutput.Proof,
		VDFIterations:   params.VDFIterations,
		CSPSolutionHash: stdSolHash,
		MinerPubKey:     minerPub.SerializeCompressed(),
		Nonce:           0,
	}

	// Step 6: PoW nonce search.
	found := false
	for nonce := uint32(0); nonce <= MaxNonce; nonce++ {
		hdr.Nonce = nonce
		hash := hdr.Hash()
		if hash.Compare(params.PoWTarget) <= 0 {
			found = true
			break
		}
	}
	if !found {
		return nil, errors.New("consensus: nonce exhausted without finding valid PoW")
	}

	// Step 7: Assemble block.
	blk := &block.Block{
		Header:       hdr,
		Transactions: allTxs,
		CSPSolution:  stdSolution,
	}
	return blk, nil
}

// solveCSP tries the candidate solution first, then the solver, then brute-force.
func solveCSP(solver CSPSolver, problem *csp.Problem, candidate *csp.Solution) (*csp.Solution, error) {
	// Fast path: candidate from generation is already valid.
	if csp.VerifySolution(problem, candidate) {
		return candidate, nil
	}

	// Try external solver if provided.
	if solver != nil {
		sol, err := solver.Solve(problem, DefaultSolveTimeout)
		if err == nil && csp.VerifySolution(problem, sol) {
			return sol, nil
		}
	}

	// Fallback: brute-force.
	sol := BruteForceSolve(problem)
	if sol != nil {
		return sol, nil
	}
	return nil, errors.New("consensus: no solver could find a solution")
}

// BruteForceSolve attempts to solve a CSP by exhaustive search.
// This is a placeholder for the real AI solver. Only viable for small problems.
func BruteForceSolve(problem *csp.Problem) *csp.Solution {
	n := len(problem.Variables)
	if n == 0 {
		return nil
	}

	sol := &csp.Solution{Values: make([]int, n)}
	if bruteForceHelper(problem, sol, 0) {
		return sol
	}
	return nil
}

func bruteForceHelper(problem *csp.Problem, sol *csp.Solution, idx int) bool {
	if idx == len(problem.Variables) {
		return csp.VerifySolution(problem, sol)
	}
	v := problem.Variables[idx]
	for val := v.Lower; val <= v.Upper; val++ {
		sol.Values[idx] = val
		if bruteForceHelper(problem, sol, idx+1) {
			return true
		}
	}
	return false
}

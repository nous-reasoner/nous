package consensus

import (
	"math"
	"math/big"

	"github.com/nous-chain/nous/crypto"
)

// AdjustmentWindow is the number of recent blocks used for adjustment.
const AdjustmentWindow = 144

// MinVDFIterations is the absolute minimum VDF iteration count.
// 1024 squarings ≈ milliseconds — prevents VDF from becoming trivially fast.
const MinVDFIterations uint64 = 1 << 10

// BlockInfo carries the minimal per-block data needed for difficulty adjustment.
type BlockInfo struct {
	Timestamp uint32
}

// AdjustDifficulty computes new difficulty parameters based on recent block history.
// chain must contain the most recent AdjustmentWindow blocks (oldest first).
// currentHeight is the height at which the new params take effect.
func AdjustDifficulty(current *DifficultyParams, chain []BlockInfo, currentHeight uint64) *DifficultyParams {
	n := len(chain)
	if n < 2 {
		return copyParams(current)
	}

	// Actual time span.
	actualSpan := int64(chain[n-1].Timestamp) - int64(chain[0].Timestamp)
	if actualSpan <= 0 {
		actualSpan = 1
	}
	expectedSpan := int64(n-1) * int64(TargetBlockTime)
	ratio := float64(actualSpan) / float64(expectedSpan)

	next := copyParams(current)

	// --- Layer 1: VDF iterations ---
	next.VDFIterations = adjustVDF(current.VDFIterations, ratio)

	// --- Layer 2: CSP difficulty (deterministic, height-based) ---
	next.CSPDifficulty = CSPParamsForHeight(currentHeight)

	// --- Layer 3: PoW target ---
	next.PoWTarget = adjustPoWTarget(current.PoWTarget, ratio)

	return next
}

func adjustVDF(current uint64, ratio float64) uint64 {
	// Inverse ratio: if blocks are too fast (ratio < 1), increase iterations.
	adjustment := 1.0 / ratio
	adjustment = clampAdjustment(adjustment, ratio)

	newT := float64(current) * adjustment
	result := uint64(math.Round(newT))
	if result < MinVDFIterations {
		result = MinVDFIterations
	}
	return result
}

// CSPParamsForHeight returns the CSP parameters for a given block height.
// Starts at genesis defaults (12 vars, 1.4 ratio) and applies any scheduled
// upgrades from CSPUpgrades whose ActivationHeight has been reached.
func CSPParamsForHeight(height uint64) CSPDifficultyParams {
	vars := 12
	ratio := 1.4
	for _, u := range CSPUpgrades {
		if height >= u.ActivationHeight {
			vars = u.BaseVariables
			ratio = u.ConstraintRatio
		}
	}
	return CSPDifficultyParams{
		BaseVariables:   vars,
		ConstraintRatio: ratio,
	}
}

func adjustPoWTarget(current crypto.Hash, ratio float64) crypto.Hash {
	// ratio > 1 means blocks are too slow → make target easier (larger).
	// ratio < 1 means blocks are too fast → make target harder (smaller).
	adjustment := ratio
	adjustment = clampAdjustment(adjustment, ratio)

	t := new(big.Int).SetBytes(current[:])
	// Multiply by adjustment using fixed-point: (t * adjNum) / adjDen.
	adjNum := int64(math.Round(adjustment * 10000))
	if adjNum <= 0 {
		adjNum = 1
	}
	t.Mul(t, big.NewInt(adjNum))
	t.Div(t, big.NewInt(10000))

	// Ensure target doesn't go to zero.
	if t.Sign() <= 0 {
		t.SetInt64(1)
	}

	// Cap at maximum target.
	maxTarget := new(big.Int).Lsh(big.NewInt(1), 256)
	maxTarget.Sub(maxTarget, big.NewInt(1))
	if t.Cmp(maxTarget) > 0 {
		t = maxTarget
	}

	var h crypto.Hash
	b := t.Bytes()
	if len(b) <= 32 {
		copy(h[32-len(b):], b)
	}
	return h
}

// clampAdjustment applies the ±25% normal cap and -50% extreme emergency cap.
// ratio is actualSpan/expectedSpan.
// adjustment is the raw multiplier to apply to the difficulty parameter.
func clampAdjustment(adjustment float64, ratio float64) float64 {
	// Extreme case: blocks are 10x slower than target → allow single -50% reduction.
	if ratio >= 10.0 {
		if adjustment > 2.0 {
			adjustment = 2.0 // cap at doubling the target (50% easier)
		}
		return adjustment
	}

	// Normal cap: ±25%.
	if adjustment > 1.25 {
		adjustment = 1.25
	}
	if adjustment < 0.75 {
		adjustment = 0.75
	}
	return adjustment
}

func copyParams(p *DifficultyParams) *DifficultyParams {
	cp := *p
	return &cp
}

package consensus

import (
	"math/big"

	"github.com/nous-chain/nous/crypto"
)

// ASERTHalflife is the time in seconds for the target to halve or double
// when blocks deviate from the ideal schedule. 12 hours.
const ASERTHalflife int64 = 43200

// IdealBlockTime is the target interval between blocks in seconds.
const IdealBlockTime int64 = 150

// ASERTAnchor is the fixed reference point for the ASERT calculation.
// Typically set from the genesis block.
type ASERTAnchor struct {
	Height    uint64
	Timestamp uint32
	Target    crypto.Hash
}

// AdjustDifficultyASERT computes the PoW target for a block at the given
// height and timestamp using the ASERT algorithm.
//
//	target = anchor_target × 2^((timestamp - expected_timestamp) / halflife)
//	expected_timestamp = anchor.Timestamp + IdealBlockTime × (height - anchor.Height)
//
// The exponent is decomposed into an integer quotient (applied via bit shift)
// and a fractional remainder (applied via a 3rd-order Taylor expansion of 2^x).
// All arithmetic is pure big.Int — no floating point.
func AdjustDifficultyASERT(anchor *ASERTAnchor, height uint64, timestamp uint32) crypto.Hash {
	expectedTimestamp := int64(anchor.Timestamp) + IdealBlockTime*int64(height-anchor.Height)
	timeDiff := int64(timestamp) - expectedTimestamp

	// Floor-divide timeDiff by halflife so that 0 <= remainder < halflife.
	quotient := timeDiff / ASERTHalflife
	remainder := timeDiff % ASERTHalflife
	if remainder < 0 {
		quotient--
		remainder += ASERTHalflife
	}

	anchorTarget := new(big.Int).SetBytes(anchor.Target[:])

	// Apply 2^quotient via bit shifting.
	if quotient >= 0 {
		anchorTarget = shiftTargetLeft(anchorTarget, uint(quotient))
	} else {
		anchorTarget = shiftTargetRight(anchorTarget, uint(-quotient))
	}

	// Apply 2^(remainder/halflife) via rational approximation.
	anchorTarget = adjustTargetFraction(anchorTarget, remainder, ASERTHalflife)

	// Clamp to [1, 2^256 - 1].
	maxTarget := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
	if anchorTarget.Cmp(maxTarget) > 0 {
		anchorTarget = maxTarget
	}
	if anchorTarget.Sign() <= 0 {
		anchorTarget.SetInt64(1)
	}

	var result crypto.Hash
	b := anchorTarget.Bytes()
	if len(b) <= 32 {
		copy(result[32-len(b):], b)
	}
	return result
}

// shiftTargetLeft multiplies target by 2^n (makes mining easier).
func shiftTargetLeft(target *big.Int, n uint) *big.Int {
	return new(big.Int).Lsh(target, n)
}

// shiftTargetRight divides target by 2^n (makes mining harder).
// The result is clamped to a minimum of 1.
func shiftTargetRight(target *big.Int, n uint) *big.Int {
	result := new(big.Int).Rsh(target, n)
	if result.Sign() <= 0 {
		result.SetInt64(1)
	}
	return result
}

// adjustTargetFraction computes target × 2^(remainder/halflife) using a
// 3rd-order Taylor expansion of 2^x = e^(x·ln2):
//
//	2^x ≈ 1 + u + u²/2 + u³/6    where u = x·ln(2)
//
// All arithmetic uses big.Int with a 10^18 precision factor.
func adjustTargetFraction(target *big.Int, remainder, halflife int64) *big.Int {
	if remainder == 0 {
		return target
	}

	// Precision factor: 10^18.
	precision := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)

	// ln(2) scaled by 10^18 ≈ 693147180559945309.
	ln2 := big.NewInt(693147180559945309)

	// x = remainder / halflife, scaled by precision.
	xScaled := new(big.Int).Mul(big.NewInt(remainder), precision)
	xScaled.Div(xScaled, big.NewInt(halflife))

	// u = x · ln(2), scaled by precision.
	u := new(big.Int).Mul(xScaled, ln2)
	u.Div(u, precision)

	// term1 = u                              (scaled by precision)
	term1 := new(big.Int).Set(u)

	// term2 = u² / (2 · precision)           (scaled by precision)
	term2 := new(big.Int).Mul(u, u)
	term2.Div(term2, new(big.Int).Mul(big.NewInt(2), precision))

	// term3 = u³ / (6 · precision²)          (scaled by precision)
	term3 := new(big.Int).Mul(u, u)
	term3.Mul(term3, u)
	precSq := new(big.Int).Mul(precision, precision)
	term3.Div(term3, new(big.Int).Mul(big.NewInt(6), precSq))

	// factor = precision + term1 + term2 + term3 = (1 + u + u²/2 + u³/6) · precision
	factor := new(big.Int).Add(precision, term1)
	factor.Add(factor, term2)
	factor.Add(factor, term3)

	// result = target × factor / precision
	result := new(big.Int).Mul(target, factor)
	result.Div(result, precision)

	return result
}

package consensus

import (
	"encoding/binary"
	"math/big"

	"github.com/nous-chain/nous/crypto"
)

// NousPerCoin is the number of base units (nou) per 1 NOUS coin.
const NousPerCoin int64 = 100_000_000

// Subsidy constants.
const (
	InitialRewardNou int64 = 10_0000_0000          // 10 NOUS in base units
	MaxTotalSupply   int64 = 21_000_000_000_00000000 // 210 亿 NOUS in nou
)

// Target block time in seconds (30 seconds).
const TargetBlockTime uint64 = 30

// DifficultyAdjustInterval is how many blocks between difficulty recalculations.
const DifficultyAdjustInterval = 144

// CSPDifficultyParams holds the CSP complexity parameters.
type CSPDifficultyParams struct {
	BaseVariables   int
	ConstraintRatio float64
}

// CSPUpgrade defines a CSP difficulty change activated at a specific block height.
// Upgrades must be ordered by ActivationHeight (ascending).
type CSPUpgrade struct {
	ActivationHeight uint64
	BaseVariables    int
	ConstraintRatio  float64
}

// CSPUpgrades is the ordered list of future CSP difficulty upgrades.
// Empty at launch — CSP difficulty stays at the genesis defaults (8 vars, 1.2 ratio)
// until a network upgrade is scheduled here.
var CSPUpgrades = []CSPUpgrade{}

// DifficultyParams is the three-layer difficulty parameter set.
type DifficultyParams struct {
	VDFIterations uint64
	CSPDifficulty CSPDifficultyParams
	PoWTarget     crypto.Hash
}

// TestnetDifficultyParams returns relaxed difficulty for testnet (fast blocks).
func TestnetDifficultyParams() *DifficultyParams {
	return &DifficultyParams{
		VDFIterations: 1 << 10, // ~1024 squarings, very fast
		CSPDifficulty: CSPDifficultyParams{
			BaseVariables:   4,
			ConstraintRatio: 1.0,
		},
		PoWTarget: CompactToTarget(0x2000ffff), // ~256 nonce attempts
	}
}

// DefaultDifficultyParams returns the initial difficulty for the genesis epoch.
func DefaultDifficultyParams() *DifficultyParams {
	return &DifficultyParams{
		VDFIterations: 1 << 20, // ~10 seconds on typical hardware
		CSPDifficulty: CSPDifficultyParams{
			BaseVariables:   12,
			ConstraintRatio: 1.4,
		},
		PoWTarget: CompactToTarget(0x1d00ffff),
	}
}

// BlockReward returns the mining reward (in nou) for a given block height.
// Constant emission: every block pays InitialRewardNou until MaxTotalSupply is reached.
// maxRewardHeight = MaxTotalSupply / InitialRewardNou = 2,100,000,000 blocks.
func BlockReward(height uint64) int64 {
	const maxRewardHeight uint64 = 2_100_000_000
	if height >= maxRewardHeight {
		return 0
	}
	return InitialRewardNou
}

// CompactToTarget converts a Bitcoin-style compact target representation
// to a full 256-bit hash target.
//
// Format: 0xEEMMMMMMM where EE = exponent, MMMMMM = mantissa.
// target = mantissa * 256^(exponent-3)
func CompactToTarget(bits uint32) crypto.Hash {
	exponent := bits >> 24
	mantissa := int64(bits & 0x007FFFFF)
	if bits&0x00800000 != 0 {
		mantissa = -mantissa
	}

	target := big.NewInt(mantissa)
	if exponent <= 3 {
		target.Rsh(target, uint(8*(3-exponent)))
	} else {
		target.Lsh(target, uint(8*(exponent-3)))
	}

	var h crypto.Hash
	b := target.Bytes()
	if len(b) <= 32 {
		copy(h[32-len(b):], b)
	}
	return h
}

// TargetToCompact converts a 256-bit target hash back to compact form.
func TargetToCompact(target crypto.Hash) uint32 {
	// Find first non-zero byte.
	i := 0
	for i < 32 && target[i] == 0 {
		i++
	}
	if i == 32 {
		return 0
	}

	// Exponent is the number of significant bytes.
	significantBytes := 32 - i
	exponent := uint32(significantBytes)

	// Extract 3-byte mantissa (big-endian).
	var mantissa uint32
	if significantBytes >= 3 {
		mantissa = uint32(target[i])<<16 | uint32(target[i+1])<<8 | uint32(target[i+2])
	} else if significantBytes == 2 {
		mantissa = uint32(target[i])<<16 | uint32(target[i+1])<<8
	} else {
		mantissa = uint32(target[i]) << 16
	}

	// If the high bit of mantissa is set, shift right to avoid sign confusion.
	if mantissa&0x00800000 != 0 {
		mantissa >>= 8
		exponent++
	}

	return (exponent << 24) | mantissa
}

// HashSolutionValues computes a deterministic hash of a CSP solution's values
// for embedding in the block header.
// Returns zero hash for nil or empty input.
func HashSolutionValues(values []int) crypto.Hash {
	if len(values) == 0 {
		return crypto.Hash{}
	}
	buf := make([]byte, len(values)*4)
	for i, v := range values {
		binary.LittleEndian.PutUint32(buf[i*4:], uint32(int32(v)))
	}
	return crypto.DoubleSha256(buf)
}

package consensus

import (
	"math/big"

	"nous/crypto"
)

// NousPerCoin is the number of base units (nou) per 1 NOUS coin.
const NousPerCoin int64 = 100_000_000

// InitialRewardNou is the block reward: 1 NOUS per block (infinite supply).
const InitialRewardNou int64 = 1_00000000

// Target block time in seconds (150 seconds / 2.5 minutes).
const TargetBlockTime uint64 = 150

// SAT parameters for Cogito Consensus.
const (
	SATVariables   = 256
	SATClausesRatio = 3.85
	SATClauses     = 986
)

// DifficultyParams holds the PoW difficulty target.
type DifficultyParams struct {
	PoWTarget crypto.Hash
}

// TestnetDifficultyParams returns relaxed difficulty for testnet (fast blocks).
func TestnetDifficultyParams() *DifficultyParams {
	return &DifficultyParams{
		PoWTarget: CompactToTarget(0x2000ffff),
	}
}

// DefaultDifficultyParams returns the initial difficulty for the genesis epoch.
func DefaultDifficultyParams() *DifficultyParams {
	return &DifficultyParams{
		PoWTarget: CompactToTarget(0x1d00ffff),
	}
}

// BlockReward returns the mining reward (in nou) for a given block height.
// Cogito Consensus: constant 1 NOUS per block, infinite supply.
func BlockReward(height uint64) int64 {
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

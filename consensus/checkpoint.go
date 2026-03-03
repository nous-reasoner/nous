package consensus

import (
	"encoding/hex"

	"nous/crypto"
)

// Checkpoint pins a block hash at a specific height.
// Blocks at checkpoint heights must have the exact hash listed here.
type Checkpoint struct {
	Height uint64
	Hash   crypto.Hash
}

// hexHash parses a hex-encoded block hash into a crypto.Hash.
// Panics on invalid input (only used for compile-time constants).
func hexHash(s string) crypto.Hash {
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 32 {
		panic("invalid checkpoint hash: " + s)
	}
	var h crypto.Hash
	copy(h[:], b)
	return h
}

// TestnetCheckpoints are the hardcoded checkpoints for the testnet.
var TestnetCheckpoints = []Checkpoint{
	{Height: 0, Hash: hexHash("046efc6998a2cdd1780a32d421ffc5651d2bc9f1a915599aad20f392302e0fbc")},
	{Height: 500, Hash: hexHash("004050bbec2b03da56e53050a88088742254a3e709d7f8f5800e2c40478f888f")},
	{Height: 1000, Hash: hexHash("000e9f07ae847a117d491eec298a0dacf9c39f10846a0a670c49426c168008a4")},
	{Height: 1500, Hash: hexHash("0003b9259af669cf20d083749e3006f020da96b1a8115dfd6a7ac70c368da171")},
	{Height: 2000, Hash: hexHash("0002e0c959523238f3aba77fd149e08cb752b3ea399a11c521f1a5b5c31cc73b")},
}

// MainnetCheckpoints are the hardcoded checkpoints for mainnet.
// Will be populated before mainnet launch.
var MainnetCheckpoints = []Checkpoint{}

// checkpointMap builds a height→hash lookup from a checkpoint list.
func checkpointMap(cps []Checkpoint) map[uint64]crypto.Hash {
	m := make(map[uint64]crypto.Hash, len(cps))
	for _, cp := range cps {
		m[cp.Height] = cp.Hash
	}
	return m
}

// ValidateCheckpoint checks whether a block at the given height has the
// correct hash according to the checkpoint list.
// Returns true if:
//   - The height is not a checkpoint height (no constraint), or
//   - The height is a checkpoint height and the hash matches.
//
// Returns false if the height is a checkpoint height and the hash does NOT match.
func ValidateCheckpoint(height uint64, hash crypto.Hash, isTestnet bool) bool {
	cps := MainnetCheckpoints
	if isTestnet {
		cps = TestnetCheckpoints
	}
	for _, cp := range cps {
		if cp.Height == height {
			return cp.Hash == hash
		}
	}
	return true // not a checkpoint height
}

// LatestCheckpointHeight returns the highest checkpoint height for the network.
// Returns 0 if no checkpoints exist.
func LatestCheckpointHeight(isTestnet bool) uint64 {
	cps := MainnetCheckpoints
	if isTestnet {
		cps = TestnetCheckpoints
	}
	var max uint64
	for _, cp := range cps {
		if cp.Height > max {
			max = cp.Height
		}
	}
	return max
}

// IsCheckpointStale returns true if currentHeight is below the latest
// checkpoint, meaning the node may be on a stale or fake chain.
func IsCheckpointStale(currentHeight uint64, isTestnet bool) bool {
	latest := LatestCheckpointHeight(isTestnet)
	return latest > 0 && currentHeight < latest
}

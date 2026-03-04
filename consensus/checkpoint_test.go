package consensus

import (
	"nous/crypto"
	"testing"
)

func TestValidateCheckpoint_NonCheckpointHeight(t *testing.T) {
	// With no testnet checkpoints, any height/hash should pass.
	anyHash := crypto.Sha256([]byte("anything"))
	if !ValidateCheckpoint(42, anyHash, true) {
		t.Fatal("non-checkpoint height should always pass")
	}
	if !ValidateCheckpoint(0, anyHash, true) {
		t.Fatal("testnet with no checkpoints should pass any hash at height 0")
	}
	if !ValidateCheckpoint(500, anyHash, true) {
		t.Fatal("testnet with no checkpoints should pass any hash at height 500")
	}
}

func TestValidateCheckpoint_AllTestnetCheckpoints(t *testing.T) {
	// TestnetCheckpoints is empty — loop is a no-op, but verify it doesn't panic.
	for _, cp := range TestnetCheckpoints {
		if !ValidateCheckpoint(cp.Height, cp.Hash, true) {
			t.Fatalf("checkpoint at height %d should pass with correct hash", cp.Height)
		}
		wrongHash := crypto.Sha256([]byte("fake"))
		if ValidateCheckpoint(cp.Height, wrongHash, true) {
			t.Fatalf("checkpoint at height %d should reject wrong hash", cp.Height)
		}
	}
}

func TestValidateCheckpoint_MainnetEmpty(t *testing.T) {
	// Mainnet has no checkpoints yet — everything should pass.
	anyHash := crypto.Sha256([]byte("anything"))
	if !ValidateCheckpoint(0, anyHash, false) {
		t.Fatal("mainnet with no checkpoints should pass any hash")
	}
	if !ValidateCheckpoint(500, anyHash, false) {
		t.Fatal("mainnet with no checkpoints should pass any hash")
	}
}

func TestLatestCheckpointHeight(t *testing.T) {
	// Testnet checkpoints are cleared — latest should be 0.
	h := LatestCheckpointHeight(true)
	if h != 0 {
		t.Fatalf("testnet latest checkpoint: want 0, got %d", h)
	}
	h = LatestCheckpointHeight(false)
	if h != 0 {
		t.Fatalf("mainnet latest checkpoint: want 0, got %d", h)
	}
}

func TestIsCheckpointStale(t *testing.T) {
	// No testnet checkpoints → never stale.
	if IsCheckpointStale(0, true) {
		t.Fatal("testnet with no checkpoints should never be stale")
	}
	if IsCheckpointStale(100, true) {
		t.Fatal("testnet with no checkpoints should never be stale")
	}
	// Mainnet has no checkpoints → never stale.
	if IsCheckpointStale(0, false) {
		t.Fatal("mainnet with no checkpoints should never be stale")
	}
}

package consensus

import (
	"nous/crypto"
	"testing"
)

func TestValidateCheckpoint_CorrectHash(t *testing.T) {
	// Height 0 with the real testnet genesis hash should pass.
	genesisHash := hexHash("046efc6998a2cdd1780a32d421ffc5651d2bc9f1a915599aad20f392302e0fbc")
	if !ValidateCheckpoint(0, genesisHash, true) {
		t.Fatal("correct checkpoint hash should pass")
	}
}

func TestValidateCheckpoint_WrongHash(t *testing.T) {
	// Height 0 with a wrong hash should fail.
	wrongHash := crypto.Sha256([]byte("wrong"))
	if ValidateCheckpoint(0, wrongHash, true) {
		t.Fatal("wrong checkpoint hash should fail")
	}
}

func TestValidateCheckpoint_NonCheckpointHeight(t *testing.T) {
	// Height 42 is not a checkpoint — any hash should pass.
	anyHash := crypto.Sha256([]byte("anything"))
	if !ValidateCheckpoint(42, anyHash, true) {
		t.Fatal("non-checkpoint height should always pass")
	}
}

func TestValidateCheckpoint_AllTestnetCheckpoints(t *testing.T) {
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
	h := LatestCheckpointHeight(true)
	if h != 2000 {
		t.Fatalf("testnet latest checkpoint: want 2000, got %d", h)
	}
	h = LatestCheckpointHeight(false)
	if h != 0 {
		t.Fatalf("mainnet latest checkpoint: want 0, got %d", h)
	}
}

func TestIsCheckpointStale(t *testing.T) {
	// Below latest testnet checkpoint → stale.
	if !IsCheckpointStale(100, true) {
		t.Fatal("height 100 should be stale (latest checkpoint is 2000)")
	}
	// At latest checkpoint → not stale.
	if IsCheckpointStale(2000, true) {
		t.Fatal("height 2000 should not be stale")
	}
	// Above latest checkpoint → not stale.
	if IsCheckpointStale(3000, true) {
		t.Fatal("height 3000 should not be stale")
	}
	// Mainnet has no checkpoints → never stale.
	if IsCheckpointStale(0, false) {
		t.Fatal("mainnet with no checkpoints should never be stale")
	}
}

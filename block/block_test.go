package block

import (
	"bytes"
	"testing"

	"github.com/nous-chain/nous/crypto"
	"github.com/nous-chain/nous/tx"
)

// ============================================================
// Header serialization
// ============================================================

func TestSerializeDeserializeRoundTrip(t *testing.T) {
	h := &Header{
		Version:               1,
		PrevBlockHash:         crypto.Sha256([]byte("prev")),
		MerkleRoot:            crypto.Sha256([]byte("merkle")),
		Timestamp:             1700000000,
		DifficultyBits:        0x1d00ffff,
		VDFOutput:             []byte{0xAA, 0xBB, 0xCC},
		VDFProof:              []byte{0xDD, 0xEE},
		VDFIterations:         100000,
		CSPSolutionHash: crypto.Sha256([]byte("csp")),
		MinerPubKey:     []byte{0x02, 0x01, 0x02, 0x03},
		Nonce:                 42,
	}

	data := h.Serialize()
	h2, err := DeserializeHeader(data)
	if err != nil {
		t.Fatalf("DeserializeHeader failed: %v", err)
	}

	if h.Version != h2.Version {
		t.Fatalf("Version: want %d, got %d", h.Version, h2.Version)
	}
	if h.PrevBlockHash != h2.PrevBlockHash {
		t.Fatal("PrevBlockHash mismatch")
	}
	if h.MerkleRoot != h2.MerkleRoot {
		t.Fatal("MerkleRoot mismatch")
	}
	if h.Timestamp != h2.Timestamp {
		t.Fatalf("Timestamp: want %d, got %d", h.Timestamp, h2.Timestamp)
	}
	if h.DifficultyBits != h2.DifficultyBits {
		t.Fatalf("DifficultyBits: want %d, got %d", h.DifficultyBits, h2.DifficultyBits)
	}
	if !bytes.Equal(h.VDFOutput, h2.VDFOutput) {
		t.Fatal("VDFOutput mismatch")
	}
	if !bytes.Equal(h.VDFProof, h2.VDFProof) {
		t.Fatal("VDFProof mismatch")
	}
	if h.VDFIterations != h2.VDFIterations {
		t.Fatalf("VDFIterations: want %d, got %d", h.VDFIterations, h2.VDFIterations)
	}
	if h.CSPSolutionHash != h2.CSPSolutionHash {
		t.Fatal("CSPSolutionHash mismatch")
	}
	if !bytes.Equal(h.MinerPubKey, h2.MinerPubKey) {
		t.Fatal("MinerPubKey mismatch")
	}
	if h.Nonce != h2.Nonce {
		t.Fatalf("Nonce: want %d, got %d", h.Nonce, h2.Nonce)
	}
}

func TestSerializeDeserializeEmptyVarFields(t *testing.T) {
	h := &Header{
		Version:    1,
		Timestamp:  1700000000,
		VDFOutput:  []byte{},
		VDFProof:   []byte{},
		MinerPubKey: []byte{},
	}

	data := h.Serialize()
	h2, err := DeserializeHeader(data)
	if err != nil {
		t.Fatalf("DeserializeHeader failed: %v", err)
	}

	if h.Version != h2.Version {
		t.Fatalf("Version mismatch")
	}
	if len(h2.VDFOutput) != 0 {
		t.Fatalf("VDFOutput should be empty, got %d bytes", len(h2.VDFOutput))
	}
	if len(h2.VDFProof) != 0 {
		t.Fatalf("VDFProof should be empty, got %d bytes", len(h2.VDFProof))
	}
	if len(h2.MinerPubKey) != 0 {
		t.Fatalf("MinerPubKey should be empty, got %d bytes", len(h2.MinerPubKey))
	}
}

// ============================================================
// Deterministic hashing
// ============================================================

func TestHashDeterministic(t *testing.T) {
	h := &Header{
		Version:       1,
		PrevBlockHash: crypto.Sha256([]byte("prev")),
		MerkleRoot:    crypto.Sha256([]byte("merkle")),
		Timestamp:     1700000000,
		DifficultyBits: 0x1d00ffff,
		VDFOutput:     []byte{0x01, 0x02},
		VDFProof:      []byte{0x03},
		VDFIterations: 5000,
		MinerPubKey:   []byte{0x02, 0xAA},
		Nonce:         99,
	}

	hash1 := h.Hash()
	hash2 := h.Hash()

	if hash1 != hash2 {
		t.Fatal("same header should produce the same hash")
	}
	if hash1.IsZero() {
		t.Fatal("hash should not be zero")
	}
}

func TestFieldMutationChangesHash(t *testing.T) {
	h := &Header{
		Version:        1,
		PrevBlockHash:  crypto.Sha256([]byte("prev")),
		MerkleRoot:     crypto.Sha256([]byte("merkle")),
		Timestamp:      1700000000,
		DifficultyBits: 0x1d00ffff,
		VDFOutput:      []byte{0x01},
		VDFProof:       []byte{0x02},
		VDFIterations:  1000,
		MinerPubKey:    []byte{0x02},
		Nonce:          0,
	}
	original := h.Hash()

	// Mutate Nonce
	h.Nonce = 1
	if h.Hash() == original {
		t.Fatal("changing Nonce should change the hash")
	}
	h.Nonce = 0

	// Mutate Timestamp
	h.Timestamp = 1700000001
	if h.Hash() == original {
		t.Fatal("changing Timestamp should change the hash")
	}
	h.Timestamp = 1700000000

	// Mutate Version
	h.Version = 2
	if h.Hash() == original {
		t.Fatal("changing Version should change the hash")
	}
	h.Version = 1

	// Mutate DifficultyBits
	h.DifficultyBits = 0x1c00ffff
	if h.Hash() == original {
		t.Fatal("changing DifficultyBits should change the hash")
	}
	h.DifficultyBits = 0x1d00ffff

	// Mutate VDFIterations
	h.VDFIterations = 2000
	if h.Hash() == original {
		t.Fatal("changing VDFIterations should change the hash")
	}
	h.VDFIterations = 1000

	// Mutate PrevBlockHash
	h.PrevBlockHash = crypto.Sha256([]byte("other"))
	if h.Hash() == original {
		t.Fatal("changing PrevBlockHash should change the hash")
	}
	h.PrevBlockHash = crypto.Sha256([]byte("prev"))

	// Sanity: back to original
	if h.Hash() != original {
		t.Fatal("restoring all fields should restore original hash")
	}
}

// ============================================================
// Merkle tree
// ============================================================

func TestMerkleRootEmpty(t *testing.T) {
	root := ComputeMerkleRoot(nil)
	if !root.IsZero() {
		t.Fatal("empty list should produce zero hash")
	}
}

func TestMerkleRootSingleTx(t *testing.T) {
	h := crypto.Sha256([]byte("tx0"))
	root := ComputeMerkleRoot([]crypto.Hash{h})
	if root != h {
		t.Fatal("single tx: merkle root should equal the tx hash")
	}
}

func TestMerkleRootTwoTx(t *testing.T) {
	h0 := crypto.Sha256([]byte("tx0"))
	h1 := crypto.Sha256([]byte("tx1"))

	var combined [64]byte
	copy(combined[:32], h0[:])
	copy(combined[32:], h1[:])
	expected := crypto.DoubleSha256(combined[:])

	root := ComputeMerkleRoot([]crypto.Hash{h0, h1})
	if root != expected {
		t.Fatalf("two txs: want %s, got %s", expected, root)
	}
}

func TestMerkleRootThreeTx(t *testing.T) {
	h0 := crypto.Sha256([]byte("tx0"))
	h1 := crypto.Sha256([]byte("tx1"))
	h2 := crypto.Sha256([]byte("tx2"))

	// Level 1: [H(h0||h1), H(h2||h2)]  (h2 duplicated because odd)
	var pair01, pair22 [64]byte
	copy(pair01[:32], h0[:])
	copy(pair01[32:], h1[:])
	left := crypto.DoubleSha256(pair01[:])

	copy(pair22[:32], h2[:])
	copy(pair22[32:], h2[:])
	right := crypto.DoubleSha256(pair22[:])

	// Level 2: H(left||right)
	var combined [64]byte
	copy(combined[:32], left[:])
	copy(combined[32:], right[:])
	expected := crypto.DoubleSha256(combined[:])

	root := ComputeMerkleRoot([]crypto.Hash{h0, h1, h2})
	if root != expected {
		t.Fatalf("three txs: want %s, got %s", expected, root)
	}
}

func TestMerkleRootFiveTx(t *testing.T) {
	hashes := make([]crypto.Hash, 5)
	for i := range hashes {
		hashes[i] = crypto.Sha256([]byte{byte(i)})
	}

	root := ComputeMerkleRoot(hashes)
	if root.IsZero() {
		t.Fatal("5-tx merkle root should not be zero")
	}

	// Deterministic
	root2 := ComputeMerkleRoot(hashes)
	if root != root2 {
		t.Fatal("merkle root should be deterministic")
	}
}

func TestMerkleRootEightTx(t *testing.T) {
	hashes := make([]crypto.Hash, 8)
	for i := range hashes {
		hashes[i] = crypto.Sha256([]byte{byte(i + 100)})
	}

	root := ComputeMerkleRoot(hashes)
	if root.IsZero() {
		t.Fatal("8-tx merkle root should not be zero")
	}

	// 8 is a power of 2, so no duplication should occur.
	// Verify by manual bottom-up computation.
	level := make([]crypto.Hash, 8)
	copy(level, hashes)
	for len(level) > 1 {
		next := make([]crypto.Hash, len(level)/2)
		for i := 0; i < len(level); i += 2 {
			var combined [64]byte
			copy(combined[:32], level[i][:])
			copy(combined[32:], level[i+1][:])
			next[i/2] = crypto.DoubleSha256(combined[:])
		}
		level = next
	}
	if root != level[0] {
		t.Fatalf("8-tx: manual vs function mismatch")
	}
}

func TestMerkleRootOddDuplication(t *testing.T) {
	h0 := crypto.Sha256([]byte("a"))
	h1 := crypto.Sha256([]byte("b"))
	h2 := crypto.Sha256([]byte("c"))

	// With 3 hashes, h2 is duplicated at level 1.
	// Root should differ from both 2-hash and 4-hash trees.
	root3 := ComputeMerkleRoot([]crypto.Hash{h0, h1, h2})
	root2 := ComputeMerkleRoot([]crypto.Hash{h0, h1})

	if root3 == root2 {
		t.Fatal("3-hash tree should differ from 2-hash tree")
	}

	// Adding the duplicate explicitly should produce the same result.
	root4explicit := ComputeMerkleRoot([]crypto.Hash{h0, h1, h2, h2})
	if root3 != root4explicit {
		t.Fatal("3-hash tree should equal 4-hash tree with last element duplicated")
	}
}

func TestMerkleRootDoesNotMutateInput(t *testing.T) {
	hashes := []crypto.Hash{
		crypto.Sha256([]byte("x")),
		crypto.Sha256([]byte("y")),
		crypto.Sha256([]byte("z")),
	}
	original := make([]crypto.Hash, len(hashes))
	copy(original, hashes)

	ComputeMerkleRoot(hashes)

	for i := range hashes {
		if hashes[i] != original[i] {
			t.Fatalf("ComputeMerkleRoot mutated input at index %d", i)
		}
	}
}

// ============================================================
// Genesis block
// ============================================================

func TestGenesisBlock(t *testing.T) {
	_, pub, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pubKeyHash := crypto.Hash160(pub.SerializeCompressed())

	genesis := GenesisBlock(pubKeyHash, 0)
	if genesis == nil {
		t.Fatal("GenesisBlock returned nil")
	}

	// PrevBlockHash should be zero
	if !genesis.Header.PrevBlockHash.IsZero() {
		t.Fatal("genesis PrevBlockHash should be zero")
	}

	// Should have exactly 1 transaction (coinbase)
	if len(genesis.Transactions) != 1 {
		t.Fatalf("genesis should have 1 tx, got %d", len(genesis.Transactions))
	}

	// Coinbase check
	if !genesis.Transactions[0].IsCoinbase() {
		t.Fatal("genesis tx should be a coinbase")
	}

	// Reward should be 10 NOUS = 10_0000_0000 nou
	if genesis.Transactions[0].Outputs[0].Value != 10_0000_0000 {
		t.Fatalf("genesis reward: want 1000000000, got %d", genesis.Transactions[0].Outputs[0].Value)
	}

	// Version
	if genesis.Header.Version != 1 {
		t.Fatalf("genesis version: want 1, got %d", genesis.Header.Version)
	}

	// MerkleRoot should match the single coinbase tx
	expectedMerkle := ComputeMerkleRoot([]crypto.Hash{genesis.Transactions[0].TxID()})
	if genesis.Header.MerkleRoot != expectedMerkle {
		t.Fatal("genesis MerkleRoot does not match coinbase TxID")
	}
}

func TestGenesisBlockDeterministic(t *testing.T) {
	_, pub, _ := crypto.GenerateKeyPair()
	pubKeyHash := crypto.Hash160(pub.SerializeCompressed())

	ts := uint32(1735689600) // fixed timestamp for determinism test
	g1 := GenesisBlock(pubKeyHash, ts)
	g2 := GenesisBlock(pubKeyHash, ts)

	if g1.Header.Hash() != g2.Header.Hash() {
		t.Fatal("genesis block hash should be deterministic for the same pubKeyHash")
	}
}

// ============================================================
// Transaction integration
// ============================================================

func TestBlockWithMultipleTransactions(t *testing.T) {
	_, pub, _ := crypto.GenerateKeyPair()
	pubKeyHash := crypto.Hash160(pub.SerializeCompressed())

	coinbase := tx.NewCoinbase(1, 10_0000_0000, pubKeyHash, "")
	regular := &tx.Transaction{
		Version: 1,
		Inputs: []tx.TxInput{
			{
				PrevOut:   tx.OutPoint{TxID: coinbase.TxID(), Index: 0},
				ScriptSig: []byte{0x01, 0x02},
				Sequence:  0xFFFFFFFF,
			},
		},
		Outputs: []tx.TxOutput{
			{Value: 5_0000_0000, ScriptPubKey: []byte{0x76}},
			{Value: 4_9999_0000, ScriptPubKey: []byte{0x76}},
		},
		LockTime: 0,
	}

	txIDs := []crypto.Hash{coinbase.TxID(), regular.TxID()}
	merkleRoot := ComputeMerkleRoot(txIDs)

	blk := &Block{
		Header: Header{
			Version:    1,
			MerkleRoot: merkleRoot,
			Timestamp:  1700000000,
			VDFOutput:  []byte{},
			VDFProof:   []byte{},
			MinerPubKey: []byte{},
		},
		Transactions: []*tx.Transaction{coinbase, regular},
	}

	// Verify MerkleRoot consistency
	recomputed := ComputeMerkleRoot(txIDs)
	if blk.Header.MerkleRoot != recomputed {
		t.Fatal("MerkleRoot should be consistent")
	}

	// Block hash should be non-zero
	if blk.Header.Hash().IsZero() {
		t.Fatal("block hash should not be zero")
	}
}

// ============================================================
// Deserialization safety: too-short header data
// ============================================================

func TestDeserializeHeaderTooShort(t *testing.T) {
	// Empty data.
	_, err := DeserializeHeader(nil)
	if err == nil {
		t.Fatal("nil data should fail")
	}
	// Way too short.
	_, err = DeserializeHeader(make([]byte, 10))
	if err == nil {
		t.Fatal("10-byte data should fail")
	}
	// Just under minimum (126).
	_, err = DeserializeHeader(make([]byte, 125))
	if err == nil {
		t.Fatal("125-byte data should fail")
	}
}

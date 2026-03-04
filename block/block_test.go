package block

import (
	"testing"

	"nous/crypto"
	"nous/tx"
)

// ============================================================
// Header serialization
// ============================================================

func TestSerializeDeserializeRoundTrip(t *testing.T) {
	h := &Header{
		Version:         1,
		PrevBlockHash:   crypto.Sha256([]byte("prev")),
		MerkleRoot:      crypto.Sha256([]byte("merkle")),
		Timestamp:       1700000000,
		DifficultyBits:  0x1d00ffff,
		Seed:            42,
		SATSolutionHash: crypto.Sha256([]byte("sat")),
		UTXOSetHash:     crypto.Sha256([]byte("utxo")),
	}

	data := h.Serialize()
	if len(data) != HeaderSize {
		t.Fatalf("serialized header size: want %d, got %d", HeaderSize, len(data))
	}

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
	if h.Seed != h2.Seed {
		t.Fatalf("Seed: want %d, got %d", h.Seed, h2.Seed)
	}
	if h.SATSolutionHash != h2.SATSolutionHash {
		t.Fatal("SATSolutionHash mismatch")
	}
	if h.UTXOSetHash != h2.UTXOSetHash {
		t.Fatal("UTXOSetHash mismatch")
	}
}

func TestHeaderFixedSize(t *testing.T) {
	h := &Header{
		Version:    1,
		Timestamp:  1700000000,
	}

	data := h.Serialize()
	if len(data) != HeaderSize {
		t.Fatalf("header size: want %d, got %d", HeaderSize, len(data))
	}
}

// ============================================================
// Deterministic hashing
// ============================================================

func TestHashDeterministic(t *testing.T) {
	h := &Header{
		Version:        1,
		PrevBlockHash:  crypto.Sha256([]byte("prev")),
		MerkleRoot:     crypto.Sha256([]byte("merkle")),
		Timestamp:      1700000000,
		DifficultyBits: 0x1d00ffff,
		Seed:           99,
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
		Seed:           0,
	}
	original := h.Hash()

	// Mutate Seed
	h.Seed = 1
	if h.Hash() == original {
		t.Fatal("changing Seed should change the hash")
	}
	h.Seed = 0

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

	var pair01, pair22 [64]byte
	copy(pair01[:32], h0[:])
	copy(pair01[32:], h1[:])
	left := crypto.DoubleSha256(pair01[:])

	copy(pair22[:32], h2[:])
	copy(pair22[32:], h2[:])
	right := crypto.DoubleSha256(pair22[:])

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

	root3 := ComputeMerkleRoot([]crypto.Hash{h0, h1, h2})
	root2 := ComputeMerkleRoot([]crypto.Hash{h0, h1})

	if root3 == root2 {
		t.Fatal("3-hash tree should differ from 2-hash tree")
	}

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

	// Testnet genesis: P2PKH with 1 NOUS reward.
	genesis := GenesisBlock(pubKeyHash, 1700000000, 0x2000ffff, true)
	if genesis == nil {
		t.Fatal("GenesisBlock returned nil")
	}

	// PrevBlockHash should be zero.
	if !genesis.Header.PrevBlockHash.IsZero() {
		t.Fatal("genesis PrevBlockHash should be zero")
	}

	// Should have exactly 1 transaction (coinbase).
	if len(genesis.Transactions) != 1 {
		t.Fatalf("genesis should have 1 tx, got %d", len(genesis.Transactions))
	}

	// Coinbase check.
	if !genesis.Transactions[0].IsCoinbase() {
		t.Fatal("genesis tx should be a coinbase")
	}

	// Reward should be 1 NOUS = 1_00000000 nou.
	if genesis.Transactions[0].Outputs[0].Amount != 1_00000000 {
		t.Fatalf("genesis reward: want 100000000, got %d", genesis.Transactions[0].Outputs[0].Amount)
	}

	// Version.
	if genesis.Header.Version != 1 {
		t.Fatalf("genesis version: want 1, got %d", genesis.Header.Version)
	}

	// MerkleRoot should match the single coinbase tx.
	expectedMerkle := ComputeMerkleRoot([]crypto.Hash{genesis.Transactions[0].TxID()})
	if genesis.Header.MerkleRoot != expectedMerkle {
		t.Fatal("genesis MerkleRoot does not match coinbase TxID")
	}
}

func TestGenesisBlockMainnetOpReturn(t *testing.T) {
	// Mainnet genesis: OP_RETURN with 0 reward (fair launch, no premine).
	genesis := GenesisBlock(nil, 1772866800, 0x1d00ffff, false)
	if genesis == nil {
		t.Fatal("GenesisBlock returned nil")
	}

	cb := genesis.Transactions[0]
	if !cb.IsCoinbase() {
		t.Fatal("genesis tx should be a coinbase")
	}

	// Amount should be 0 (unspendable).
	if cb.Outputs[0].Amount != 0 {
		t.Fatalf("mainnet genesis reward: want 0, got %d", cb.Outputs[0].Amount)
	}

	// Output script should start with OP_RETURN.
	if !tx.IsUnspendable(cb.Outputs[0].PkScript) {
		t.Fatal("mainnet genesis output should be unspendable (OP_RETURN)")
	}

	// Script should contain the genesis message.
	msg := "NOUS Genesis 2026-03-07 / Cogito, ergo sum."
	script := cb.Outputs[0].PkScript
	// OP_RETURN (1 byte) + length (1 byte) + message
	if len(script) != 2+len(msg) {
		t.Fatalf("script length: want %d, got %d", 2+len(msg), len(script))
	}
	if string(script[2:]) != msg {
		t.Fatalf("genesis message: want %q, got %q", msg, string(script[2:]))
	}

	// MerkleRoot should match.
	expectedMerkle := ComputeMerkleRoot([]crypto.Hash{cb.TxID()})
	if genesis.Header.MerkleRoot != expectedMerkle {
		t.Fatal("genesis MerkleRoot does not match coinbase TxID")
	}
}

func TestGenesisBlockDeterministic(t *testing.T) {
	_, pub, _ := crypto.GenerateKeyPair()
	pubKeyHash := crypto.Hash160(pub.SerializeCompressed())

	// Testnet: deterministic for same pubKeyHash and timestamp.
	ts := uint32(1735689600)
	g1 := GenesisBlock(pubKeyHash, ts, 0x2000ffff, true)
	g2 := GenesisBlock(pubKeyHash, ts, 0x2000ffff, true)
	if g1.Header.Hash() != g2.Header.Hash() {
		t.Fatal("testnet genesis hash should be deterministic")
	}

	// Mainnet: deterministic (ignores pubKeyHash, uses fixed OP_RETURN message).
	g3 := GenesisBlock(nil, ts, 0x1d00ffff, false)
	g4 := GenesisBlock(nil, ts, 0x1d00ffff, false)
	if g3.Header.Hash() != g4.Header.Hash() {
		t.Fatal("mainnet genesis hash should be deterministic")
	}
}

// ============================================================
// Transaction integration
// ============================================================

func TestBlockWithMultipleTransactions(t *testing.T) {
	_, pub, _ := crypto.GenerateKeyPair()
	pubKeyHash := crypto.Hash160(pub.SerializeCompressed())

	coinbase := tx.NewCoinbaseTx(1, 1_00000000, tx.CreateP2PKHLockScript(pubKeyHash), tx.ChainIDNous)
	regular := &tx.Transaction{
		Version: 1,
		Inputs: []tx.TxIn{
			{
				PrevOut:         tx.OutPoint{TxID: coinbase.TxID(), Index: 0},
				SignatureScript: []byte{0x01, 0x02},
				Sequence:        0xFFFFFFFF,
			},
		},
		Outputs: []tx.TxOut{
			{Amount: 5000_0000, PkScript: []byte{0x76}},
			{Amount: 4999_0000, PkScript: []byte{0x76}},
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
		},
		Transactions: []*tx.Transaction{coinbase, regular},
	}

	// Verify MerkleRoot consistency.
	recomputed := ComputeMerkleRoot(txIDs)
	if blk.Header.MerkleRoot != recomputed {
		t.Fatal("MerkleRoot should be consistent")
	}

	// Block hash should be non-zero.
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
	// Just under minimum (148).
	_, err = DeserializeHeader(make([]byte, 147))
	if err == nil {
		t.Fatal("147-byte data should fail")
	}
}

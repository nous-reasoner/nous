package tx

import (
	"bytes"
	"testing"

	"nous/crypto"
)

// ============================================================
// 1. TxID excludes SignatureScript (malleability resistant)
// ============================================================

func TestTxID_ExcludesSignature(t *testing.T) {
	txn := &Transaction{
		Version: 2,
		ChainID: ChainIDNous,
		Inputs: []TxIn{
			{
				PrevOut:         OutPoint{TxID: crypto.Sha256([]byte("prev")), Index: 0},
				SignatureScript: []byte{0x01, 0x02, 0x03},
				Sequence:        0xFFFFFFFF,
			},
		},
		Outputs: []TxOut{
			{Amount: 50_0000_0000, PkScript: []byte{0x76, 0xa9}},
		},
	}

	id1 := txn.TxID()

	// Change the SignatureScript.
	txn.Inputs[0].SignatureScript = []byte{0xAA, 0xBB, 0xCC, 0xDD}
	id2 := txn.TxID()

	if id1 != id2 {
		t.Fatal("TxID should not change when SignatureScript changes")
	}
}

// ============================================================
// 2. TxHash includes SignatureScript
// ============================================================

func TestTxHash_IncludesSignature(t *testing.T) {
	txn := &Transaction{
		Version: 2,
		ChainID: ChainIDNous,
		Inputs: []TxIn{
			{
				PrevOut:         OutPoint{TxID: crypto.Sha256([]byte("prev")), Index: 0},
				SignatureScript: []byte{0x01, 0x02, 0x03},
				Sequence:        0xFFFFFFFF,
			},
		},
		Outputs: []TxOut{
			{Amount: 50_0000_0000, PkScript: []byte{0x76, 0xa9}},
		},
	}

	hash1 := txn.TxHash()

	// Change the SignatureScript.
	txn.Inputs[0].SignatureScript = []byte{0xAA, 0xBB, 0xCC, 0xDD}
	hash2 := txn.TxHash()

	if hash1 == hash2 {
		t.Fatal("TxHash should change when SignatureScript changes")
	}
}

// ============================================================
// 3. SumOutputs detects overflow
// ============================================================

func TestAmountOverflow(t *testing.T) {
	outs := []TxOut{
		{Amount: MaxAmount, PkScript: []byte{0x76}},
		{Amount: 1, PkScript: []byte{0x76}},
	}
	_, err := SumOutputs(outs)
	if err == nil {
		t.Fatal("SumOutputs should detect overflow past MaxAmount")
	}
}

// ============================================================
// 4. Dust limit: output below DustLimit rejected
// ============================================================

func TestDustLimit(t *testing.T) {
	priv, pub, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	_ = priv
	pubKeyHash := crypto.Hash160(pub.SerializeCompressed())

	utxos := NewUTXOSet()
	fakeOp := OutPoint{TxID: crypto.Sha256([]byte("dust-test")), Index: 0}
	utxos.Add(fakeOp, TxOut{Amount: 10_0000_0000, PkScript: CreateP2PKHLockScript(pubKeyHash)}, 0, false)

	dustTx := &Transaction{
		Version: 2,
		ChainID: ChainIDNous,
		Inputs:  []TxIn{{PrevOut: fakeOp, Sequence: 0xFFFFFFFF}},
		Outputs: []TxOut{{Amount: DustLimit - 1, PkScript: CreateP2PKHLockScript(pubKeyHash)}},
	}

	err = ValidateTx(dustTx, utxos, 100)
	if err == nil {
		t.Fatal("output below dust limit should be rejected")
	}
}

// ============================================================
// 5. Coinbase height encoding
// ============================================================

func TestCoinbase_HeightEncoding(t *testing.T) {
	tests := []struct {
		height   uint64
		expected []byte // first bytes of SignatureScript: [len][bytes...]
	}{
		{0, []byte{1, 0}},                // height 0 → [1, 0x00]
		{1, []byte{1, 1}},                // height 1 → [1, 0x01]
		{255, []byte{1, 0xFF}},            // height 255 → [1, 0xFF]
		{256, []byte{2, 0x00, 0x01}},      // height 256 → [2, 0x00, 0x01]
		{70000, []byte{3, 0x70, 0x11, 0x01}}, // height 70000 → [3, LE bytes]
	}

	for _, tc := range tests {
		cb := NewCoinbaseTx(tc.height, Coin, []byte{0x76}, ChainIDNous)
		ss := cb.Inputs[0].SignatureScript

		if len(ss) < len(tc.expected) {
			t.Fatalf("height %d: SignatureScript too short: %x", tc.height, ss)
		}
		for i, b := range tc.expected {
			if ss[i] != b {
				t.Fatalf("height %d: byte %d: want 0x%02x, got 0x%02x (full: %x)",
					tc.height, i, b, ss[i], ss)
			}
		}
	}
}

// ============================================================
// 6. Transaction weight
// ============================================================

func TestTxWeight(t *testing.T) {
	txn := &Transaction{
		Version: 2,
		ChainID: ChainIDNous,
		Inputs: []TxIn{
			{
				PrevOut:         OutPoint{TxID: crypto.Sha256([]byte("prev")), Index: 0},
				SignatureScript: []byte{0x01, 0x02, 0x03, 0x04, 0x05},
				Sequence:        0xFFFFFFFF,
			},
		},
		Outputs: []TxOut{
			{Amount: 50_0000_0000, PkScript: CreateP2PKHLockScript(make([]byte, 20))},
		},
	}

	w := TxWeight(txn)
	baseSize := int64(len(txn.SerializeNoWitness()))
	fullSize := int64(len(txn.Serialize()))
	sigSize := fullSize - baseSize

	expected := baseSize*UTXOWeight + sigSize*SignatureWeight
	if w != expected {
		t.Fatalf("TxWeight: want %d, got %d (base=%d, full=%d, sig=%d)",
			expected, w, baseSize, fullSize, sigSize)
	}

	// Weight should be > 0.
	if w <= 0 {
		t.Fatal("weight should be positive")
	}
}

// ============================================================
// 7. ChainID mismatch rejected by ValidateTx
// ============================================================

func TestChainID_Mismatch(t *testing.T) {
	_, pub, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pubKeyHash := crypto.Hash160(pub.SerializeCompressed())

	utxos := NewUTXOSet()
	fakeOp := OutPoint{TxID: crypto.Sha256([]byte("chain-test")), Index: 0}
	utxos.Add(fakeOp, TxOut{Amount: 10_0000_0000, PkScript: CreateP2PKHLockScript(pubKeyHash)}, 0, false)

	badChain := [4]byte{0xBA, 0xAD, 0xBE, 0xEF}
	txn := &Transaction{
		Version: 2,
		ChainID: badChain,
		Inputs:  []TxIn{{PrevOut: fakeOp, Sequence: 0xFFFFFFFF}},
		Outputs: []TxOut{{Amount: 9_0000_0000, PkScript: CreateP2PKHLockScript(pubKeyHash)}},
	}

	err = ValidateTx(txn, utxos, 100)
	if err == nil {
		t.Fatal("wrong ChainID should be rejected")
	}
}

// ============================================================
// Serialize / Deserialize round-trip
// ============================================================

func TestSerializeDeserializeRoundTrip(t *testing.T) {
	original := &Transaction{
		Version: 2,
		ChainID: ChainIDNous,
		Inputs: []TxIn{
			{
				PrevOut:         OutPoint{TxID: crypto.Sha256([]byte("prev")), Index: 0},
				SignatureScript: []byte{0x01, 0x02, 0x03},
				Sequence:        0xFFFFFFFF,
			},
			{
				PrevOut:         OutPoint{TxID: crypto.Sha256([]byte("prev2")), Index: 1},
				SignatureScript: []byte{0x04, 0x05},
				Sequence:        0xFFFFFFFE,
			},
		},
		Outputs: []TxOut{
			{Amount: 50_0000_0000, ScriptVersion: 0, PkScript: []byte{0x76, 0xa9}},
			{Amount: 25_0000_0000, ScriptVersion: 0, PkScript: []byte{0x76, 0xa9, 0x14}},
		},
		LockTime:     100,
		ExpiryHeight: 500,
	}

	data := original.Serialize()
	decoded, err := Deserialize(data)
	if err != nil {
		t.Fatalf("Deserialize failed: %v", err)
	}

	if original.Version != decoded.Version {
		t.Fatalf("Version: want %d, got %d", original.Version, decoded.Version)
	}
	if original.ChainID != decoded.ChainID {
		t.Fatalf("ChainID mismatch")
	}
	if original.LockTime != decoded.LockTime {
		t.Fatalf("LockTime: want %d, got %d", original.LockTime, decoded.LockTime)
	}
	if original.ExpiryHeight != decoded.ExpiryHeight {
		t.Fatalf("ExpiryHeight: want %d, got %d", original.ExpiryHeight, decoded.ExpiryHeight)
	}
	if len(original.Inputs) != len(decoded.Inputs) {
		t.Fatalf("input count: want %d, got %d", len(original.Inputs), len(decoded.Inputs))
	}
	for i := range original.Inputs {
		if original.Inputs[i].PrevOut != decoded.Inputs[i].PrevOut {
			t.Fatalf("input %d PrevOut mismatch", i)
		}
		if !bytes.Equal(original.Inputs[i].SignatureScript, decoded.Inputs[i].SignatureScript) {
			t.Fatalf("input %d SignatureScript mismatch", i)
		}
		if original.Inputs[i].Sequence != decoded.Inputs[i].Sequence {
			t.Fatalf("input %d Sequence mismatch", i)
		}
	}
	if len(original.Outputs) != len(decoded.Outputs) {
		t.Fatalf("output count: want %d, got %d", len(original.Outputs), len(decoded.Outputs))
	}
	for i := range original.Outputs {
		if original.Outputs[i].Amount != decoded.Outputs[i].Amount {
			t.Fatalf("output %d Amount: want %d, got %d", i, original.Outputs[i].Amount, decoded.Outputs[i].Amount)
		}
		if original.Outputs[i].ScriptVersion != decoded.Outputs[i].ScriptVersion {
			t.Fatalf("output %d ScriptVersion: want %d, got %d", i, original.Outputs[i].ScriptVersion, decoded.Outputs[i].ScriptVersion)
		}
		if !bytes.Equal(original.Outputs[i].PkScript, decoded.Outputs[i].PkScript) {
			t.Fatalf("output %d PkScript mismatch", i)
		}
	}

	// Serialized forms should match.
	if !bytes.Equal(original.Serialize(), decoded.Serialize()) {
		t.Fatal("re-serialized form does not match original")
	}
}

// ============================================================
// TxID deterministic
// ============================================================

func TestTxIDDeterministic(t *testing.T) {
	txn := &Transaction{
		Version: 2,
		ChainID: ChainIDNous,
		Inputs: []TxIn{
			{
				PrevOut:         OutPoint{TxID: crypto.Hash{}, Index: 0xFFFFFFFF},
				SignatureScript: []byte{0x04, 0x01, 0x00, 0x00, 0x00},
				Sequence:        0xFFFFFFFF,
			},
		},
		Outputs: []TxOut{
			{Amount: 50_0000_0000, PkScript: []byte{0x76}},
		},
	}

	id1 := txn.TxID()
	id2 := txn.TxID()
	if id1 != id2 {
		t.Fatal("TxID should be deterministic")
	}
	if id1.IsZero() {
		t.Fatal("TxID should not be zero")
	}
}

// ============================================================
// VarInt encode/decode boundary values
// ============================================================

func TestVarIntRoundTrip(t *testing.T) {
	tests := []uint64{
		0,
		0xFC,        // max single-byte
		0xFD,        // min 2-byte
		0xFFFF,      // max 2-byte
		0x10000,     // min 4-byte
		0xFFFFFFFF,  // max 4-byte
		0x100000000, // min 8-byte
	}

	for _, val := range tests {
		var buf bytes.Buffer
		writeVarInt(&buf, val)
		r := bytes.NewReader(buf.Bytes())
		got, err := readVarInt(r)
		if err != nil {
			t.Fatalf("readVarInt(%d): %v", val, err)
		}
		if got != val {
			t.Fatalf("VarInt round-trip: want %d, got %d", val, got)
		}
	}
}

func TestVarIntEncoding(t *testing.T) {
	// Single byte: value < 0xFD
	var buf bytes.Buffer
	writeVarInt(&buf, 0)
	if buf.Len() != 1 {
		t.Fatalf("VarInt(0) should be 1 byte, got %d", buf.Len())
	}

	buf.Reset()
	writeVarInt(&buf, 0xFC)
	if buf.Len() != 1 {
		t.Fatalf("VarInt(0xFC) should be 1 byte, got %d", buf.Len())
	}

	// 2-byte: 0xFD prefix + uint16
	buf.Reset()
	writeVarInt(&buf, 0xFD)
	if buf.Len() != 3 {
		t.Fatalf("VarInt(0xFD) should be 3 bytes, got %d", buf.Len())
	}

	// 4-byte: 0xFE prefix + uint32
	buf.Reset()
	writeVarInt(&buf, 0x10000)
	if buf.Len() != 5 {
		t.Fatalf("VarInt(0x10000) should be 5 bytes, got %d", buf.Len())
	}

	// 8-byte: 0xFF prefix + uint64
	buf.Reset()
	writeVarInt(&buf, 0x100000000)
	if buf.Len() != 9 {
		t.Fatalf("VarInt(0x100000000) should be 9 bytes, got %d", buf.Len())
	}
}

// ============================================================
// NewCoinbaseTx
// ============================================================

func TestNewCoinbaseStructure(t *testing.T) {
	pubKeyHash := make([]byte, 20)
	for i := range pubKeyHash {
		pubKeyHash[i] = byte(i)
	}

	cb := NewCoinbaseTx(42, 50_0000_0000, CreateP2PKHLockScript(pubKeyHash), ChainIDNous)

	if !cb.IsCoinbase() {
		t.Fatal("NewCoinbaseTx should produce a coinbase transaction")
	}

	if len(cb.Outputs) != 1 {
		t.Fatalf("coinbase should have 1 output, got %d", len(cb.Outputs))
	}

	if cb.Outputs[0].Amount != 50_0000_0000 {
		t.Fatalf("coinbase reward: want 5000000000, got %d", cb.Outputs[0].Amount)
	}

	// Output should be a valid P2PKH script.
	extracted := ExtractPubKeyHashFromP2PKH(cb.Outputs[0].PkScript)
	if extracted == nil {
		t.Fatal("coinbase output should have a P2PKH script")
	}
	if !bytes.Equal(extracted, pubKeyHash) {
		t.Fatal("coinbase P2PKH hash mismatch")
	}

	// SignatureScript should contain the height.
	if len(cb.Inputs[0].SignatureScript) == 0 {
		t.Fatal("coinbase SignatureScript should not be empty")
	}

	// ChainID should be set.
	if cb.ChainID != ChainIDNous {
		t.Fatal("coinbase ChainID should be ChainIDNous")
	}

	// Version should be 2.
	if cb.Version != 2 {
		t.Fatalf("coinbase Version: want 2, got %d", cb.Version)
	}
}

// ============================================================
// P2PKH script generation
// ============================================================

func TestP2PKHLockScript(t *testing.T) {
	hash := make([]byte, 20)
	for i := range hash {
		hash[i] = byte(i + 1)
	}

	script := CreateP2PKHLockScript(hash)

	if len(script) != 25 {
		t.Fatalf("P2PKH lock script should be 25 bytes, got %d", len(script))
	}
	if script[0] != OpDup {
		t.Fatal("script[0] should be OP_DUP")
	}
	if script[1] != OpHash160 {
		t.Fatal("script[1] should be OP_HASH160")
	}
	if script[2] != 20 {
		t.Fatal("script[2] should be 20 (push 20 bytes)")
	}
	if !bytes.Equal(script[3:23], hash) {
		t.Fatal("script hash mismatch")
	}
	if script[23] != OpEqualVerify {
		t.Fatal("script[23] should be OP_EQUALVERIFY")
	}
	if script[24] != OpCheckSig {
		t.Fatal("script[24] should be OP_CHECKSIG")
	}
}

func TestP2PKHUnlockScript(t *testing.T) {
	sig := []byte{0x30, 0x44} // fake sig bytes
	pubKey := []byte{0x02, 0xAA, 0xBB}

	script := CreateP2PKHUnlockScript(sig, pubKey)

	expectedLen := 1 + len(sig) + 1 + len(pubKey)
	if len(script) != expectedLen {
		t.Fatalf("unlock script length: want %d, got %d", expectedLen, len(script))
	}
	if script[0] != byte(len(sig)) {
		t.Fatal("first byte should be sig length")
	}
	if !bytes.Equal(script[1:1+len(sig)], sig) {
		t.Fatal("sig data mismatch")
	}
	offset := 1 + len(sig)
	if script[offset] != byte(len(pubKey)) {
		t.Fatal("pubkey length byte mismatch")
	}
	if !bytes.Equal(script[offset+1:], pubKey) {
		t.Fatal("pubkey data mismatch")
	}
}

// ============================================================
// Script engine: valid P2PKH
// ============================================================

func TestScriptEngineValidP2PKH(t *testing.T) {
	priv, pub, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pubKeyHash := crypto.Hash160(pub.SerializeCompressed())

	// Create a funding transaction (coinbase).
	fundingTx := NewCoinbaseTx(0, 50_0000_0000, CreateP2PKHLockScript(pubKeyHash), ChainIDNous)

	// Create a spending transaction.
	spendTx := &Transaction{
		Version: 2,
		ChainID: ChainIDNous,
		Inputs: []TxIn{
			{
				PrevOut:  OutPoint{TxID: fundingTx.TxID(), Index: 0},
				Sequence: 0xFFFFFFFF,
			},
		},
		Outputs: []TxOut{
			{Amount: 50_0000_0000, PkScript: CreateP2PKHLockScript(pubKeyHash)},
		},
	}

	// Sign the spending transaction.
	subscript := fundingTx.Outputs[0].PkScript
	sigHash := spendTx.SigHash(0, subscript)
	sig, err := crypto.Sign(priv, sigHash)
	if err != nil {
		t.Fatal(err)
	}

	// Set the unlock script.
	spendTx.Inputs[0].SignatureScript = CreateP2PKHUnlockScript(sig.Bytes(), pub.SerializeCompressed())

	// Execute the script.
	ok := ExecuteScript(spendTx.Inputs[0].SignatureScript, fundingTx.Outputs[0].PkScript, spendTx, 0)
	if !ok {
		t.Fatal("valid P2PKH script should verify successfully")
	}
}

// ============================================================
// Script engine: wrong signature rejected
// ============================================================

func TestScriptEngineWrongSignature(t *testing.T) {
	priv, pub, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pubKeyHash := crypto.Hash160(pub.SerializeCompressed())

	fundingTx := NewCoinbaseTx(0, 50_0000_0000, CreateP2PKHLockScript(pubKeyHash), ChainIDNous)

	spendTx := &Transaction{
		Version: 2,
		ChainID: ChainIDNous,
		Inputs: []TxIn{
			{
				PrevOut:  OutPoint{TxID: fundingTx.TxID(), Index: 0},
				Sequence: 0xFFFFFFFF,
			},
		},
		Outputs: []TxOut{
			{Amount: 50_0000_0000, PkScript: CreateP2PKHLockScript(pubKeyHash)},
		},
	}

	// Sign with a wrong hash.
	wrongHash := crypto.Sha256([]byte("wrong data"))
	sig, err := crypto.Sign(priv, wrongHash)
	if err != nil {
		t.Fatal(err)
	}

	spendTx.Inputs[0].SignatureScript = CreateP2PKHUnlockScript(sig.Bytes(), pub.SerializeCompressed())

	ok := ExecuteScript(spendTx.Inputs[0].SignatureScript, fundingTx.Outputs[0].PkScript, spendTx, 0)
	if ok {
		t.Fatal("wrong signature should be rejected")
	}
}

// ============================================================
// UTXO set: add, spend, get, balance
// ============================================================

func TestUTXOSetAddSpendGet(t *testing.T) {
	utxos := NewUTXOSet()
	pubKeyHash := make([]byte, 20)
	for i := range pubKeyHash {
		pubKeyHash[i] = byte(i)
	}

	op := OutPoint{TxID: crypto.Sha256([]byte("tx1")), Index: 0}
	output := TxOut{Amount: 100, PkScript: CreateP2PKHLockScript(pubKeyHash)}

	// Add
	utxos.Add(op, output, 1, false)
	got := utxos.Get(op)
	if got == nil {
		t.Fatal("UTXO should exist after Add")
	}
	if got.Output.Amount != 100 {
		t.Fatalf("UTXO value: want 100, got %d", got.Output.Amount)
	}

	// Balance
	bal := utxos.GetBalance(pubKeyHash)
	if bal != 100 {
		t.Fatalf("balance: want 100, got %d", bal)
	}

	// Spend
	ok := utxos.Spend(op)
	if !ok {
		t.Fatal("Spend should return true")
	}
	if utxos.Get(op) != nil {
		t.Fatal("UTXO should be gone after Spend")
	}

	// Balance after spend
	bal = utxos.GetBalance(pubKeyHash)
	if bal != 0 {
		t.Fatalf("balance after spend: want 0, got %d", bal)
	}

	// Double spend
	ok = utxos.Spend(op)
	if ok {
		t.Fatal("double Spend should return false")
	}
}

func TestUTXOSetAddTransaction(t *testing.T) {
	utxos := NewUTXOSet()
	pubKeyHash := make([]byte, 20)
	for i := range pubKeyHash {
		pubKeyHash[i] = byte(i)
	}

	cb := NewCoinbaseTx(0, 50_0000_0000, CreateP2PKHLockScript(pubKeyHash), ChainIDNous)
	utxos.AddTransaction(cb, 0)

	txID := cb.TxID()
	got := utxos.Get(OutPoint{TxID: txID, Index: 0})
	if got == nil {
		t.Fatal("UTXO should exist after AddTransaction")
	}
	if got.Output.Amount != 50_0000_0000 {
		t.Fatalf("UTXO value: want 5000000000, got %d", got.Output.Amount)
	}

	bal := utxos.GetBalance(pubKeyHash)
	if bal != 50_0000_0000 {
		t.Fatalf("balance: want 5000000000, got %d", bal)
	}
}

// ============================================================
// Transaction validation: valid tx passes
// ============================================================

func TestValidateTransactionValid(t *testing.T) {
	priv, pub, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pubKeyHash := crypto.Hash160(pub.SerializeCompressed())

	// Create and add coinbase to UTXO set.
	cb := NewCoinbaseTx(0, 50_0000_0000, CreateP2PKHLockScript(pubKeyHash), ChainIDNous)
	utxos := NewUTXOSet()
	utxos.AddTransaction(cb, 0)

	// Create spending transaction.
	spendTx := &Transaction{
		Version: 2,
		ChainID: ChainIDNous,
		Inputs: []TxIn{
			{
				PrevOut:  OutPoint{TxID: cb.TxID(), Index: 0},
				Sequence: 0xFFFFFFFF,
			},
		},
		Outputs: []TxOut{
			{Amount: 49_0000_0000, PkScript: CreateP2PKHLockScript(pubKeyHash)},
		},
	}

	// Sign it.
	subscript := cb.Outputs[0].PkScript
	sigHash := spendTx.SigHash(0, subscript)
	sig, err := crypto.Sign(priv, sigHash)
	if err != nil {
		t.Fatal(err)
	}
	spendTx.Inputs[0].SignatureScript = CreateP2PKHUnlockScript(sig.Bytes(), pub.SerializeCompressed())

	if err := ValidateTransaction(spendTx, utxos, 100); err != nil {
		t.Fatalf("valid transaction should pass: %v", err)
	}
}

// ============================================================
// Transaction validation: double-spend rejected
// ============================================================

func TestValidateTransactionDoubleSpend(t *testing.T) {
	priv, pub, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pubKeyHash := crypto.Hash160(pub.SerializeCompressed())

	cb := NewCoinbaseTx(0, 50_0000_0000, CreateP2PKHLockScript(pubKeyHash), ChainIDNous)
	utxos := NewUTXOSet()
	utxos.AddTransaction(cb, 0)

	// First spend.
	spend1 := &Transaction{
		Version: 2,
		ChainID: ChainIDNous,
		Inputs: []TxIn{
			{
				PrevOut:  OutPoint{TxID: cb.TxID(), Index: 0},
				Sequence: 0xFFFFFFFF,
			},
		},
		Outputs: []TxOut{
			{Amount: 50_0000_0000, PkScript: CreateP2PKHLockScript(pubKeyHash)},
		},
	}
	subscript := cb.Outputs[0].PkScript
	sigHash := spend1.SigHash(0, subscript)
	sig, _ := crypto.Sign(priv, sigHash)
	spend1.Inputs[0].SignatureScript = CreateP2PKHUnlockScript(sig.Bytes(), pub.SerializeCompressed())

	// Validate and apply first spend.
	if err := ValidateTransaction(spend1, utxos, 100); err != nil {
		t.Fatalf("first spend should be valid: %v", err)
	}
	utxos.Spend(spend1.Inputs[0].PrevOut)

	// Second spend of the same UTXO should fail.
	spend2 := &Transaction{
		Version: 2,
		ChainID: ChainIDNous,
		Inputs: []TxIn{
			{
				PrevOut:  OutPoint{TxID: cb.TxID(), Index: 0},
				Sequence: 0xFFFFFFFF,
			},
		},
		Outputs: []TxOut{
			{Amount: 50_0000_0000, PkScript: CreateP2PKHLockScript(pubKeyHash)},
		},
	}
	sigHash2 := spend2.SigHash(0, subscript)
	sig2, _ := crypto.Sign(priv, sigHash2)
	spend2.Inputs[0].SignatureScript = CreateP2PKHUnlockScript(sig2.Bytes(), pub.SerializeCompressed())

	err = ValidateTransaction(spend2, utxos, 100)
	if err == nil {
		t.Fatal("double-spend should be rejected")
	}
}

// ============================================================
// Transaction validation: insufficient funds rejected
// ============================================================

func TestValidateTransactionInsufficientFunds(t *testing.T) {
	priv, pub, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pubKeyHash := crypto.Hash160(pub.SerializeCompressed())

	cb := NewCoinbaseTx(0, 10_0000_0000, CreateP2PKHLockScript(pubKeyHash), ChainIDNous)
	utxos := NewUTXOSet()
	utxos.AddTransaction(cb, 0)

	// Try to spend 20 NOUS (more than available).
	spendTx := &Transaction{
		Version: 2,
		ChainID: ChainIDNous,
		Inputs: []TxIn{
			{
				PrevOut:  OutPoint{TxID: cb.TxID(), Index: 0},
				Sequence: 0xFFFFFFFF,
			},
		},
		Outputs: []TxOut{
			{Amount: 20_0000_0000, PkScript: CreateP2PKHLockScript(pubKeyHash)},
		},
	}

	subscript := cb.Outputs[0].PkScript
	sigHash := spendTx.SigHash(0, subscript)
	sig, _ := crypto.Sign(priv, sigHash)
	spendTx.Inputs[0].SignatureScript = CreateP2PKHUnlockScript(sig.Bytes(), pub.SerializeCompressed())

	err = ValidateTransaction(spendTx, utxos, 100)
	if err == nil {
		t.Fatal("insufficient funds should be rejected")
	}
}

// ============================================================
// Coinbase validation
// ============================================================

func TestValidateCoinbase(t *testing.T) {
	pubKeyHash := make([]byte, 20)
	cb := NewCoinbaseTx(0, 50_0000_0000, CreateP2PKHLockScript(pubKeyHash), ChainIDNous)

	err := ValidateTransaction(cb, NewUTXOSet(), 0)
	if err != nil {
		t.Fatalf("valid coinbase should pass: %v", err)
	}
}

// ============================================================
// Transaction validation: duplicate input within single tx rejected
// ============================================================

func TestValidateTransactionDuplicateInput(t *testing.T) {
	priv, pub, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pubKeyHash := crypto.Hash160(pub.SerializeCompressed())

	cb := NewCoinbaseTx(0, 10_0000_0000, CreateP2PKHLockScript(pubKeyHash), ChainIDNous)
	utxos := NewUTXOSet()
	utxos.AddTransaction(cb, 0)

	// Build a transaction that references the same UTXO twice.
	dupTx := &Transaction{
		Version: 2,
		ChainID: ChainIDNous,
		Inputs: []TxIn{
			{PrevOut: OutPoint{TxID: cb.TxID(), Index: 0}, Sequence: 0xFFFFFFFF},
			{PrevOut: OutPoint{TxID: cb.TxID(), Index: 0}, Sequence: 0xFFFFFFFF},
		},
		Outputs: []TxOut{
			{Amount: 19_0000_0000, PkScript: CreateP2PKHLockScript(pubKeyHash)},
		},
	}

	subscript := cb.Outputs[0].PkScript
	for i := 0; i < 2; i++ {
		sigHash := dupTx.SigHash(i, subscript)
		sig, _ := crypto.Sign(priv, sigHash)
		dupTx.Inputs[i].SignatureScript = CreateP2PKHUnlockScript(sig.Bytes(), pub.SerializeCompressed())
	}

	err = ValidateTransaction(dupTx, utxos, 100)
	if err == nil {
		t.Fatal("transaction with duplicate inputs should be rejected")
	}
}

// ============================================================
// Transaction validation: immature coinbase rejected
// ============================================================

func TestValidateTransactionImmatureCoinbase(t *testing.T) {
	priv, pub, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pubKeyHash := crypto.Hash160(pub.SerializeCompressed())

	// Coinbase created at height 50.
	cb := NewCoinbaseTx(50, 10_0000_0000, CreateP2PKHLockScript(pubKeyHash), ChainIDNous)
	utxos := NewUTXOSet()
	utxos.AddTransaction(cb, 50)

	spendTx := &Transaction{
		Version: 2,
		ChainID: ChainIDNous,
		Inputs:  []TxIn{{PrevOut: OutPoint{TxID: cb.TxID(), Index: 0}, Sequence: 0xFFFFFFFF}},
		Outputs: []TxOut{{Amount: 9_0000_0000, PkScript: CreateP2PKHLockScript(pubKeyHash)}},
	}
	subscript := cb.Outputs[0].PkScript
	sigHash := spendTx.SigHash(0, subscript)
	sig, _ := crypto.Sign(priv, sigHash)
	spendTx.Inputs[0].SignatureScript = CreateP2PKHUnlockScript(sig.Bytes(), pub.SerializeCompressed())

	// At height 51, should fail.
	err = ValidateTransaction(spendTx, utxos, 51)
	if err == nil {
		t.Fatal("spending immature coinbase should be rejected")
	}

	// At height 149, should still fail.
	err = ValidateTransaction(spendTx, utxos, 149)
	if err == nil {
		t.Fatal("spending coinbase at 99 confirmations should be rejected")
	}

	// At height 150, should pass.
	err = ValidateTransaction(spendTx, utxos, 150)
	if err != nil {
		t.Fatalf("spending mature coinbase should pass: %v", err)
	}
}

// ============================================================
// Overflow: output value exceeding MaxAmount rejected
// ============================================================

func TestValidateTransactionOverflowOutputValue(t *testing.T) {
	_, pub, _ := crypto.GenerateKeyPair()
	pubKeyHash := crypto.Hash160(pub.SerializeCompressed())

	utxos := NewUTXOSet()
	fakeOp := OutPoint{TxID: crypto.Sha256([]byte("fake")), Index: 0}
	utxos.Add(fakeOp, TxOut{Amount: 10_0000_0000, PkScript: CreateP2PKHLockScript(pubKeyHash)}, 0, false)

	spendTx := &Transaction{
		Version: 2,
		ChainID: ChainIDNous,
		Inputs:  []TxIn{{PrevOut: fakeOp, Sequence: 0xFFFFFFFF}},
		Outputs: []TxOut{{Amount: MaxAmount + 1, PkScript: CreateP2PKHLockScript(pubKeyHash)}},
	}

	err := ValidateTransaction(spendTx, utxos, 100)
	if err == nil {
		t.Fatal("output value exceeding MaxAmount should be rejected")
	}
}

// ============================================================
// Deserialization safety: too-short data
// ============================================================

func TestDeserializeTooShort(t *testing.T) {
	// Empty data.
	_, err := Deserialize(nil)
	if err == nil {
		t.Fatal("nil data should fail")
	}
	// Less than minimum (18 bytes).
	_, err = Deserialize([]byte{0x01, 0x00, 0x00, 0x00})
	if err == nil {
		t.Fatal("4-byte data should fail")
	}
}

// ============================================================
// Deserialization safety: huge input count
// ============================================================

func TestDeserializeHugeInputCount(t *testing.T) {
	var buf bytes.Buffer
	buf.Write([]byte{0x01, 0x00, 0x00, 0x00}) // version = 1
	buf.Write([]byte{0x4E, 0x4F, 0x55, 0x53}) // ChainID = NOUS
	// VarInt 0xFF followed by a huge uint64 for input count.
	buf.WriteByte(0xFF)
	buf.Write([]byte{0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00}) // 65536 > MaxTxInputs
	// Pad.
	for buf.Len() < 30 {
		buf.WriteByte(0x00)
	}

	_, err := Deserialize(buf.Bytes())
	if err == nil {
		t.Fatal("huge input count should fail")
	}
}

// ============================================================
// Dust output: value below DustLimit rejected via ValidateTransaction
// ============================================================

func TestValidateTransactionDustOutput(t *testing.T) {
	priv, pub, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	_ = priv
	pubKeyHash := crypto.Hash160(pub.SerializeCompressed())

	utxos := NewUTXOSet()
	fakeOp := OutPoint{TxID: crypto.Sha256([]byte("dust-test2")), Index: 0}
	utxos.Add(fakeOp, TxOut{Amount: 10_0000_0000, PkScript: CreateP2PKHLockScript(pubKeyHash)}, 0, false)

	dustTx := &Transaction{
		Version: 2,
		ChainID: ChainIDNous,
		Inputs:  []TxIn{{PrevOut: fakeOp, Sequence: 0xFFFFFFFF}},
		Outputs: []TxOut{{Amount: DustLimit - 1, PkScript: CreateP2PKHLockScript(pubKeyHash)}},
	}

	err = ValidateTransaction(dustTx, utxos, 100)
	if err == nil {
		t.Fatal("output below dust limit should be rejected")
	}

	// Output at exactly DustLimit should pass the dust check.
	okTx := &Transaction{
		Version: 2,
		ChainID: ChainIDNous,
		Inputs:  []TxIn{{PrevOut: fakeOp, Sequence: 0xFFFFFFFF}},
		Outputs: []TxOut{{Amount: DustLimit, PkScript: CreateP2PKHLockScript(pubKeyHash)}},
	}

	err = ValidateTransaction(okTx, utxos, 100)
	// May fail on script verification (unsigned), but should NOT fail on dust.
	if err != nil && err.Error() != "" {
		if bytes.Contains([]byte(err.Error()), []byte("dust")) {
			t.Fatalf("output at dust limit should not be rejected for dust: %v", err)
		}
	}
}

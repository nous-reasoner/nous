package tx

import (
	"bytes"
	"testing"

	"github.com/nous-chain/nous/crypto"
)

// ============================================================
// Serialize / Deserialize round-trip
// ============================================================

func TestSerializeDeserializeRoundTrip(t *testing.T) {
	original := &Transaction{
		Version: 1,
		Inputs: []TxInput{
			{
				PrevOut:   OutPoint{TxID: crypto.Sha256([]byte("prev")), Index: 0},
				ScriptSig: []byte{0x01, 0x02, 0x03},
				Sequence:  0xFFFFFFFF,
			},
			{
				PrevOut:   OutPoint{TxID: crypto.Sha256([]byte("prev2")), Index: 1},
				ScriptSig: []byte{0x04, 0x05},
				Sequence:  0xFFFFFFFE,
			},
		},
		Outputs: []TxOutput{
			{Value: 50_0000_0000, ScriptPubKey: []byte{0x76, 0xa9}},
			{Value: 25_0000_0000, ScriptPubKey: []byte{0x76, 0xa9, 0x14}},
		},
		LockTime: 100,
	}

	data := original.Serialize()
	decoded, err := Deserialize(data)
	if err != nil {
		t.Fatalf("Deserialize failed: %v", err)
	}

	if original.Version != decoded.Version {
		t.Fatalf("Version: want %d, got %d", original.Version, decoded.Version)
	}
	if original.LockTime != decoded.LockTime {
		t.Fatalf("LockTime: want %d, got %d", original.LockTime, decoded.LockTime)
	}
	if len(original.Inputs) != len(decoded.Inputs) {
		t.Fatalf("input count: want %d, got %d", len(original.Inputs), len(decoded.Inputs))
	}
	for i := range original.Inputs {
		if original.Inputs[i].PrevOut != decoded.Inputs[i].PrevOut {
			t.Fatalf("input %d PrevOut mismatch", i)
		}
		if !bytes.Equal(original.Inputs[i].ScriptSig, decoded.Inputs[i].ScriptSig) {
			t.Fatalf("input %d ScriptSig mismatch", i)
		}
		if original.Inputs[i].Sequence != decoded.Inputs[i].Sequence {
			t.Fatalf("input %d Sequence mismatch", i)
		}
	}
	if len(original.Outputs) != len(decoded.Outputs) {
		t.Fatalf("output count: want %d, got %d", len(original.Outputs), len(decoded.Outputs))
	}
	for i := range original.Outputs {
		if original.Outputs[i].Value != decoded.Outputs[i].Value {
			t.Fatalf("output %d Value: want %d, got %d", i, original.Outputs[i].Value, decoded.Outputs[i].Value)
		}
		if !bytes.Equal(original.Outputs[i].ScriptPubKey, decoded.Outputs[i].ScriptPubKey) {
			t.Fatalf("output %d ScriptPubKey mismatch", i)
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
	tx := &Transaction{
		Version: 1,
		Inputs: []TxInput{
			{
				PrevOut:   OutPoint{TxID: crypto.Hash{}, Index: 0xFFFFFFFF},
				ScriptSig: []byte{0x04, 0x01, 0x00, 0x00, 0x00},
				Sequence:  0xFFFFFFFF,
			},
		},
		Outputs: []TxOutput{
			{Value: 50_0000_0000, ScriptPubKey: []byte{0x76}},
		},
		LockTime: 0,
	}

	id1 := tx.TxID()
	id2 := tx.TxID()
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
// NewCoinbase
// ============================================================

func TestNewCoinbaseStructure(t *testing.T) {
	pubKeyHash := make([]byte, 20)
	for i := range pubKeyHash {
		pubKeyHash[i] = byte(i)
	}

	cb := NewCoinbase(42, 50_0000_0000, pubKeyHash, "test message")

	if !cb.IsCoinbase() {
		t.Fatal("NewCoinbase should produce a coinbase transaction")
	}

	if len(cb.Outputs) != 1 {
		t.Fatalf("coinbase should have 1 output, got %d", len(cb.Outputs))
	}

	if cb.Outputs[0].Value != 50_0000_0000 {
		t.Fatalf("coinbase reward: want 5000000000, got %d", cb.Outputs[0].Value)
	}

	// Output should be a valid P2PKH script.
	extracted := ExtractPubKeyHashFromP2PKH(cb.Outputs[0].ScriptPubKey)
	if extracted == nil {
		t.Fatal("coinbase output should have a P2PKH script")
	}
	if !bytes.Equal(extracted, pubKeyHash) {
		t.Fatal("coinbase P2PKH hash mismatch")
	}

	// ScriptSig should contain the height.
	if len(cb.Inputs[0].ScriptSig) == 0 {
		t.Fatal("coinbase ScriptSig should not be empty")
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

	// Expected: OP_DUP OP_HASH160 0x14 <20 bytes> OP_EQUALVERIFY OP_CHECKSIG
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

	// Expected: <len(sig)> <sig> <len(pubKey)> <pubKey>
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
	fundingTx := NewCoinbase(0, 50_0000_0000, pubKeyHash, "")

	// Create a spending transaction.
	spendTx := &Transaction{
		Version: 1,
		Inputs: []TxInput{
			{
				PrevOut:  OutPoint{TxID: fundingTx.TxID(), Index: 0},
				Sequence: 0xFFFFFFFF,
			},
		},
		Outputs: []TxOutput{
			{Value: 50_0000_0000, ScriptPubKey: CreateP2PKHLockScript(pubKeyHash)},
		},
		LockTime: 0,
	}

	// Sign the spending transaction.
	subscript := fundingTx.Outputs[0].ScriptPubKey
	sigHash := spendTx.SigHash(0, subscript)
	sig, err := crypto.Sign(priv, sigHash)
	if err != nil {
		t.Fatal(err)
	}

	// Set the unlock script.
	spendTx.Inputs[0].ScriptSig = CreateP2PKHUnlockScript(sig.Bytes(), pub.SerializeCompressed())

	// Execute the script.
	ok := ExecuteScript(spendTx.Inputs[0].ScriptSig, fundingTx.Outputs[0].ScriptPubKey, spendTx, 0)
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

	fundingTx := NewCoinbase(0, 50_0000_0000, pubKeyHash, "")

	spendTx := &Transaction{
		Version: 1,
		Inputs: []TxInput{
			{
				PrevOut:  OutPoint{TxID: fundingTx.TxID(), Index: 0},
				Sequence: 0xFFFFFFFF,
			},
		},
		Outputs: []TxOutput{
			{Value: 50_0000_0000, ScriptPubKey: CreateP2PKHLockScript(pubKeyHash)},
		},
		LockTime: 0,
	}

	// Sign with a wrong hash (not the actual sighash).
	wrongHash := crypto.Sha256([]byte("wrong data"))
	sig, err := crypto.Sign(priv, wrongHash)
	if err != nil {
		t.Fatal(err)
	}

	spendTx.Inputs[0].ScriptSig = CreateP2PKHUnlockScript(sig.Bytes(), pub.SerializeCompressed())

	ok := ExecuteScript(spendTx.Inputs[0].ScriptSig, fundingTx.Outputs[0].ScriptPubKey, spendTx, 0)
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
	output := TxOutput{Value: 100, ScriptPubKey: CreateP2PKHLockScript(pubKeyHash)}

	// Add
	utxos.Add(op, output, 1, false)
	got := utxos.Get(op)
	if got == nil {
		t.Fatal("UTXO should exist after Add")
	}
	if got.Output.Value != 100 {
		t.Fatalf("UTXO value: want 100, got %d", got.Output.Value)
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

	cb := NewCoinbase(0, 50_0000_0000, pubKeyHash, "")
	utxos.AddTransaction(cb, 0)

	txID := cb.TxID()
	got := utxos.Get(OutPoint{TxID: txID, Index: 0})
	if got == nil {
		t.Fatal("UTXO should exist after AddTransaction")
	}
	if got.Output.Value != 50_0000_0000 {
		t.Fatalf("UTXO value: want 5000000000, got %d", got.Output.Value)
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
	cb := NewCoinbase(0, 50_0000_0000, pubKeyHash, "")
	utxos := NewUTXOSet()
	utxos.AddTransaction(cb, 0)

	// Create spending transaction.
	spendTx := &Transaction{
		Version: 1,
		Inputs: []TxInput{
			{
				PrevOut:  OutPoint{TxID: cb.TxID(), Index: 0},
				Sequence: 0xFFFFFFFF,
			},
		},
		Outputs: []TxOutput{
			{Value: 49_0000_0000, ScriptPubKey: CreateP2PKHLockScript(pubKeyHash)},
		},
		LockTime: 0,
	}

	// Sign it.
	subscript := cb.Outputs[0].ScriptPubKey
	sigHash := spendTx.SigHash(0, subscript)
	sig, err := crypto.Sign(priv, sigHash)
	if err != nil {
		t.Fatal(err)
	}
	spendTx.Inputs[0].ScriptSig = CreateP2PKHUnlockScript(sig.Bytes(), pub.SerializeCompressed())

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

	cb := NewCoinbase(0, 50_0000_0000, pubKeyHash, "")
	utxos := NewUTXOSet()
	utxos.AddTransaction(cb, 0)

	// First spend.
	spend1 := &Transaction{
		Version: 1,
		Inputs: []TxInput{
			{
				PrevOut:  OutPoint{TxID: cb.TxID(), Index: 0},
				Sequence: 0xFFFFFFFF,
			},
		},
		Outputs: []TxOutput{
			{Value: 50_0000_0000, ScriptPubKey: CreateP2PKHLockScript(pubKeyHash)},
		},
		LockTime: 0,
	}
	subscript := cb.Outputs[0].ScriptPubKey
	sigHash := spend1.SigHash(0, subscript)
	sig, _ := crypto.Sign(priv, sigHash)
	spend1.Inputs[0].ScriptSig = CreateP2PKHUnlockScript(sig.Bytes(), pub.SerializeCompressed())

	// Validate and apply first spend.
	if err := ValidateTransaction(spend1, utxos, 100); err != nil {
		t.Fatalf("first spend should be valid: %v", err)
	}
	utxos.Spend(spend1.Inputs[0].PrevOut)

	// Second spend of the same UTXO should fail.
	spend2 := &Transaction{
		Version: 1,
		Inputs: []TxInput{
			{
				PrevOut:  OutPoint{TxID: cb.TxID(), Index: 0},
				Sequence: 0xFFFFFFFF,
			},
		},
		Outputs: []TxOutput{
			{Value: 50_0000_0000, ScriptPubKey: CreateP2PKHLockScript(pubKeyHash)},
		},
		LockTime: 0,
	}
	sigHash2 := spend2.SigHash(0, subscript)
	sig2, _ := crypto.Sign(priv, sigHash2)
	spend2.Inputs[0].ScriptSig = CreateP2PKHUnlockScript(sig2.Bytes(), pub.SerializeCompressed())

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

	// Coinbase with 10 NOUS.
	cb := NewCoinbase(0, 10_0000_0000, pubKeyHash, "")
	utxos := NewUTXOSet()
	utxos.AddTransaction(cb, 0)

	// Try to spend 20 NOUS (more than available).
	spendTx := &Transaction{
		Version: 1,
		Inputs: []TxInput{
			{
				PrevOut:  OutPoint{TxID: cb.TxID(), Index: 0},
				Sequence: 0xFFFFFFFF,
			},
		},
		Outputs: []TxOutput{
			{Value: 20_0000_0000, ScriptPubKey: CreateP2PKHLockScript(pubKeyHash)},
		},
		LockTime: 0,
	}

	subscript := cb.Outputs[0].ScriptPubKey
	sigHash := spendTx.SigHash(0, subscript)
	sig, _ := crypto.Sign(priv, sigHash)
	spendTx.Inputs[0].ScriptSig = CreateP2PKHUnlockScript(sig.Bytes(), pub.SerializeCompressed())

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
	cb := NewCoinbase(0, 50_0000_0000, pubKeyHash, "")

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

	cb := NewCoinbase(0, 10_0000_0000, pubKeyHash, "")
	utxos := NewUTXOSet()
	utxos.AddTransaction(cb, 0)

	// Build a transaction that references the same UTXO twice.
	dupTx := &Transaction{
		Version: 1,
		Inputs: []TxInput{
			{PrevOut: OutPoint{TxID: cb.TxID(), Index: 0}, Sequence: 0xFFFFFFFF},
			{PrevOut: OutPoint{TxID: cb.TxID(), Index: 0}, Sequence: 0xFFFFFFFF},
		},
		Outputs: []TxOutput{
			{Value: 19_0000_0000, ScriptPubKey: CreateP2PKHLockScript(pubKeyHash)},
		},
		LockTime: 0,
	}

	// Sign both inputs (same UTXO, different SigHash because inputIndex differs).
	subscript := cb.Outputs[0].ScriptPubKey
	for i := 0; i < 2; i++ {
		sigHash := dupTx.SigHash(i, subscript)
		sig, _ := crypto.Sign(priv, sigHash)
		dupTx.Inputs[i].ScriptSig = CreateP2PKHUnlockScript(sig.Bytes(), pub.SerializeCompressed())
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
	cb := NewCoinbase(50, 10_0000_0000, pubKeyHash, "")
	utxos := NewUTXOSet()
	utxos.AddTransaction(cb, 50)

	spendTx := &Transaction{
		Version: 1,
		Inputs:  []TxInput{{PrevOut: OutPoint{TxID: cb.TxID(), Index: 0}, Sequence: 0xFFFFFFFF}},
		Outputs: []TxOutput{{Value: 9_0000_0000, ScriptPubKey: CreateP2PKHLockScript(pubKeyHash)}},
	}
	subscript := cb.Outputs[0].ScriptPubKey
	sigHash := spendTx.SigHash(0, subscript)
	sig, _ := crypto.Sign(priv, sigHash)
	spendTx.Inputs[0].ScriptSig = CreateP2PKHUnlockScript(sig.Bytes(), pub.SerializeCompressed())

	// At height 51 (only 1 confirmation), should fail.
	err = ValidateTransaction(spendTx, utxos, 51)
	if err == nil {
		t.Fatal("spending immature coinbase should be rejected")
	}

	// At height 149 (99 confirmations), should still fail.
	err = ValidateTransaction(spendTx, utxos, 149)
	if err == nil {
		t.Fatal("spending coinbase at 99 confirmations should be rejected")
	}

	// At height 150 (100 confirmations), should pass.
	err = ValidateTransaction(spendTx, utxos, 150)
	if err != nil {
		t.Fatalf("spending mature coinbase should pass: %v", err)
	}
}

// ============================================================
// Overflow: output value exceeding MaxMoney rejected
// ============================================================

func TestValidateTransactionOverflowOutputValue(t *testing.T) {
	_, pub, _ := crypto.GenerateKeyPair()
	pubKeyHash := crypto.Hash160(pub.SerializeCompressed())

	// Create a non-coinbase UTXO with a sane value.
	utxos := NewUTXOSet()
	fakeOp := OutPoint{TxID: crypto.Sha256([]byte("fake")), Index: 0}
	utxos.Add(fakeOp, TxOutput{Value: 10_0000_0000, ScriptPubKey: CreateP2PKHLockScript(pubKeyHash)}, 0, false)

	// Transaction with output exceeding MaxMoney.
	spendTx := &Transaction{
		Version: 1,
		Inputs:  []TxInput{{PrevOut: fakeOp, Sequence: 0xFFFFFFFF}},
		Outputs: []TxOutput{{Value: MaxMoney + 1, ScriptPubKey: CreateP2PKHLockScript(pubKeyHash)}},
	}

	err := ValidateTransaction(spendTx, utxos, 100)
	if err == nil {
		t.Fatal("output value exceeding MaxMoney should be rejected")
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
	// Less than minimum (10 bytes).
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
	// VarInt 0xFF followed by a huge uint64 for input count.
	buf.WriteByte(0xFF)
	buf.Write([]byte{0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00}) // 65536 > MaxTxInputs=10000? No, 65536 > 10000. Yes.
	// Pad to reach minimum 10 bytes.
	for buf.Len() < 20 {
		buf.WriteByte(0x00)
	}

	_, err := Deserialize(buf.Bytes())
	if err == nil {
		t.Fatal("huge input count should fail")
	}
}

// ============================================================
// Dust output: value below DustLimit rejected
// ============================================================

func TestValidateTransactionDustOutput(t *testing.T) {
	priv, pub, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	_ = priv
	pubKeyHash := crypto.Hash160(pub.SerializeCompressed())

	utxos := NewUTXOSet()
	fakeOp := OutPoint{TxID: crypto.Sha256([]byte("dust-test")), Index: 0}
	utxos.Add(fakeOp, TxOutput{Value: 10_0000_0000, ScriptPubKey: CreateP2PKHLockScript(pubKeyHash)}, 0, false)

	// Transaction with output value = DustLimit - 1.
	dustTx := &Transaction{
		Version: 1,
		Inputs:  []TxInput{{PrevOut: fakeOp, Sequence: 0xFFFFFFFF}},
		Outputs: []TxOutput{{Value: DustLimit - 1, ScriptPubKey: CreateP2PKHLockScript(pubKeyHash)}},
	}

	err = ValidateTransaction(dustTx, utxos, 100)
	if err == nil {
		t.Fatal("output below dust limit should be rejected")
	}

	// Transaction with output value = DustLimit should pass (at least the dust check).
	okTx := &Transaction{
		Version: 1,
		Inputs:  []TxInput{{PrevOut: fakeOp, Sequence: 0xFFFFFFFF}},
		Outputs: []TxOutput{{Value: DustLimit, ScriptPubKey: CreateP2PKHLockScript(pubKeyHash)}},
	}

	err = ValidateTransaction(okTx, utxos, 100)
	// This may fail on script verification (unsigned), but should NOT fail on dust.
	if err != nil && err.Error() != "" {
		// Check it's not a dust error.
		if bytes.Contains([]byte(err.Error()), []byte("dust")) {
			t.Fatalf("output at dust limit should not be rejected for dust: %v", err)
		}
	}
}

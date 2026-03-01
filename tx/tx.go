// Package tx defines transactions and the UTXO model for NOUS.
package tx

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/nous-chain/nous/crypto"
)

// ChainIDNous is the chain identifier for the NOUS mainnet.
var ChainIDNous = [4]byte{0x4E, 0x4F, 0x55, 0x53} // "NOUS"

// OutPoint references a specific output of a previous transaction.
type OutPoint struct {
	TxID  crypto.Hash
	Index uint32
}

// Deserialization limits to prevent OOM from malicious input.
const (
	MaxTxInputs   = 10_000
	MaxTxOutputs  = 10_000
	MaxScriptSize = 10_000
)

// TxIn represents a transaction input (spending a previous UTXO).
type TxIn struct {
	PrevOut         OutPoint
	SignatureScript []byte
	Sequence        uint32
}

// TxOut represents a transaction output (creating a new UTXO).
type TxOut struct {
	Amount        int64  // value in nou (1 NOUS = 1e8 nou)
	ScriptVersion uint16
	PkScript      []byte
}

// Transaction represents a NOUS transaction.
type Transaction struct {
	Version      uint32
	ChainID      [4]byte
	Inputs       []TxIn
	Outputs      []TxOut
	LockTime     uint32
	ExpiryHeight uint32
}

// Serialize encodes the transaction into a deterministic byte slice.
// Wire format:
//
//	[4]Version [4]ChainID [varint]#in
//	  per in: [32]TxID [4]Index [varbytes]SignatureScript [4]Sequence
//	[varint]#out
//	  per out: [8]Amount [2]ScriptVersion [varbytes]PkScript
//	[4]LockTime [4]ExpiryHeight
func (t *Transaction) Serialize() []byte {
	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, t.Version)
	buf.Write(t.ChainID[:])
	writeVarInt(&buf, uint64(len(t.Inputs)))
	for _, in := range t.Inputs {
		buf.Write(in.PrevOut.TxID[:])
		binary.Write(&buf, binary.LittleEndian, in.PrevOut.Index)
		writeVarBytes(&buf, in.SignatureScript)
		binary.Write(&buf, binary.LittleEndian, in.Sequence)
	}
	writeVarInt(&buf, uint64(len(t.Outputs)))
	for _, out := range t.Outputs {
		binary.Write(&buf, binary.LittleEndian, out.Amount)
		binary.Write(&buf, binary.LittleEndian, out.ScriptVersion)
		writeVarBytes(&buf, out.PkScript)
	}
	binary.Write(&buf, binary.LittleEndian, t.LockTime)
	binary.Write(&buf, binary.LittleEndian, t.ExpiryHeight)
	return buf.Bytes()
}

// SerializeNoWitness encodes the transaction with all SignatureScript fields
// replaced by empty byte slices. This is used for computing the malleability-
// resistant TxID.
func (t *Transaction) SerializeNoWitness() []byte {
	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, t.Version)
	buf.Write(t.ChainID[:])
	writeVarInt(&buf, uint64(len(t.Inputs)))
	for _, in := range t.Inputs {
		buf.Write(in.PrevOut.TxID[:])
		binary.Write(&buf, binary.LittleEndian, in.PrevOut.Index)
		writeVarBytes(&buf, nil) // empty SignatureScript
		binary.Write(&buf, binary.LittleEndian, in.Sequence)
	}
	writeVarInt(&buf, uint64(len(t.Outputs)))
	for _, out := range t.Outputs {
		binary.Write(&buf, binary.LittleEndian, out.Amount)
		binary.Write(&buf, binary.LittleEndian, out.ScriptVersion)
		writeVarBytes(&buf, out.PkScript)
	}
	binary.Write(&buf, binary.LittleEndian, t.LockTime)
	binary.Write(&buf, binary.LittleEndian, t.ExpiryHeight)
	return buf.Bytes()
}

// TxID computes the malleability-resistant transaction identifier.
// For non-coinbase transactions, it is the double-SHA-256 of the serialized
// transaction with all SignatureScript fields stripped (SerializeNoWitness).
// For coinbase transactions, the full serialization is used since coinbase
// inputs don't have malleatable signatures and the height is encoded in
// the SignatureScript.
func (t *Transaction) TxID() crypto.Hash {
	if t.IsCoinbase() {
		return crypto.DoubleSha256(t.Serialize())
	}
	return crypto.DoubleSha256(t.SerializeNoWitness())
}

// TxHash computes the full transaction hash including signatures.
func (t *Transaction) TxHash() crypto.Hash {
	return crypto.DoubleSha256(t.Serialize())
}

// IsCoinbase returns true if this is a coinbase (block reward) transaction.
func (t *Transaction) IsCoinbase() bool {
	return len(t.Inputs) == 1 &&
		t.Inputs[0].PrevOut.TxID == crypto.Hash{} &&
		t.Inputs[0].PrevOut.Index == 0xFFFFFFFF
}

// Deserialize decodes a transaction from its serialized byte slice.
func Deserialize(data []byte) (*Transaction, error) {
	if len(data) < 18 {
		return nil, fmt.Errorf("tx: data too short (%d bytes, min 18)", len(data))
	}
	r := bytes.NewReader(data)
	t := &Transaction{}

	if err := binary.Read(r, binary.LittleEndian, &t.Version); err != nil {
		return nil, fmt.Errorf("tx: read version: %w", err)
	}
	if _, err := io.ReadFull(r, t.ChainID[:]); err != nil {
		return nil, fmt.Errorf("tx: read chainID: %w", err)
	}

	inputCount, err := readVarInt(r)
	if err != nil {
		return nil, fmt.Errorf("tx: read input count: %w", err)
	}
	if inputCount > MaxTxInputs {
		return nil, fmt.Errorf("tx: input count %d exceeds max %d", inputCount, MaxTxInputs)
	}
	t.Inputs = make([]TxIn, inputCount)
	for i := uint64(0); i < inputCount; i++ {
		if _, err := io.ReadFull(r, t.Inputs[i].PrevOut.TxID[:]); err != nil {
			return nil, fmt.Errorf("tx: read input %d txid: %w", i, err)
		}
		if err := binary.Read(r, binary.LittleEndian, &t.Inputs[i].PrevOut.Index); err != nil {
			return nil, fmt.Errorf("tx: read input %d index: %w", i, err)
		}
		scriptLen, err := readVarInt(r)
		if err != nil {
			return nil, fmt.Errorf("tx: read input %d script len: %w", i, err)
		}
		if scriptLen > MaxScriptSize {
			return nil, fmt.Errorf("tx: input %d script size %d exceeds max %d", i, scriptLen, MaxScriptSize)
		}
		t.Inputs[i].SignatureScript = make([]byte, scriptLen)
		if _, err := io.ReadFull(r, t.Inputs[i].SignatureScript); err != nil {
			return nil, fmt.Errorf("tx: read input %d script: %w", i, err)
		}
		if err := binary.Read(r, binary.LittleEndian, &t.Inputs[i].Sequence); err != nil {
			return nil, fmt.Errorf("tx: read input %d sequence: %w", i, err)
		}
	}

	outputCount, err := readVarInt(r)
	if err != nil {
		return nil, fmt.Errorf("tx: read output count: %w", err)
	}
	if outputCount > MaxTxOutputs {
		return nil, fmt.Errorf("tx: output count %d exceeds max %d", outputCount, MaxTxOutputs)
	}
	t.Outputs = make([]TxOut, outputCount)
	for i := uint64(0); i < outputCount; i++ {
		if err := binary.Read(r, binary.LittleEndian, &t.Outputs[i].Amount); err != nil {
			return nil, fmt.Errorf("tx: read output %d amount: %w", i, err)
		}
		if err := binary.Read(r, binary.LittleEndian, &t.Outputs[i].ScriptVersion); err != nil {
			return nil, fmt.Errorf("tx: read output %d script version: %w", i, err)
		}
		scriptLen, err := readVarInt(r)
		if err != nil {
			return nil, fmt.Errorf("tx: read output %d script len: %w", i, err)
		}
		if scriptLen > MaxScriptSize {
			return nil, fmt.Errorf("tx: output %d script size %d exceeds max %d", i, scriptLen, MaxScriptSize)
		}
		t.Outputs[i].PkScript = make([]byte, scriptLen)
		if _, err := io.ReadFull(r, t.Outputs[i].PkScript); err != nil {
			return nil, fmt.Errorf("tx: read output %d script: %w", i, err)
		}
	}

	if err := binary.Read(r, binary.LittleEndian, &t.LockTime); err != nil {
		return nil, fmt.Errorf("tx: read locktime: %w", err)
	}
	if err := binary.Read(r, binary.LittleEndian, &t.ExpiryHeight); err != nil {
		return nil, fmt.Errorf("tx: read expiry height: %w", err)
	}

	return t, nil
}

// SigHash computes the SIGHASH_ALL hash for signing a specific input.
// It clears all input SignatureScripts, sets the given input's SignatureScript
// to subscript, serializes the modified transaction, and returns the
// double-SHA-256 hash.
func (t *Transaction) SigHash(inputIndex int, subscript []byte) crypto.Hash {
	txCopy := &Transaction{
		Version:      t.Version,
		ChainID:      t.ChainID,
		Inputs:       make([]TxIn, len(t.Inputs)),
		Outputs:      make([]TxOut, len(t.Outputs)),
		LockTime:     t.LockTime,
		ExpiryHeight: t.ExpiryHeight,
	}
	for i, in := range t.Inputs {
		txCopy.Inputs[i] = TxIn{
			PrevOut:  in.PrevOut,
			Sequence: in.Sequence,
		}
		if i == inputIndex {
			txCopy.Inputs[i].SignatureScript = subscript
		}
	}
	copy(txCopy.Outputs, t.Outputs)

	// Serialize and append SIGHASH_ALL type (uint32 LE = 1).
	data := txCopy.Serialize()
	var buf bytes.Buffer
	buf.Write(data)
	binary.Write(&buf, binary.LittleEndian, uint32(1)) // SIGHASH_ALL
	return crypto.DoubleSha256(buf.Bytes())
}

// --- encoding helpers ---

func writeVarInt(buf *bytes.Buffer, v uint64) {
	switch {
	case v < 0xFD:
		buf.WriteByte(byte(v))
	case v <= 0xFFFF:
		buf.WriteByte(0xFD)
		binary.Write(buf, binary.LittleEndian, uint16(v))
	case v <= 0xFFFFFFFF:
		buf.WriteByte(0xFE)
		binary.Write(buf, binary.LittleEndian, uint32(v))
	default:
		buf.WriteByte(0xFF)
		binary.Write(buf, binary.LittleEndian, v)
	}
}

func readVarInt(r *bytes.Reader) (uint64, error) {
	b, err := r.ReadByte()
	if err != nil {
		return 0, err
	}
	switch b {
	case 0xFD:
		var v uint16
		if err := binary.Read(r, binary.LittleEndian, &v); err != nil {
			return 0, err
		}
		return uint64(v), nil
	case 0xFE:
		var v uint32
		if err := binary.Read(r, binary.LittleEndian, &v); err != nil {
			return 0, err
		}
		return uint64(v), nil
	case 0xFF:
		var v uint64
		if err := binary.Read(r, binary.LittleEndian, &v); err != nil {
			return 0, err
		}
		return v, nil
	default:
		return uint64(b), nil
	}
}

func writeVarBytes(buf *bytes.Buffer, data []byte) {
	writeVarInt(buf, uint64(len(data)))
	buf.Write(data)
}

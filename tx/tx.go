// Package tx defines transactions and the UTXO model for NOUS.
package tx

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/nous-chain/nous/crypto"
)

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

// TxInput represents a transaction input (spending a previous UTXO).
type TxInput struct {
	PrevOut   OutPoint
	ScriptSig []byte
	Sequence  uint32
}

// TxOutput represents a transaction output (creating a new UTXO).
type TxOutput struct {
	Value        int64 // amount in nou (1 NOUS = 1e8 nou)
	ScriptPubKey []byte
}

// Transaction represents a NOUS transaction.
type Transaction struct {
	Version  uint32
	Inputs   []TxInput
	Outputs  []TxOutput
	LockTime uint32
}

// Serialize encodes the transaction into a deterministic byte slice.
func (t *Transaction) Serialize() []byte {
	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, t.Version)
	writeVarInt(&buf, uint64(len(t.Inputs)))
	for _, in := range t.Inputs {
		buf.Write(in.PrevOut.TxID[:])
		binary.Write(&buf, binary.LittleEndian, in.PrevOut.Index)
		writeVarBytes(&buf, in.ScriptSig)
		binary.Write(&buf, binary.LittleEndian, in.Sequence)
	}
	writeVarInt(&buf, uint64(len(t.Outputs)))
	for _, out := range t.Outputs {
		binary.Write(&buf, binary.LittleEndian, out.Value)
		writeVarBytes(&buf, out.ScriptPubKey)
	}
	binary.Write(&buf, binary.LittleEndian, t.LockTime)
	return buf.Bytes()
}

// Deserialize decodes a transaction from its serialized byte slice.
func Deserialize(data []byte) (*Transaction, error) {
	if len(data) < 10 {
		return nil, fmt.Errorf("tx: data too short (%d bytes, min 10)", len(data))
	}
	r := bytes.NewReader(data)
	t := &Transaction{}

	if err := binary.Read(r, binary.LittleEndian, &t.Version); err != nil {
		return nil, fmt.Errorf("tx: read version: %w", err)
	}

	inputCount, err := readVarInt(r)
	if err != nil {
		return nil, fmt.Errorf("tx: read input count: %w", err)
	}
	if inputCount > MaxTxInputs {
		return nil, fmt.Errorf("tx: input count %d exceeds max %d", inputCount, MaxTxInputs)
	}
	t.Inputs = make([]TxInput, inputCount)
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
		t.Inputs[i].ScriptSig = make([]byte, scriptLen)
		if _, err := io.ReadFull(r, t.Inputs[i].ScriptSig); err != nil {
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
	t.Outputs = make([]TxOutput, outputCount)
	for i := uint64(0); i < outputCount; i++ {
		if err := binary.Read(r, binary.LittleEndian, &t.Outputs[i].Value); err != nil {
			return nil, fmt.Errorf("tx: read output %d value: %w", i, err)
		}
		scriptLen, err := readVarInt(r)
		if err != nil {
			return nil, fmt.Errorf("tx: read output %d script len: %w", i, err)
		}
		if scriptLen > MaxScriptSize {
			return nil, fmt.Errorf("tx: output %d script size %d exceeds max %d", i, scriptLen, MaxScriptSize)
		}
		t.Outputs[i].ScriptPubKey = make([]byte, scriptLen)
		if _, err := io.ReadFull(r, t.Outputs[i].ScriptPubKey); err != nil {
			return nil, fmt.Errorf("tx: read output %d script: %w", i, err)
		}
	}

	if err := binary.Read(r, binary.LittleEndian, &t.LockTime); err != nil {
		return nil, fmt.Errorf("tx: read locktime: %w", err)
	}

	return t, nil
}

// TxID computes the double-SHA-256 hash of the serialized transaction.
func (t *Transaction) TxID() crypto.Hash {
	return crypto.DoubleSha256(t.Serialize())
}

// IsCoinbase returns true if this is a coinbase (block reward) transaction.
func (t *Transaction) IsCoinbase() bool {
	return len(t.Inputs) == 1 &&
		t.Inputs[0].PrevOut.TxID == crypto.Hash{} &&
		t.Inputs[0].PrevOut.Index == 0xFFFFFFFF
}

// NewCoinbase creates a coinbase transaction paying the block reward to the
// owner of the given public key hash. The scriptSig encodes the block height
// (BIP34 style) followed by an optional message.
func NewCoinbase(blockHeight uint32, reward int64, minerPubKeyHash []byte, message string) *Transaction {
	// Encode block height as varint-sized little-endian bytes in ScriptSig.
	var sig bytes.Buffer
	heightBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(heightBytes, blockHeight)
	// Trim trailing zero bytes for compact encoding, keep at least 1 byte.
	trimmed := heightBytes
	for len(trimmed) > 1 && trimmed[len(trimmed)-1] == 0 {
		trimmed = trimmed[:len(trimmed)-1]
	}
	sig.WriteByte(byte(len(trimmed)))
	sig.Write(trimmed)
	if len(message) > 0 {
		sig.Write([]byte(message))
	}

	return &Transaction{
		Version: 1,
		Inputs: []TxInput{
			{
				PrevOut:   OutPoint{TxID: crypto.Hash{}, Index: 0xFFFFFFFF},
				ScriptSig: sig.Bytes(),
				Sequence:  0xFFFFFFFF,
			},
		},
		Outputs: []TxOutput{
			{Value: reward, ScriptPubKey: CreateP2PKHLockScript(minerPubKeyHash)},
		},
		LockTime: 0,
	}
}

// SigHash computes the SIGHASH_ALL hash for signing a specific input.
// It clears all input scriptSigs, sets the given input's scriptSig to subscript,
// serializes the modified transaction, and returns the double-SHA-256 hash.
func (t *Transaction) SigHash(inputIndex int, subscript []byte) crypto.Hash {
	// Make a deep-enough copy: we need to modify scriptSigs.
	txCopy := &Transaction{
		Version:  t.Version,
		Inputs:   make([]TxInput, len(t.Inputs)),
		Outputs:  make([]TxOutput, len(t.Outputs)),
		LockTime: t.LockTime,
	}
	for i, in := range t.Inputs {
		txCopy.Inputs[i] = TxInput{
			PrevOut:  in.PrevOut,
			Sequence: in.Sequence,
		}
		if i == inputIndex {
			txCopy.Inputs[i].ScriptSig = subscript
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

package tx

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/nous-chain/nous/crypto"
)

// NewCoinbaseTx creates a coinbase transaction paying the block reward.
// The SignatureScript encodes the block height as compact little-endian bytes
// (BIP34 style): [len][height_le_bytes...].
//
// pkScript is the full locking script for the output (e.g. from CreateP2PKHLockScript).
// chainID binds the coinbase to a specific chain.
func NewCoinbaseTx(height uint64, reward Amount, pkScript []byte, chainID [4]byte) *Transaction {
	var sig bytes.Buffer

	// Encode height as LE uint64, trim trailing zeros, keep at least 1 byte.
	heightBytes := make([]byte, 8)
	heightBytes[0] = byte(height)
	heightBytes[1] = byte(height >> 8)
	heightBytes[2] = byte(height >> 16)
	heightBytes[3] = byte(height >> 24)
	heightBytes[4] = byte(height >> 32)
	heightBytes[5] = byte(height >> 40)
	heightBytes[6] = byte(height >> 48)
	heightBytes[7] = byte(height >> 56)

	trimmed := heightBytes
	for len(trimmed) > 1 && trimmed[len(trimmed)-1] == 0 {
		trimmed = trimmed[:len(trimmed)-1]
	}
	sig.WriteByte(byte(len(trimmed)))
	sig.Write(trimmed)

	return &Transaction{
		Version: 2,
		ChainID: chainID,
		Inputs: []TxIn{
			{
				PrevOut:         OutPoint{TxID: crypto.Hash{}, Index: 0xFFFFFFFF},
				SignatureScript: sig.Bytes(),
				Sequence:        0xFFFFFFFF,
			},
		},
		Outputs: []TxOut{
			{Amount: reward, ScriptVersion: 0, PkScript: pkScript},
		},
		LockTime:     0,
		ExpiryHeight: 0,
	}
}

// ValidateCoinbase checks the structural validity of a coinbase transaction.
func ValidateCoinbase(t *Transaction) error {
	if !t.IsCoinbase() {
		return errors.New("validate: not a coinbase transaction")
	}
	if len(t.Inputs) != 1 {
		return errors.New("validate: coinbase must have exactly 1 input")
	}
	if t.Inputs[0].PrevOut.TxID != ([32]byte{}) {
		return errors.New("validate: coinbase input must reference zero hash")
	}
	if t.Inputs[0].PrevOut.Index != 0xFFFFFFFF {
		return errors.New("validate: coinbase input index must be 0xFFFFFFFF")
	}
	if len(t.Outputs) == 0 {
		return errors.New("validate: coinbase must have at least 1 output")
	}
	// Validate SignatureScript contains height encoding.
	ss := t.Inputs[0].SignatureScript
	if len(ss) < 1 {
		return errors.New("validate: coinbase SignatureScript is empty")
	}
	heightLen := int(ss[0])
	if heightLen == 0 || heightLen > 8 {
		return fmt.Errorf("validate: coinbase height length %d out of range [1,8]", heightLen)
	}
	if len(ss) < 1+heightLen {
		return fmt.Errorf("validate: coinbase SignatureScript too short for height encoding")
	}
	return nil
}

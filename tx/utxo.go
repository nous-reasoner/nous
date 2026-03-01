// Package tx — UTXO set management.
package tx

import (
	"bytes"
	"errors"

	"github.com/nous-chain/nous/crypto"
)

// UTXO represents an unspent transaction output.
type UTXO struct {
	OutPoint   OutPoint
	Output     TxOut
	Height     uint64 // block height where this UTXO was created
	IsCoinbase bool   // true if this output came from a coinbase transaction
}

// UndoEntry records a single UTXO that was spent by a block, so it can be restored on rollback.
type UndoEntry struct {
	SpentUTXO UTXO // the complete UTXO before it was spent
}

// UndoData records everything needed to roll back a block's effects on the UTXO set.
type UndoData struct {
	SpentUTXOs []UndoEntry    // UTXOs consumed by this block (restore on rollback)
	CreatedTxs []crypto.Hash  // transaction hashes created by this block (remove outputs on rollback)
}

// UTXOSet manages the set of all unspent transaction outputs.
type UTXOSet struct {
	utxos map[OutPoint]*UTXO
}

// NewUTXOSet creates an empty UTXO set.
func NewUTXOSet() *UTXOSet {
	return &UTXOSet{
		utxos: make(map[OutPoint]*UTXO),
	}
}

// Add inserts a new UTXO into the set.
func (s *UTXOSet) Add(op OutPoint, output TxOut, height uint64, isCoinbase bool) {
	s.utxos[op] = &UTXO{OutPoint: op, Output: output, Height: height, IsCoinbase: isCoinbase}
}

// Spend removes a UTXO from the set. Returns false if not found.
func (s *UTXOSet) Spend(op OutPoint) bool {
	if _, ok := s.utxos[op]; !ok {
		return false
	}
	delete(s.utxos, op)
	return true
}

// Get retrieves a UTXO by its outpoint. Returns nil if not found.
func (s *UTXOSet) Get(op OutPoint) *UTXO {
	return s.utxos[op]
}

// AddTransaction adds all outputs of a transaction to the UTXO set.
func (s *UTXOSet) AddTransaction(t *Transaction, height uint64) {
	txID := t.TxID()
	cb := t.IsCoinbase()
	for i, out := range t.Outputs {
		op := OutPoint{TxID: txID, Index: uint32(i)}
		s.Add(op, out, height, cb)
	}
}

// ApplyBlock processes a slice of transactions: for each transaction,
// spend its inputs then add its outputs. Coinbase inputs are skipped.
func (s *UTXOSet) ApplyBlock(txs []*Transaction, height uint64) {
	for _, tx := range txs {
		if !tx.IsCoinbase() {
			for _, in := range tx.Inputs {
				s.Spend(in.PrevOut)
			}
		}
		s.AddTransaction(tx, height)
	}
}

// ApplyBlockWithUndo applies transactions like ApplyBlock but also records
// undo data so the block can be rolled back later (for chain reorganization).
func (s *UTXOSet) ApplyBlockWithUndo(txs []*Transaction, height uint64) *UndoData {
	undo := &UndoData{}
	for _, t := range txs {
		if !t.IsCoinbase() {
			for _, in := range t.Inputs {
				// Save the UTXO before spending it.
				if u := s.Get(in.PrevOut); u != nil {
					undo.SpentUTXOs = append(undo.SpentUTXOs, UndoEntry{SpentUTXO: *u})
				}
				s.Spend(in.PrevOut)
			}
		}
		undo.CreatedTxs = append(undo.CreatedTxs, t.TxID())
		s.AddTransaction(t, height)
	}
	return undo
}

// RollbackBlock reverses a block's effects on the UTXO set using undo data.
// 1. Remove all outputs created by the block's transactions.
// 2. Restore all UTXOs that were spent by the block.
func (s *UTXOSet) RollbackBlock(undo *UndoData) error {
	if undo == nil {
		return errors.New("utxo: nil undo data")
	}
	// Step 1: Remove outputs created by this block.
	createdSet := make(map[crypto.Hash]bool, len(undo.CreatedTxs))
	for _, txid := range undo.CreatedTxs {
		createdSet[txid] = true
	}
	for op := range s.utxos {
		if createdSet[op.TxID] {
			delete(s.utxos, op)
		}
	}
	// Step 2: Restore spent UTXOs.
	for _, entry := range undo.SpentUTXOs {
		u := entry.SpentUTXO
		s.utxos[u.OutPoint] = &UTXO{
			OutPoint:   u.OutPoint,
			Output:     u.Output,
			Height:     u.Height,
			IsCoinbase: u.IsCoinbase,
		}
	}
	return nil
}

// Count returns the number of UTXOs in the set.
func (s *UTXOSet) Count() int {
	return len(s.utxos)
}

// Clone returns a deep copy of the UTXO set.
func (s *UTXOSet) Clone() *UTXOSet {
	clone := NewUTXOSet()
	for op, u := range s.utxos {
		clone.utxos[op] = &UTXO{
			OutPoint:   u.OutPoint,
			Output:     TxOut{Amount: u.Output.Amount, ScriptVersion: u.Output.ScriptVersion, PkScript: append([]byte(nil), u.Output.PkScript...)},
			Height:     u.Height,
			IsCoinbase: u.IsCoinbase,
		}
	}
	return clone
}

// FindByPubKeyHash returns all UTXOs whose PkScript is a P2PKH script
// paying to the given 20-byte public key hash.
func (s *UTXOSet) FindByPubKeyHash(pubKeyHash []byte) []*UTXO {
	var result []*UTXO
	for _, utxo := range s.utxos {
		scriptHash := ExtractPubKeyHashFromP2PKH(utxo.Output.PkScript)
		if scriptHash != nil && bytes.Equal(scriptHash, pubKeyHash) {
			result = append(result, utxo)
		}
	}
	return result
}

// GetBalance returns the total balance for a given 20-byte public key hash
// by scanning all UTXOs for matching P2PKH scripts.
// Uses overflow-safe addition; caps at MaxAmount if sum would overflow.
func (s *UTXOSet) GetBalance(pubKeyHash []byte) int64 {
	var total int64
	for _, utxo := range s.utxos {
		scriptHash := ExtractPubKeyHashFromP2PKH(utxo.Output.PkScript)
		if scriptHash != nil && bytes.Equal(scriptHash, pubKeyHash) {
			sum, err := safeAdd(total, utxo.Output.Amount)
			if err != nil {
				return MaxAmount // cap at max if overflow
			}
			total = sum
		}
	}
	return total
}

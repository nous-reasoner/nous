package tx

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"log"

	bolt "go.etcd.io/bbolt"
)

var utxoBucket = []byte("utxos")

// BoltUTXOSet is a UTXOStore backed by a BoltDB file.
type BoltUTXOSet struct {
	db *bolt.DB
}

// NewBoltUTXOSet opens (or creates) a BoltDB-backed UTXO set at dbPath.
func NewBoltUTXOSet(dbPath string) (*BoltUTXOSet, error) {
	db, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		return nil, fmt.Errorf("utxo bolt: open %s: %w", dbPath, err)
	}
	// Ensure the bucket exists.
	if err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(utxoBucket)
		return err
	}); err != nil {
		db.Close()
		return nil, fmt.Errorf("utxo bolt: create bucket: %w", err)
	}
	return &BoltUTXOSet{db: db}, nil
}

// Close closes the underlying BoltDB.
func (s *BoltUTXOSet) Close() error {
	return s.db.Close()
}

// --- key / value encoding ---

// encodeKey: TxID(32) + Index(4 LE) = 36 bytes.
func encodeKey(op OutPoint) []byte {
	key := make([]byte, 36)
	copy(key[:32], op.TxID[:])
	binary.LittleEndian.PutUint32(key[32:], op.Index)
	return key
}

func decodeKey(key []byte) OutPoint {
	var op OutPoint
	copy(op.TxID[:], key[:32])
	op.Index = binary.LittleEndian.Uint32(key[32:])
	return op
}

// encodeValue: Amount(8 LE) + ScriptLen(2 LE) + Script(var) + Height(8 LE) + Flags(1).
// Flags bit 0 = IsCoinbase.
func encodeValue(u *UTXO) []byte {
	scriptLen := len(u.Output.PkScript)
	buf := make([]byte, 8+2+scriptLen+8+1)
	off := 0
	binary.LittleEndian.PutUint64(buf[off:], uint64(u.Output.Amount))
	off += 8
	binary.LittleEndian.PutUint16(buf[off:], uint16(scriptLen))
	off += 2
	copy(buf[off:], u.Output.PkScript)
	off += scriptLen
	binary.LittleEndian.PutUint64(buf[off:], u.Height)
	off += 8
	var flags byte
	if u.IsCoinbase {
		flags = 1
	}
	buf[off] = flags
	return buf
}

func decodeValue(key, val []byte) *UTXO {
	// Minimum value size: Amount(8) + ScriptLen(2) + Height(8) + Flags(1) = 19 bytes.
	if len(val) < 19 {
		return nil
	}
	op := decodeKey(key)
	off := 0
	amount := int64(binary.LittleEndian.Uint64(val[off:]))
	off += 8
	scriptLen := int(binary.LittleEndian.Uint16(val[off:]))
	off += 2
	// Validate that the remaining data is large enough for script + height + flags.
	if off+scriptLen+8+1 > len(val) {
		return nil
	}
	script := make([]byte, scriptLen)
	copy(script, val[off:off+scriptLen])
	off += scriptLen
	height := binary.LittleEndian.Uint64(val[off:])
	off += 8
	isCoinbase := val[off]&1 != 0
	return &UTXO{
		OutPoint:   op,
		Output:     TxOut{Amount: amount, PkScript: script},
		Height:     height,
		IsCoinbase: isCoinbase,
	}
}

// --- UTXOStore interface ---

func (s *BoltUTXOSet) Add(op OutPoint, output TxOut, height uint64, isCoinbase bool) {
	u := &UTXO{OutPoint: op, Output: output, Height: height, IsCoinbase: isCoinbase}
	key := encodeKey(op)
	val := encodeValue(u)
	if err := s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(utxoBucket).Put(key, val)
	}); err != nil {
		log.Fatalf("utxo bolt: Add failed: %v", err)
	}
}

func (s *BoltUTXOSet) Spend(op OutPoint) bool {
	key := encodeKey(op)
	var found bool
	if err := s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(utxoBucket)
		if b.Get(key) != nil {
			found = true
			return b.Delete(key)
		}
		return nil
	}); err != nil {
		log.Fatalf("utxo bolt: Spend failed: %v", err)
	}
	return found
}

func (s *BoltUTXOSet) Get(op OutPoint) *UTXO {
	key := encodeKey(op)
	var u *UTXO
	s.db.View(func(tx *bolt.Tx) error {
		val := tx.Bucket(utxoBucket).Get(key)
		if val != nil {
			u = decodeValue(key, val)
		}
		return nil
	})
	return u
}

func (s *BoltUTXOSet) AddTransaction(t *Transaction, height uint64) {
	txID := t.TxID()
	cb := t.IsCoinbase()
	if err := s.db.Update(func(btx *bolt.Tx) error {
		b := btx.Bucket(utxoBucket)
		for i, out := range t.Outputs {
			if IsUnspendable(out.PkScript) {
				continue
			}
			op := OutPoint{TxID: txID, Index: uint32(i)}
			u := &UTXO{OutPoint: op, Output: out, Height: height, IsCoinbase: cb}
			if err := b.Put(encodeKey(op), encodeValue(u)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		log.Fatalf("utxo bolt: AddTransaction failed: %v", err)
	}
}

func (s *BoltUTXOSet) ApplyBlock(txs []*Transaction, height uint64) {
	if err := s.db.Update(func(btx *bolt.Tx) error {
		b := btx.Bucket(utxoBucket)
		for _, t := range txs {
			if !t.IsCoinbase() {
				for _, in := range t.Inputs {
					b.Delete(encodeKey(in.PrevOut))
				}
			}
			txID := t.TxID()
			cb := t.IsCoinbase()
			for i, out := range t.Outputs {
				if IsUnspendable(out.PkScript) {
					continue
				}
				op := OutPoint{TxID: txID, Index: uint32(i)}
				u := &UTXO{OutPoint: op, Output: out, Height: height, IsCoinbase: cb}
				if err := b.Put(encodeKey(op), encodeValue(u)); err != nil {
					return err
				}
			}
		}
		return nil
	}); err != nil {
		log.Fatalf("utxo bolt: ApplyBlock failed: %v", err)
	}
}

func (s *BoltUTXOSet) ApplyBlockWithUndo(txs []*Transaction, height uint64) *UndoData {
	undo := &UndoData{}
	if err := s.db.Update(func(btx *bolt.Tx) error {
		b := btx.Bucket(utxoBucket)
		for _, t := range txs {
			if !t.IsCoinbase() {
				for _, in := range t.Inputs {
					key := encodeKey(in.PrevOut)
					val := b.Get(key)
					if val != nil {
						u := decodeValue(key, val)
						if u != nil {
							undo.SpentUTXOs = append(undo.SpentUTXOs, UndoEntry{SpentUTXO: *u})
						}
					}
					b.Delete(key)
				}
			}
			txID := t.TxID()
			undo.CreatedTxs = append(undo.CreatedTxs, txID)
			cb := t.IsCoinbase()
			for i, out := range t.Outputs {
				if IsUnspendable(out.PkScript) {
					continue
				}
				op := OutPoint{TxID: txID, Index: uint32(i)}
				u := &UTXO{OutPoint: op, Output: out, Height: height, IsCoinbase: cb}
				if err := b.Put(encodeKey(op), encodeValue(u)); err != nil {
					return err
				}
			}
		}
		return nil
	}); err != nil {
		log.Fatalf("utxo bolt: ApplyBlockWithUndo failed: %v", err)
	}
	return undo
}

func (s *BoltUTXOSet) RollbackBlock(undo *UndoData) error {
	if undo == nil {
		return errors.New("utxo: nil undo data")
	}
	return s.db.Update(func(btx *bolt.Tx) error {
		b := btx.Bucket(utxoBucket)
		// Step 1: Remove outputs created by this block.
		// Collect keys first, then delete. Modifying a bucket during cursor
		// iteration is unsafe in BoltDB — the cursor may skip keys, leaving
		// phantom UTXOs that inflate the supply.
		for _, txid := range undo.CreatedTxs {
			prefix := txid[:]
			var toDelete [][]byte
			c := b.Cursor()
			for k, _ := c.Seek(prefix); k != nil && bytes.HasPrefix(k, prefix); k, _ = c.Next() {
				keyCopy := make([]byte, len(k))
				copy(keyCopy, k)
				toDelete = append(toDelete, keyCopy)
			}
			for _, k := range toDelete {
				if err := b.Delete(k); err != nil {
					return err
				}
			}
		}
		// Step 2: Restore spent UTXOs.
		for _, entry := range undo.SpentUTXOs {
			u := entry.SpentUTXO
			key := encodeKey(u.OutPoint)
			val := encodeValue(&u)
			if err := b.Put(key, val); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *BoltUTXOSet) Count() int {
	var count int
	s.db.View(func(tx *bolt.Tx) error {
		count = tx.Bucket(utxoBucket).Stats().KeyN
		return nil
	})
	return count
}

// TotalSupply returns the sum of all UTXO amounts.
func (s *BoltUTXOSet) TotalSupply() int64 {
	var total int64
	s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(utxoBucket).ForEach(func(k, v []byte) error {
			u := decodeValue(k, v)
			total += u.Output.Amount
			return nil
		})
	})
	return total
}

func (s *BoltUTXOSet) FindByPubKeyHash(pubKeyHash []byte) []*UTXO {
	var result []*UTXO
	s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(utxoBucket)
		return b.ForEach(func(k, v []byte) error {
			u := decodeValue(k, v)
			scriptHash := ExtractPubKeyHashFromP2PKH(u.Output.PkScript)
			if scriptHash != nil && bytes.Equal(scriptHash, pubKeyHash) {
				result = append(result, u)
			}
			return nil
		})
	})
	return result
}

func (s *BoltUTXOSet) GetBalance(pubKeyHash []byte) int64 {
	var total int64
	s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(utxoBucket)
		return b.ForEach(func(k, v []byte) error {
			u := decodeValue(k, v)
			scriptHash := ExtractPubKeyHashFromP2PKH(u.Output.PkScript)
			if scriptHash != nil && bytes.Equal(scriptHash, pubKeyHash) {
				sum, err := safeAdd(total, u.Output.Amount)
				if err != nil {
					total = MaxAmount
					return nil
				}
				total = sum
			}
			return nil
		})
	})
	return total
}

// RebuildFromBlocks clears the UTXO set and replays all blocks from the given
// block store to rebuild it. This is called when utxo.db is empty or missing.
func (s *BoltUTXOSet) RebuildFromBlocks(loadBlock func(height uint64) ([]*Transaction, error), tipHeight uint64) error {
	// Clear existing data.
	if err := s.db.Update(func(tx *bolt.Tx) error {
		if err := tx.DeleteBucket(utxoBucket); err != nil {
			return err
		}
		_, err := tx.CreateBucket(utxoBucket)
		return err
	}); err != nil {
		return fmt.Errorf("utxo bolt: clear bucket: %w", err)
	}
	// Replay all blocks.
	for h := uint64(0); h <= tipHeight; h++ {
		txs, err := loadBlock(h)
		if err != nil {
			return fmt.Errorf("utxo bolt: rebuild height %d: %w", h, err)
		}
		s.ApplyBlock(txs, h)
	}
	return nil
}

// Compile-time check: *BoltUTXOSet implements UTXOStore.
var _ UTXOStore = (*BoltUTXOSet)(nil)

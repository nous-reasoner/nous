package network

import (
	"math"
	"sort"
	"sync"

	"github.com/nous-chain/nous/crypto"
	"github.com/nous-chain/nous/tx"
)

// MempoolEntry wraps a transaction with its fee rate for priority ordering.
type MempoolEntry struct {
	Tx      *tx.Transaction
	TxID    crypto.Hash
	FeeRate int64 // fee per byte (nou/byte), higher = higher priority
	Size    int   // serialized size in bytes
}

// maxInt64 is math.MaxInt64, stored as a package variable for overflow checks.
const maxInt64 = int64(math.MaxInt64)

// MaxMempoolTxCount is the maximum number of transactions in the mempool.
const MaxMempoolTxCount = 5_000

// Mempool holds unconfirmed transactions waiting to be included in a block.
type Mempool struct {
	mu      sync.RWMutex
	entries map[crypto.Hash]*MempoolEntry
}

// NewMempool creates an empty mempool.
func NewMempool() *Mempool {
	return &Mempool{
		entries: make(map[crypto.Hash]*MempoolEntry),
	}
}

// Add inserts a transaction into the mempool. Returns false if already present.
func (mp *Mempool) Add(transaction *tx.Transaction) bool {
	txID := transaction.TxID()
	serialized := transaction.Serialize()
	size := len(serialized)

	// Compute output sum (placeholder metric; real fee comes from AddWithFee).
	var totalOut int64
	for _, out := range transaction.Outputs {
		// Overflow-safe addition; stop accumulating on overflow.
		if out.Amount > 0 && totalOut > maxInt64-out.Amount {
			break
		}
		totalOut += out.Amount
	}

	mp.mu.Lock()
	defer mp.mu.Unlock()

	if _, exists := mp.entries[txID]; exists {
		return false
	}
	if len(mp.entries) >= MaxMempoolTxCount {
		return false
	}

	mp.entries[txID] = &MempoolEntry{
		Tx:   transaction,
		TxID: txID,
		Size: size,
	}
	return true
}

// AddWithFee inserts a transaction with a precomputed fee.
func (mp *Mempool) AddWithFee(transaction *tx.Transaction, fee int64) bool {
	txID := transaction.TxID()
	serialized := transaction.Serialize()
	size := len(serialized)

	feeRate := int64(0)
	if size > 0 {
		feeRate = fee * 1000 / int64(size) // fee per kilobyte
	}

	mp.mu.Lock()
	defer mp.mu.Unlock()

	if _, exists := mp.entries[txID]; exists {
		return false
	}
	if len(mp.entries) >= MaxMempoolTxCount {
		return false
	}

	mp.entries[txID] = &MempoolEntry{
		Tx:      transaction,
		TxID:    txID,
		FeeRate: feeRate,
		Size:    size,
	}
	return true
}

// Get retrieves a transaction by its hash.
func (mp *Mempool) Get(txID crypto.Hash) *tx.Transaction {
	mp.mu.RLock()
	defer mp.mu.RUnlock()
	e, ok := mp.entries[txID]
	if !ok {
		return nil
	}
	return e.Tx
}

// Has checks if a transaction is in the mempool.
func (mp *Mempool) Has(txID crypto.Hash) bool {
	mp.mu.RLock()
	defer mp.mu.RUnlock()
	_, ok := mp.entries[txID]
	return ok
}

// Remove deletes a transaction from the mempool.
func (mp *Mempool) Remove(txID crypto.Hash) {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	delete(mp.entries, txID)
}

// RemoveConfirmed removes all transactions that appear in the given block.
func (mp *Mempool) RemoveConfirmed(txs []*tx.Transaction) {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	for _, t := range txs {
		delete(mp.entries, t.TxID())
	}
}

// Count returns the number of transactions in the mempool.
func (mp *Mempool) Count() int {
	mp.mu.RLock()
	defer mp.mu.RUnlock()
	return len(mp.entries)
}

// GetByFeeRate returns all mempool entries sorted by fee rate (highest first).
func (mp *Mempool) GetByFeeRate() []*MempoolEntry {
	mp.mu.RLock()
	defer mp.mu.RUnlock()
	entries := make([]*MempoolEntry, 0, len(mp.entries))
	for _, e := range mp.entries {
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].FeeRate > entries[j].FeeRate
	})
	return entries
}

// GetTopN returns the top N transactions by fee rate.
func (mp *Mempool) GetTopN(n int) []*tx.Transaction {
	entries := mp.GetByFeeRate()
	if n > len(entries) {
		n = len(entries)
	}
	txs := make([]*tx.Transaction, n)
	for i := 0; i < n; i++ {
		txs[i] = entries[i].Tx
	}
	return txs
}

// All returns all transactions in the mempool (no guaranteed order).
func (mp *Mempool) All() []*tx.Transaction {
	mp.mu.RLock()
	defer mp.mu.RUnlock()
	txs := make([]*tx.Transaction, 0, len(mp.entries))
	for _, e := range mp.entries {
		txs = append(txs, e.Tx)
	}
	return txs
}

package network

import (
	"math"
	"sort"
	"sync"

	"nous/crypto"
	"nous/tx"
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
	// spent tracks every OutPoint consumed by a mempool transaction,
	// mapping each OutPoint to the TxID that spends it.
	// This prevents two transactions from spending the same UTXO.
	spent map[tx.OutPoint]crypto.Hash
}

// NewMempool creates an empty mempool.
func NewMempool() *Mempool {
	return &Mempool{
		entries: make(map[crypto.Hash]*MempoolEntry),
		spent:   make(map[tx.OutPoint]crypto.Hash),
	}
}

// hasDoubleSpend checks if any input of the transaction conflicts with an
// existing mempool transaction. Must be called with mp.mu held.
func (mp *Mempool) hasDoubleSpend(transaction *tx.Transaction) bool {
	if transaction.IsCoinbase() {
		return false
	}
	for _, in := range transaction.Inputs {
		if _, exists := mp.spent[in.PrevOut]; exists {
			return true
		}
	}
	return false
}

// trackSpent records all inputs of a transaction in the spent set.
// Must be called with mp.mu held.
func (mp *Mempool) trackSpent(transaction *tx.Transaction, txID crypto.Hash) {
	if transaction.IsCoinbase() {
		return
	}
	for _, in := range transaction.Inputs {
		mp.spent[in.PrevOut] = txID
	}
}

// untrackSpent removes all inputs of a transaction from the spent set.
// Must be called with mp.mu held.
func (mp *Mempool) untrackSpent(transaction *tx.Transaction) {
	if transaction.IsCoinbase() {
		return
	}
	for _, in := range transaction.Inputs {
		delete(mp.spent, in.PrevOut)
	}
}

// Add inserts a transaction into the mempool.
// Returns false if already present, the pool is full, or the transaction
// would double-spend an OutPoint already consumed by another mempool tx.
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
	if mp.hasDoubleSpend(transaction) {
		return false
	}

	mp.entries[txID] = &MempoolEntry{
		Tx:   transaction,
		TxID: txID,
		Size: size,
	}
	mp.trackSpent(transaction, txID)
	return true
}

// AddWithFee inserts a transaction with a precomputed fee.
// Returns false if already present, the pool is full, or the transaction
// would double-spend an OutPoint already consumed by another mempool tx.
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
	if mp.hasDoubleSpend(transaction) {
		return false
	}

	mp.entries[txID] = &MempoolEntry{
		Tx:      transaction,
		TxID:    txID,
		FeeRate: feeRate,
		Size:    size,
	}
	mp.trackSpent(transaction, txID)
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

// Remove deletes a transaction from the mempool and cleans its spent entries.
func (mp *Mempool) Remove(txID crypto.Hash) {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	if e, ok := mp.entries[txID]; ok {
		mp.untrackSpent(e.Tx)
		delete(mp.entries, txID)
	}
}

// RemoveConfirmed removes all transactions that appear in the given block
// and also removes any remaining mempool transactions that conflict with the
// block's inputs (i.e. mempool double-spends that lost to the confirmed tx).
func (mp *Mempool) RemoveConfirmed(txs []*tx.Transaction) {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	// Collect all OutPoints spent by the confirmed block.
	blockSpent := make(map[tx.OutPoint]bool)
	for _, t := range txs {
		if t.IsCoinbase() {
			continue
		}
		for _, in := range t.Inputs {
			blockSpent[in.PrevOut] = true
		}
	}

	// Remove confirmed transactions.
	for _, t := range txs {
		id := t.TxID()
		if e, ok := mp.entries[id]; ok {
			mp.untrackSpent(e.Tx)
			delete(mp.entries, id)
		}
	}

	// Evict any remaining mempool tx that spends an OutPoint now confirmed.
	// This handles the case where tx A and tx B both spend UTXO X:
	// the block included tx A, so tx B (which lost) must be evicted.
	var evict []crypto.Hash
	for op := range blockSpent {
		if spender, exists := mp.spent[op]; exists {
			evict = append(evict, spender)
		}
	}
	for _, id := range evict {
		if e, ok := mp.entries[id]; ok {
			mp.untrackSpent(e.Tx)
			delete(mp.entries, id)
		}
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

package node

import (
	"fmt"
	"log"
	"sync"

	"nous/block"
	"nous/consensus"
	"nous/crypto"
	"nous/storage"
	"nous/tx"
)

// ChainAdapter bridges consensus.ChainState + storage.BlockStore to the
// network.ChainAccess interface required by BlockSyncer.
//
// On mining nodes, the miner writes blocks directly to the chain and store,
// bypassing this adapter. The adapter handles this by refreshing its index
// from the store when it detects a cache miss.
//
// The chainMu mutex is shared with the Reasoner to prevent concurrent access
// to ChainState, which is not internally synchronized.
type ChainAdapter struct {
	chainMu    *sync.Mutex
	chain      *consensus.ChainState
	store      *storage.BlockStore
	hashIndex  map[crypto.Hash]uint64 // block hash → height
	heightHash map[uint64]crypto.Hash // height → block hash
	indexed    uint64                 // highest height indexed so far
}

// NewChainAdapter creates a new chain adapter.
// chainMu must be shared with the Reasoner to prevent data races on ChainState.
func NewChainAdapter(
	chain *consensus.ChainState,
	store *storage.BlockStore,
	chainMu *sync.Mutex,
) *ChainAdapter {
	ca := &ChainAdapter{
		chainMu:    chainMu,
		chain:      chain,
		store:      store,
		hashIndex:  make(map[crypto.Hash]uint64),
		heightHash: make(map[uint64]crypto.Hash),
	}
	// Index genesis block from store.
	ca.refreshIndex()
	return ca
}

// refreshIndex indexes any new blocks in the store that we haven't indexed yet.
// Must be called with chainMu held.
func (ca *ChainAdapter) refreshIndex() {
	// Index blocks from indexed+1 up to what's available in store.
	// Cap at the current chain height to avoid indexing stale blocks
	// that remain in the store after a reorg to a shorter chain.
	chainHeight := ca.chain.Height
	start := ca.indexed + 1
	if ca.indexed == 0 && len(ca.hashIndex) == 0 {
		start = 0 // first call, index from genesis
	}
	for h := start; h <= chainHeight; h++ {
		blk, err := ca.store.LoadBlockByHeight(h)
		if err != nil {
			break // no more blocks in store
		}
		hash := blk.Header.Hash()
		ca.hashIndex[hash] = h
		ca.heightHash[h] = hash
		ca.indexed = h
	}
}

func (ca *ChainAdapter) Height() uint64 {
	ca.chainMu.Lock()
	defer ca.chainMu.Unlock()
	return ca.chain.Height
}

func (ca *ChainAdapter) TipHash() crypto.Hash {
	ca.chainMu.Lock()
	defer ca.chainMu.Unlock()
	return ca.chain.Tip.Hash()
}

func (ca *ChainAdapter) HasBlock(hash crypto.Hash) bool {
	ca.chainMu.Lock()
	defer ca.chainMu.Unlock()
	if _, ok := ca.hashIndex[hash]; ok {
		return true
	}
	// Maybe the miner added blocks we haven't indexed yet.
	ca.refreshIndex()
	_, ok := ca.hashIndex[hash]
	return ok
}

func (ca *ChainAdapter) GetBlockByHeight(height uint64) (*block.Block, error) {
	// Store has its own lock; no need for chainMu.
	return ca.store.LoadBlockByHeight(height)
}

func (ca *ChainAdapter) GetBlockHashByHeight(height uint64) (crypto.Hash, error) {
	ca.chainMu.Lock()
	defer ca.chainMu.Unlock()
	if hash, ok := ca.heightHash[height]; ok {
		return hash, nil
	}
	// Try refreshing from store (miner may have added new blocks).
	ca.refreshIndex()
	if hash, ok := ca.heightHash[height]; ok {
		return hash, nil
	}
	return crypto.Hash{}, fmt.Errorf("block at height %d not found", height)
}

func (ca *ChainAdapter) GetBlockByHash(hash crypto.Hash) (*block.Block, error) {
	ca.chainMu.Lock()
	defer ca.chainMu.Unlock()
	height, ok := ca.hashIndex[hash]
	if !ok {
		ca.refreshIndex()
		height, ok = ca.hashIndex[hash]
		if !ok {
			return nil, fmt.Errorf("block %x not found", hash[:8])
		}
	}
	return ca.store.LoadBlockByHeight(height)
}

// AddBlock validates and adds a block to the chain. Returns the new chain height.
//
// Handles three outcomes from chain.AddBlock:
//   - Tip extension: block extends the current tip (height += 1)
//   - Side-chain block: block is stored in the index but the tip is unchanged
//   - Reorg: a heavier side chain becomes the new main chain
//
// Only main-chain blocks are persisted to the store. Network-side effects
// (SetBlockHeight, RemoveConfirmed) are NOT called here — callers handle those.
func (ca *ChainAdapter) AddBlock(blk *block.Block) (uint64, error) {
	ca.chainMu.Lock()
	defer ca.chainMu.Unlock()

	oldHeight := ca.chain.Height
	oldTipHash := ca.chain.Tip.Hash()

	if err := ca.chain.AddBlock(blk); err != nil {
		return 0, err
	}

	newHeight := ca.chain.Height
	newTipHash := ca.chain.Tip.Hash()

	// If the tip didn't change, this is a lighter side-chain block.
	// chain.AddBlock stored it in the block index but didn't switch tips.
	// Don't save to disk or update our caches.
	if newTipHash == oldTipHash {
		return oldHeight, nil
	}

	blkHash := blk.Header.Hash()

	// Simple tip extension: the submitted block became the new tip.
	if newHeight == oldHeight+1 && blkHash == newTipHash {
		if err := ca.store.SaveBlock(blk, newHeight); err != nil {
			log.Printf("chain_adapter: save block %d: %v", newHeight, err)
		}
		// Remove any stale hash that was previously at this height
		// (possible if a stale block file was left from a prior reorg).
		if oldHash, ok := ca.heightHash[newHeight]; ok && oldHash != blkHash {
			delete(ca.hashIndex, oldHash)
		}
		ca.hashIndex[blkHash] = newHeight
		ca.heightHash[newHeight] = blkHash
		ca.indexed = newHeight
		ca.store.SaveChainTip(storage.ChainTip{Hash: newTipHash, Height: newHeight})
		return newHeight, nil
	}

	// Reorg: the tip changed but not via simple extension.
	// Rebuild the store and caches for affected heights.
	log.Printf("chain_adapter: reorg detected (height %d→%d)", oldHeight, newHeight)

	// Clean stale entries for heights above the new tip and delete
	// the stale block files from the store. Without deletion,
	// refreshIndex could re-index these stale blocks, causing
	// handleGetBlocks to return orphan-producing hashes to peers.
	for h := newHeight + 1; h <= oldHeight; h++ {
		if hash, ok := ca.heightHash[h]; ok {
			delete(ca.hashIndex, hash)
			delete(ca.heightHash, h)
		}
		if err := ca.store.DeleteBlock(h); err != nil {
			log.Printf("chain_adapter: reorg: delete stale block %d: %v", h, err)
		}
	}

	// Find the fork point by walking backwards from the lower of the two heights.
	forkHeight := newHeight
	if oldHeight < forkHeight {
		forkHeight = oldHeight
	}
	for forkHeight > 0 {
		chainBlock := ca.chain.GetMainChainBlock(forkHeight)
		if chainBlock == nil {
			forkHeight--
			continue
		}
		chainHash := chainBlock.Header.Hash()
		if cachedHash, ok := ca.heightHash[forkHeight]; ok && cachedHash == chainHash {
			break
		}
		forkHeight--
	}

	// Save all blocks from fork point+1 to new tip.
	for h := forkHeight + 1; h <= newHeight; h++ {
		// Remove stale hash entry for this height.
		if oldHash, ok := ca.heightHash[h]; ok {
			delete(ca.hashIndex, oldHash)
		}
		chainBlock := ca.chain.GetMainChainBlock(h)
		if chainBlock == nil {
			log.Printf("chain_adapter: reorg: missing block at height %d", h)
			continue
		}
		if err := ca.store.SaveBlock(chainBlock, h); err != nil {
			log.Printf("chain_adapter: reorg: save block %d: %v", h, err)
		}
		hash := chainBlock.Header.Hash()
		ca.hashIndex[hash] = h
		ca.heightHash[h] = hash
	}
	ca.indexed = newHeight

	ca.store.SaveChainTip(storage.ChainTip{Hash: newTipHash, Height: newHeight})
	log.Printf("chain_adapter: reorg complete (fork at %d, new height %d)", forkHeight, newHeight)

	return newHeight, nil
}

// ValidateTx validates a transaction against the current UTXO set and chain height.
func (ca *ChainAdapter) ValidateTx(txn *tx.Transaction) error {
	ca.chainMu.Lock()
	defer ca.chainMu.Unlock()
	return tx.ValidateTx(txn, ca.chain.UTXOSet, ca.chain.Height, ca.chain.IsTestnet)
}

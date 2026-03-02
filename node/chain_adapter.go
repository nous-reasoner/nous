package node

import (
	"fmt"
	"log"
	"sync"

	"nous/block"
	"nous/consensus"
	"nous/crypto"
	"nous/network"
	"nous/storage"
)

// ChainAdapter bridges consensus.ChainState + storage.BlockStore to the
// network.ChainAccess interface required by BlockSyncer.
//
// On mining nodes, the miner writes blocks directly to the chain and store,
// bypassing this adapter. The adapter handles this by refreshing its index
// from the store when it detects a cache miss.
type ChainAdapter struct {
	mu         sync.Mutex
	chain      *consensus.ChainState
	store      *storage.BlockStore
	server     *network.Server
	hashIndex  map[crypto.Hash]uint64 // block hash → height
	heightHash map[uint64]crypto.Hash // height → block hash
	indexed    uint64                 // highest height indexed so far
}

// NewChainAdapter creates a new chain adapter.
func NewChainAdapter(
	chain *consensus.ChainState,
	store *storage.BlockStore,
	server *network.Server,
) *ChainAdapter {
	ca := &ChainAdapter{
		chain:      chain,
		store:      store,
		server:     server,
		hashIndex:  make(map[crypto.Hash]uint64),
		heightHash: make(map[uint64]crypto.Hash),
	}
	// Index genesis block from store.
	ca.refreshIndex()
	return ca
}

// refreshIndex indexes any new blocks in the store that we haven't indexed yet.
// Must be called with ca.mu held.
func (ca *ChainAdapter) refreshIndex() {
	// Index blocks from indexed+1 up to what's available in store.
	start := ca.indexed + 1
	if ca.indexed == 0 && len(ca.hashIndex) == 0 {
		start = 0 // first call, index from genesis
	}
	for h := start; ; h++ {
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
	ca.mu.Lock()
	defer ca.mu.Unlock()
	return ca.chain.Height
}

func (ca *ChainAdapter) TipHash() crypto.Hash {
	ca.mu.Lock()
	defer ca.mu.Unlock()
	return ca.chain.Tip.Hash()
}

func (ca *ChainAdapter) HasBlock(hash crypto.Hash) bool {
	ca.mu.Lock()
	defer ca.mu.Unlock()
	if _, ok := ca.hashIndex[hash]; ok {
		return true
	}
	// Maybe the miner added blocks we haven't indexed yet.
	ca.refreshIndex()
	_, ok := ca.hashIndex[hash]
	return ok
}

func (ca *ChainAdapter) GetBlockByHeight(height uint64) (*block.Block, error) {
	// Store has its own lock; no need for ca.mu.
	return ca.store.LoadBlockByHeight(height)
}

func (ca *ChainAdapter) GetBlockHashByHeight(height uint64) (crypto.Hash, error) {
	ca.mu.Lock()
	defer ca.mu.Unlock()
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
	ca.mu.Lock()
	defer ca.mu.Unlock()
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

func (ca *ChainAdapter) AddBlock(blk *block.Block) (uint64, error) {
	ca.mu.Lock()
	defer ca.mu.Unlock()

	newHeight := ca.chain.Height + 1
	if err := ca.chain.AddBlock(blk); err != nil {
		return 0, err
	}

	if err := ca.store.SaveBlock(blk, newHeight); err != nil {
		log.Printf("chain_adapter: save block %d: %v", newHeight, err)
	}

	tipHash := blk.Header.Hash()
	ca.hashIndex[tipHash] = newHeight
	ca.heightHash[newHeight] = tipHash
	ca.indexed = newHeight

	ca.store.SaveChainTip(storage.ChainTip{Hash: tipHash, Height: newHeight})
	ca.server.SetBlockHeight(newHeight)
	ca.server.Mempool().RemoveConfirmed(blk.Transactions)

	return newHeight, nil
}

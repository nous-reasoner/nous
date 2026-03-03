// Package node provides the reasoning loop and JSON-RPC server for the NOUS daemon.
package node

import (
	"log"
	"sync"
	"time"

	"nous/block"
	"nous/consensus"
	"nous/crypto"
	"nous/network"
	"nous/storage"
	"nous/tx"
)

// MaxBlockTxs is the maximum number of mempool transactions per block.
const MaxBlockTxs = 500

// Reasoner runs the reasoning (mining) loop in a background goroutine.
type Reasoner struct {
	chain   *consensus.ChainState
	server  *network.Server
	store   *storage.BlockStore
	pubKey  *crypto.PublicKey
	chainMu *sync.Mutex // shared with ChainAdapter to prevent data races on ChainState

	mu      sync.Mutex // protects running/quit/done
	running bool
	quit    chan struct{}
	done    chan struct{}
}

// NewReasoner creates a new reasoner.
// chainMu must be the same mutex used by ChainAdapter.
func NewReasoner(
	chain *consensus.ChainState,
	server *network.Server,
	store *storage.BlockStore,
	pubKey *crypto.PublicKey,
	chainMu *sync.Mutex,
) *Reasoner {
	return &Reasoner{
		chain:   chain,
		server:  server,
		store:   store,
		pubKey:  pubKey,
		chainMu: chainMu,
	}
}

// Start begins the reasoning loop in a background goroutine.
func (r *Reasoner) Start() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running {
		return
	}
	r.running = true
	r.quit = make(chan struct{})
	r.done = make(chan struct{})
	go r.loop()
}

// StartReasoning is an alias for Start.
func (r *Reasoner) StartReasoning() {
	r.Start()
}

// Stop signals the reasoning loop to stop and waits for it to finish.
func (r *Reasoner) Stop() {
	r.mu.Lock()
	if !r.running {
		r.mu.Unlock()
		return
	}
	r.running = false
	close(r.quit)
	r.mu.Unlock()
	<-r.done
}

// IsRunning returns whether the reasoner is active.
func (r *Reasoner) IsRunning() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running
}

// ApplyBlock adds an externally received block to the chain state and store.
func (r *Reasoner) ApplyBlock(blk *block.Block) error {
	r.chainMu.Lock()
	defer r.chainMu.Unlock()

	oldHeight := r.chain.Height
	if err := r.chain.AddBlock(blk); err != nil {
		return err
	}

	newHeight := r.chain.Height
	// Only persist if the tip actually changed (skip lighter side-chain blocks).
	if newHeight != oldHeight {
		tipHash := r.chain.Tip.Hash()
		if err := r.store.SaveBlock(blk, newHeight); err != nil {
			log.Printf("reasoner: save block %d: %v", newHeight, err)
		}
		r.store.SaveChainTip(storage.ChainTip{Hash: tipHash, Height: newHeight})
		r.server.SetBlockHeight(newHeight)
		r.server.Mempool().RemoveConfirmed(blk.Transactions)
	}
	return nil
}

// Chain returns the chain state (for RPC queries).
func (r *Reasoner) Chain() *consensus.ChainState {
	return r.chain
}

func (r *Reasoner) loop() {
	defer close(r.done)
	log.Println("reasoner: started")

	for {
		select {
		case <-r.quit:
			log.Println("reasoner: stopped")
			return
		default:
		}

		r.reasonOne()

		// Brief pause between reasoning attempts.
		select {
		case <-r.quit:
			return
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (r *Reasoner) reasonOne() {
	// Snapshot chain state under the shared lock.
	r.chainMu.Lock()
	tip := r.chain.Tip
	height := r.chain.Height + 1
	diff := r.chain.GetDifficulty()
	utxoSet := r.chain.UTXOSet
	r.chainMu.Unlock()

	pubKeyHash := crypto.Hash160(r.pubKey.SerializeCompressed())

	// Gather mempool transactions.
	mempoolTxs := r.server.Mempool().GetTopN(MaxBlockTxs)

	// Filter valid transactions against current UTXO set.
	var validTxs []*tx.Transaction
	for _, t := range mempoolTxs {
		if err := tx.ValidateTransaction(t, utxoSet, height, r.chain.IsTestnet); err == nil {
			validTxs = append(validTxs, t)
		}
	}

	log.Printf("reasoner: reasoning block %d (%d txs)...", height, len(validTxs))
	log.Printf("reasoner: block %d target=0x%08x", height, consensus.TargetToCompact(diff.PoWTarget))

	blk, err := consensus.MineBlock(tip, validTxs, pubKeyHash, diff, height, utxoSet, r.chain.IsTestnet)
	if err != nil {
		log.Printf("reasoner: block %d failed: %v", height, err)
		return
	}

	// Apply to our own chain under the shared lock.
	r.chainMu.Lock()
	// Check tip hasn't changed while we were reasoning.
	if r.chain.Tip.Hash() != tip.Hash() {
		r.chainMu.Unlock()
		log.Printf("reasoner: block %d stale, tip changed", height)
		return
	}

	if err := r.chain.AddBlockUnchecked(blk); err != nil {
		r.chainMu.Unlock()
		log.Printf("reasoner: apply block %d: %v", height, err)
		return
	}

	if err := r.store.SaveBlock(blk, height); err != nil {
		log.Printf("reasoner: save block %d: %v", height, err)
	}
	tipHash := blk.Header.Hash()
	r.store.SaveChainTip(storage.ChainTip{Hash: tipHash, Height: height})
	r.server.SetBlockHeight(height)
	r.server.Mempool().RemoveConfirmed(blk.Transactions)
	r.chainMu.Unlock()

	log.Printf("reasoner: mined block %d hash=%x", height, tipHash[:8])

	// Broadcast to peers.
	payload, err := network.EncodeBlock(blk)
	if err != nil {
		log.Printf("reasoner: encode block %d for broadcast: %v", height, err)
		return
	}
	r.server.BroadcastMessage(&network.MsgBlock{Payload: payload})
}

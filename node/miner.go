// Package node provides the mining loop and JSON-RPC server for the NOUS daemon.
package node

import (
	"log"
	"sync"
	"time"

	"github.com/nous-chain/nous/block"
	"github.com/nous-chain/nous/consensus"
	"github.com/nous-chain/nous/crypto"
	"github.com/nous-chain/nous/network"
	"github.com/nous-chain/nous/storage"
	"github.com/nous-chain/nous/tx"
)

// MaxBlockTxs is the maximum number of mempool transactions per block.
const MaxBlockTxs = 500

// Miner runs the mining loop in a background goroutine.
type Miner struct {
	chain   *consensus.ChainState
	server  *network.Server
	store   *storage.BlockStore
	privKey *crypto.PrivateKey
	pubKey  *crypto.PublicKey
	solver  consensus.CSPSolver

	mu      sync.Mutex
	running bool
	quit    chan struct{}
	done    chan struct{}
}

// NewMiner creates a new miner.
func NewMiner(
	chain *consensus.ChainState,
	server *network.Server,
	store *storage.BlockStore,
	privKey *crypto.PrivateKey,
	pubKey *crypto.PublicKey,
) *Miner {
	return &Miner{
		chain:   chain,
		server:  server,
		store:   store,
		privKey: privKey,
		pubKey:  pubKey,
	}
}

// SetSolver configures the CSP solver used during mining.
func (m *Miner) SetSolver(s consensus.CSPSolver) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.solver = s
}

// Start begins the mining loop in a background goroutine.
func (m *Miner) Start() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running {
		return
	}
	m.running = true
	m.quit = make(chan struct{})
	m.done = make(chan struct{})
	go m.loop()
}

// Stop signals the mining loop to stop and waits for it to finish.
func (m *Miner) Stop() {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return
	}
	m.running = false
	close(m.quit)
	m.mu.Unlock()
	<-m.done
}

// IsRunning returns whether the miner is active.
func (m *Miner) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

// ApplyBlock adds an externally received block to the chain state and store.
func (m *Miner) ApplyBlock(blk *block.Block) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	newHeight := m.chain.Height + 1
	if err := m.chain.AddBlock(blk); err != nil {
		return err
	}
	if err := m.store.SaveBlock(blk, newHeight); err != nil {
		log.Printf("miner: save block %d: %v", newHeight, err)
	}
	tipHash := blk.Header.Hash()
	m.store.SaveChainTip(storage.ChainTip{Hash: tipHash, Height: newHeight})
	m.server.SetBlockHeight(newHeight)
	m.server.Mempool().RemoveConfirmed(blk.Transactions)
	return nil
}

// Chain returns the chain state (for RPC queries).
func (m *Miner) Chain() *consensus.ChainState {
	return m.chain
}

func (m *Miner) loop() {
	defer close(m.done)
	log.Println("miner: started")

	for {
		select {
		case <-m.quit:
			log.Println("miner: stopped")
			return
		default:
		}

		m.mineOne()

		// Brief pause between mining attempts.
		select {
		case <-m.quit:
			return
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (m *Miner) mineOne() {
	m.mu.Lock()
	tip := m.chain.Tip
	height := m.chain.Height + 1
	diff := m.chain.GetDifficulty()
	m.mu.Unlock()

	// Gather mempool transactions.
	mempoolTxs := m.server.Mempool().GetTopN(MaxBlockTxs)

	// Filter valid transactions against current UTXO set.
	var validTxs []*tx.Transaction
	for _, t := range mempoolTxs {
		if err := tx.ValidateTransaction(t, m.chain.UTXOSet, height); err == nil {
			validTxs = append(validTxs, t)
		}
	}

	log.Printf("miner: mining block %d (%d txs)...", height, len(validTxs))

	blk, err := consensus.MineBlock(tip, validTxs, m.privKey, m.pubKey, diff, height, m.solver, m.chain.UTXOSet)
	if err != nil {
		log.Printf("miner: block %d failed: %v", height, err)
		return
	}

	// Apply to our own chain.
	m.mu.Lock()
	// Check tip hasn't changed while we were mining.
	if m.chain.Tip.Hash() != tip.Hash() {
		m.mu.Unlock()
		log.Printf("miner: block %d stale, tip changed", height)
		return
	}

	if err := m.chain.AddBlockUnchecked(blk); err != nil {
		m.mu.Unlock()
		log.Printf("miner: apply block %d: %v", height, err)
		return
	}

	if err := m.store.SaveBlock(blk, height); err != nil {
		log.Printf("miner: save block %d: %v", height, err)
	}
	tipHash := blk.Header.Hash()
	m.store.SaveChainTip(storage.ChainTip{Hash: tipHash, Height: height})
	m.server.SetBlockHeight(height)
	m.server.Mempool().RemoveConfirmed(blk.Transactions)
	m.mu.Unlock()

	log.Printf("miner: mined block %d hash=%x", height, tipHash[:8])

	// Broadcast to peers (gob-encoded full block, matching sync.go's DecodeBlock).
	payload, err := network.EncodeBlock(blk)
	if err != nil {
		log.Printf("miner: encode block %d for broadcast: %v", height, err)
		return
	}
	m.server.BroadcastMessage(&network.MsgBlock{Payload: payload})
}

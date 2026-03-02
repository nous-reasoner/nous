package network

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"nous/block"
	"nous/crypto"
)

// MaxBlocksPerInv is the maximum number of block hashes in a single inv message.
const MaxBlocksPerInv = 500

// Orphan pool limits.
const (
	MaxOrphanBlocks = 500
	OrphanExpiry    = time.Hour
)

// SyncState tracks the current synchronization status.
type SyncState int

const (
	SyncIdle       SyncState = iota
	SyncInProgress           // actively downloading blocks
	SyncComplete             // caught up with best peer
)

// ChainAccess provides the network layer with read/write access to the chain.
// Implemented by the node layer to avoid circular imports.
type ChainAccess interface {
	// Height returns the current chain height.
	Height() uint64
	// TipHash returns the hash of the current chain tip.
	TipHash() crypto.Hash
	// HasBlock checks if a block hash is known.
	HasBlock(hash crypto.Hash) bool
	// GetBlockByHeight returns a block at the given height.
	GetBlockByHeight(height uint64) (*block.Block, error)
	// GetBlockHashByHeight returns the block hash at the given height.
	GetBlockHashByHeight(height uint64) (crypto.Hash, error)
	// GetBlockByHash returns a block by its hash.
	GetBlockByHash(hash crypto.Hash) (*block.Block, error)
	// AddBlock validates and adds a block to the chain. Returns the new height.
	AddBlock(blk *block.Block) (uint64, error)
}

// orphanBlock holds a block whose parent is not yet known.
type orphanBlock struct {
	Block      *block.Block
	Hash       crypto.Hash
	ParentHash crypto.Hash
	AddedAt    time.Time
}

// BlockSyncer manages the block synchronization protocol.
type BlockSyncer struct {
	server *Server
	chain  ChainAccess

	mu        sync.Mutex
	state     SyncState
	syncPeer  *Peer
	pending   map[crypto.Hash]bool // blocks we've requested but not yet received
	blockChan chan *block.Block    // received blocks waiting to be processed

	// Orphan pool: blocks whose parents are not yet known.
	orphans        map[crypto.Hash]*orphanBlock             // block hash → orphan
	orphanByParent map[crypto.Hash]map[crypto.Hash]struct{} // parent hash → set of orphan hashes
}

// NewBlockSyncer creates a new block syncer.
func NewBlockSyncer(server *Server, chain ChainAccess) *BlockSyncer {
	return &BlockSyncer{
		server:         server,
		chain:          chain,
		state:          SyncIdle,
		pending:        make(map[crypto.Hash]bool),
		blockChan:      make(chan *block.Block, 64),
		orphans:        make(map[crypto.Hash]*orphanBlock),
		orphanByParent: make(map[crypto.Hash]map[crypto.Hash]struct{}),
	}
}

// State returns the current sync state.
func (bs *BlockSyncer) State() SyncState {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	return bs.state
}

// Start registers message handlers and begins sync if needed.
func (bs *BlockSyncer) Start() {
	bs.server.OnMessage(CmdGetBlocks, bs.handleGetBlocks)
	bs.server.OnMessage(CmdGetData, bs.handleGetData)
	bs.server.OnMessage(CmdBlock, bs.handleBlock)
	bs.server.OnMessage(CmdTx, bs.handleTx)
	bs.server.OnMessage(CmdInv, bs.handleInv)
}

// SyncFromPeer initiates block download from a specific peer.
func (bs *BlockSyncer) SyncFromPeer(peer *Peer) error {
	bs.mu.Lock()
	if bs.state == SyncInProgress {
		bs.mu.Unlock()
		return errors.New("sync: already in progress")
	}
	bs.state = SyncInProgress
	bs.syncPeer = peer
	bs.mu.Unlock()

	log.Printf("sync: starting from peer %s (height %d, our height %d)",
		peer.Addr, peer.BlockHeight, bs.chain.Height())

	// Send getblocks starting from our tip.
	tipHash := bs.chain.TipHash()
	peer.SendMessage(bs.server.config.Magic, &MsgGetBlocks{
		StartHash: tipHash,
		StopHash:  crypto.Hash{}, // get as many as possible
	})

	return nil
}

// TriggerSync finds a peer and starts syncing if we're not already syncing.
// We always attempt getblocks since peer.BlockHeight may be stale.
func (bs *BlockSyncer) TriggerSync() {
	bs.mu.Lock()
	if bs.state == SyncInProgress {
		// Check if the sync peer is still connected.
		if bs.syncPeer != nil && bs.server.peers.Get(bs.syncPeer.Addr) == nil {
			log.Printf("sync: peer %s disconnected, resetting sync state", bs.syncPeer.Addr)
			bs.state = SyncIdle
			bs.syncPeer = nil
		} else {
			bs.mu.Unlock()
			return
		}
	}
	bs.mu.Unlock()

	best := bs.server.peers.BestPeer()
	if best == nil {
		return
	}
	if !best.Handshaked {
		return
	}
	bs.SyncFromPeer(best)
}

// --- message handlers ---

// handleGetBlocks responds with an inv message containing block hashes
// starting after the requested StartHash.
func (bs *BlockSyncer) handleGetBlocks(peer *Peer, msg Message) {
	gb := msg.(*MsgGetBlocks)

	// Find the start height. Walk our chain to find the StartHash.
	startHeight := uint64(0)
	ourHeight := bs.chain.Height()

	for h := uint64(0); h <= ourHeight; h++ {
		hash, err := bs.chain.GetBlockHashByHeight(h)
		if err != nil {
			continue
		}
		if hash == gb.StartHash {
			startHeight = h + 1 // start AFTER the known block
			break
		}
	}

	// Build inv list from startHeight.
	var items []InvItem
	for h := startHeight; h <= ourHeight && len(items) < MaxBlocksPerInv; h++ {
		hash, err := bs.chain.GetBlockHashByHeight(h)
		if err != nil {
			break
		}
		if gb.StopHash != (crypto.Hash{}) && hash == gb.StopHash {
			break
		}
		items = append(items, InvItem{Type: InvTypeBlock, Hash: hash})
	}

	// Always send inv, even if empty. An empty inv lets the requester
	// transition to SyncComplete in handleInv.
	peer.SendMessage(bs.server.config.Magic, &MsgInv{Items: items})
}

// handleInv processes inventory announcements. Requests any blocks we don't have.
func (bs *BlockSyncer) handleInv(peer *Peer, msg Message) {
	inv := msg.(*MsgInv)

	var needed []InvItem
	for _, item := range inv.Items {
		if item.Type == InvTypeBlock && !bs.chain.HasBlock(item.Hash) {
			bs.mu.Lock()
			bs.pending[item.Hash] = true
			bs.mu.Unlock()
			needed = append(needed, item)
		}
	}

	if len(needed) > 0 {
		peer.SendMessage(bs.server.config.Magic, &MsgGetData{Items: needed})
	} else {
		// No new blocks — sync is complete.
		bs.mu.Lock()
		if bs.state == SyncInProgress {
			bs.state = SyncComplete
			log.Printf("sync: complete at height %d", bs.chain.Height())
		}
		bs.mu.Unlock()
	}
}

// handleGetData responds with the requested blocks or transactions.
func (bs *BlockSyncer) handleGetData(peer *Peer, msg Message) {
	gd := msg.(*MsgGetData)

	for _, item := range gd.Items {
		switch item.Type {
		case InvTypeBlock:
			bs.sendBlockByHash(peer, item.Hash)
		case InvTypeTx:
			bs.sendTx(peer, item.Hash)
		}
	}
}

func (bs *BlockSyncer) sendBlockByHash(peer *Peer, hash crypto.Hash) {
	blk, err := bs.chain.GetBlockByHash(hash)
	if err != nil {
		return
	}
	payload, err := EncodeBlock(blk)
	if err != nil {
		return
	}
	peer.SendMessage(bs.server.config.Magic, &MsgBlock{Payload: payload})
}

func (bs *BlockSyncer) sendTx(peer *Peer, hash crypto.Hash) {
	t := bs.server.mempool.Get(hash)
	if t == nil {
		return
	}
	peer.SendMessage(bs.server.config.Magic, &MsgTx{Payload: t.Serialize()})
}

// handleBlock processes a received block.
func (bs *BlockSyncer) handleBlock(peer *Peer, msg Message) {
	blkMsg := msg.(*MsgBlock)

	blk, err := DecodeBlock(blkMsg.Payload)
	if err != nil {
		log.Printf("sync: decode block from %s: %v", peer.Addr, err)
		return
	}

	blockHash := blk.Header.Hash()

	// Remove from pending.
	bs.mu.Lock()
	delete(bs.pending, blockHash)
	pendingCount := len(bs.pending)
	bs.mu.Unlock()

	// Add to chain.
	accepted := false
	newHeight, err := bs.chain.AddBlock(blk)
	if err != nil {
		// If the block's parent is unknown, store it as an orphan.
		if strings.Contains(err.Error(), "orphan") {
			bs.addOrphan(blk, peer)
		} else {
			log.Printf("sync: reject block %x from %s: %v", blockHash[:8], peer.Addr, err)
		}
	} else {
		accepted = true
		log.Printf("sync: accepted block %x at height %d from %s", blockHash[:8], newHeight, peer.Addr)

		// Update server's advertised height.
		bs.server.SetBlockHeight(newHeight)

		// Remove confirmed transactions from mempool.
		bs.server.mempool.RemoveConfirmed(blk.Transactions)

		// Process any orphan blocks waiting for this parent.
		bs.processOrphans(blockHash, peer)
	}

	// If we were syncing and have no more pending blocks, request more or finish.
	bs.mu.Lock()
	syncing := bs.state == SyncInProgress
	bs.mu.Unlock()

	if syncing && pendingCount == 0 {
		// Always request more blocks — peer.BlockHeight may be stale.
		// If there are no new blocks, we'll get an empty inv and mark sync complete.
		tipHash := bs.chain.TipHash()
		peer.SendMessage(bs.server.config.Magic, &MsgGetBlocks{
			StartHash: tipHash,
			StopHash:  crypto.Hash{},
		})
	}

	// Relay accepted blocks to other peers (not the sender).
	if accepted {
		bs.relayBlock(peer, blkMsg)
	}
}

// addOrphan stores a block whose parent is not yet known and requests the parent.
func (bs *BlockSyncer) addOrphan(blk *block.Block, peer *Peer) {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	hash := blk.Header.Hash()
	parentHash := blk.Header.PrevBlockHash

	// Already in orphan pool?
	if _, exists := bs.orphans[hash]; exists {
		return
	}

	// Evict expired orphans first.
	bs.evictExpiredOrphansLocked()

	// If pool is still full, drop the oldest orphan.
	if len(bs.orphans) >= MaxOrphanBlocks {
		bs.evictOldestOrphanLocked()
	}

	bs.orphans[hash] = &orphanBlock{
		Block:      blk,
		Hash:       hash,
		ParentHash: parentHash,
		AddedAt:    time.Now(),
	}
	if bs.orphanByParent[parentHash] == nil {
		bs.orphanByParent[parentHash] = make(map[crypto.Hash]struct{})
	}
	bs.orphanByParent[parentHash][hash] = struct{}{}

	log.Printf("sync: orphan block %x (parent %x) stored (%d orphans)",
		hash[:8], parentHash[:8], len(bs.orphans))

	// Request the missing parent from the peer, unless it's already pending.
	if !bs.pending[parentHash] {
		peer.SendMessage(bs.server.config.Magic, &MsgGetData{
			Items: []InvItem{{Type: InvTypeBlock, Hash: parentHash}},
		})
	}
}

// processOrphans tries to accept orphan blocks that depend on the accepted block.
func (bs *BlockSyncer) processOrphans(acceptedHash crypto.Hash, peer *Peer) {
	bs.mu.Lock()
	children, exists := bs.orphanByParent[acceptedHash]
	if !exists || len(children) == 0 {
		bs.mu.Unlock()
		return
	}
	// Collect orphans to process.
	toProcess := make([]*orphanBlock, 0, len(children))
	for childHash := range children {
		if orphan, ok := bs.orphans[childHash]; ok {
			toProcess = append(toProcess, orphan)
		}
	}
	// Remove from orphan maps before processing (avoid re-entry issues).
	delete(bs.orphanByParent, acceptedHash)
	for _, o := range toProcess {
		delete(bs.orphans, o.Hash)
	}
	bs.mu.Unlock()

	// Try to add each orphan.
	for _, o := range toProcess {
		newHeight, err := bs.chain.AddBlock(o.Block)
		if err != nil {
			log.Printf("sync: orphan block %x still invalid: %v", o.Hash[:8], err)
			continue
		}
		log.Printf("sync: accepted orphan block %x at height %d", o.Hash[:8], newHeight)
		bs.server.SetBlockHeight(newHeight)
		bs.server.mempool.RemoveConfirmed(o.Block.Transactions)
		// Recursively process orphans that depend on this newly accepted block.
		bs.processOrphans(o.Hash, peer)
	}
}

func (bs *BlockSyncer) evictExpiredOrphansLocked() {
	now := time.Now()
	for hash, o := range bs.orphans {
		if now.Sub(o.AddedAt) > OrphanExpiry {
			bs.removeOrphanLocked(hash)
		}
	}
}

func (bs *BlockSyncer) evictOldestOrphanLocked() {
	var oldest *orphanBlock
	for _, o := range bs.orphans {
		if oldest == nil || o.AddedAt.Before(oldest.AddedAt) {
			oldest = o
		}
	}
	if oldest != nil {
		bs.removeOrphanLocked(oldest.Hash)
	}
}

func (bs *BlockSyncer) removeOrphanLocked(hash crypto.Hash) {
	o, ok := bs.orphans[hash]
	if !ok {
		return
	}
	delete(bs.orphans, hash)
	if children, exists := bs.orphanByParent[o.ParentHash]; exists {
		delete(children, hash)
		if len(children) == 0 {
			delete(bs.orphanByParent, o.ParentHash)
		}
	}
}

// handleTx processes a received transaction.
func (bs *BlockSyncer) handleTx(peer *Peer, msg Message) {
	txMsg := msg.(*MsgTx)
	// For now, just add raw tx to mempool via deserialization.
	// Full validation would require the UTXO set which lives in consensus.
	_ = txMsg
	// TODO: deserialize tx, validate, add to mempool, relay
}

// relayBlock forwards a block to all peers except the sender.
func (bs *BlockSyncer) relayBlock(sender *Peer, blkMsg *MsgBlock) {
	for _, p := range bs.server.peers.All() {
		if p.Addr != sender.Addr && p.Handshaked {
			p.SendMessage(bs.server.config.Magic, blkMsg)
		}
	}
}

// BroadcastBlock encodes and broadcasts a new block to all peers.
func (bs *BlockSyncer) BroadcastBlock(blk *block.Block) error {
	payload, err := EncodeBlock(blk)
	if err != nil {
		return err
	}
	bs.server.BroadcastMessage(&MsgBlock{Payload: payload})
	return nil
}

// --- block serialization helpers ---

// EncodeBlock serializes a block to bytes using gob encoding.
func EncodeBlock(blk *block.Block) ([]byte, error) {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(blk); err != nil {
		return nil, fmt.Errorf("encode block: %w", err)
	}
	return buf.Bytes(), nil
}

// DecodeBlock deserializes a block from gob-encoded bytes.
// It rejects payloads larger than MaxPayloadSize and validates the decoded
// block has a sane number of transactions to prevent OOM from crafted data.
func DecodeBlock(data []byte) (*block.Block, error) {
	if len(data) > MaxPayloadSize {
		return nil, fmt.Errorf("decode block: data size %d exceeds max %d", len(data), MaxPayloadSize)
	}
	var blk block.Block
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&blk); err != nil {
		return nil, fmt.Errorf("decode block: %w", err)
	}
	if len(blk.Transactions) > block.MaxBlockTransactions {
		return nil, fmt.Errorf("decode block: tx count %d exceeds max %d",
			len(blk.Transactions), block.MaxBlockTransactions)
	}
	return &blk, nil
}

// WaitForSync blocks until sync reaches the target height or times out.
func (bs *BlockSyncer) WaitForSync(targetHeight uint64, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if bs.chain.Height() >= targetHeight {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("sync: timeout waiting for height %d (at %d)", targetHeight, bs.chain.Height())
}

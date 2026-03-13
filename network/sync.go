package network

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"nous/block"
	"nous/consensus"
	"nous/crypto"
	"nous/tx"
)

// MaxBlocksPerInv is the maximum number of block hashes in a single inv message.
const MaxBlocksPerInv = 500

// Orphan pool limits.
const (
	MaxOrphanBlocks = 500
	OrphanExpiry    = time.Hour
)

// Stale sync detection.
const (
	SyncStaleTimeout   = 120 * time.Second // reset sync if no progress for this long
	PendingExpiry      = 120 * time.Second // drop pending entries older than this
	OrphanRetryPeriod  = 30 * time.Second  // retry getblocks if orphans pile up with no height gain
	MaxOrphanRetries   = 3                 // switch peer after this many orphan retries without progress
	BadPeerCooldown    = 10 * time.Minute  // ignore bad sync peer for this long
)

// SyncState tracks the current synchronization status.
type SyncState int

const (
	SyncIdle          SyncState = iota
	SyncInProgress              // actively downloading blocks (v1 legacy path)
	SyncComplete                // caught up with best peer
	SyncHeadersPhase            // v2: downloading headers from best peer
	SyncBlocksPhase             // v2: parallel block download using header chain
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
	// ValidateTx validates a transaction against the current UTXO set.
	ValidateTx(txn *tx.Transaction) error
}

// orphanBlock holds a block whose parent is not yet known.
type orphanBlock struct {
	Block      *block.Block
	Hash       crypto.Hash
	ParentHash crypto.Hash
	AddedAt    time.Time
}

// pendingEntry tracks when a block hash was added to the pending set.
type pendingEntry struct {
	AddedAt time.Time
}

// BlockSyncer manages the block synchronization protocol.
type BlockSyncer struct {
	server *Server
	chain  ChainAccess

	mu              sync.Mutex
	state           SyncState
	syncPeer        *Peer
	pending         map[crypto.Hash]*pendingEntry // blocks we've requested but not yet received
	blockChan       chan *block.Block             // received blocks waiting to be processed
	syncCompletedAt time.Time                     // when last sync completed (cooldown)
	syncStartedAt   time.Time                     // when current sync started
	lastProgressAt  time.Time                     // last time chain height increased during sync
	lastProgressH   uint64                        // chain height at lastProgressAt
	lastOrphanRetry  time.Time // rate-limit orphan retry getblocks
	orphanRetryCount int       // consecutive orphan retries without progress

	// Orphan pool: blocks whose parents are not yet known.
	orphans        map[crypto.Hash]*orphanBlock             // block hash → orphan
	orphanByParent map[crypto.Hash]map[crypto.Hash]struct{} // parent hash → set of orphan hashes

	// Bad sync peers: temporarily skip peers that send too many orphans.
	badSyncPeers map[string]time.Time // peer addr → when marked bad

	// --- Headers-first sync (v2) ---
	headerChain       []headerEntry          // verified header skeleton from phase 1
	headerPeer        *Peer                  // peer serving headers
	headerRetries     int                    // consecutive header failures
	lastHeaderAt      time.Time              // last header progress
	// Parallel block download (phase 2)
	downloadQueue     []crypto.Hash          // block hashes to download, in order
	downloadNext      int                    // next index in downloadQueue to assign
	activeChunks      map[string]*chunkInfo  // peer addr → active chunk
	blockBuffer       map[uint64]*block.Block // height → downloaded block waiting for in-order processing
	blockBufferIdx    map[crypto.Hash]uint64 // hash → height (reverse lookup)
	nextProcessHeight uint64                 // next height to add to chain
	downloadBaseHeight uint64                // height of downloadQueue[0]
}

// headerEntry is a verified header from the headers-first phase.
type headerEntry struct {
	Hash   crypto.Hash
	Header block.Header
}

// chunkInfo tracks a block download chunk assigned to a peer.
type chunkInfo struct {
	StartIdx  int       // index into downloadQueue
	EndIdx    int       // exclusive end index
	AssignedAt time.Time
}

// NewBlockSyncer creates a new block syncer.
func NewBlockSyncer(server *Server, chain ChainAccess) *BlockSyncer {
	now := time.Now()
	return &BlockSyncer{
		server:         server,
		chain:          chain,
		state:          SyncIdle,
		pending:        make(map[crypto.Hash]*pendingEntry),
		blockChan:      make(chan *block.Block, 64),
		orphans:        make(map[crypto.Hash]*orphanBlock),
		orphanByParent: make(map[crypto.Hash]map[crypto.Hash]struct{}),
		badSyncPeers:   make(map[string]time.Time),
		lastProgressAt: now,
		lastProgressH:  chain.Height(),
		activeChunks:   make(map[string]*chunkInfo),
		blockBuffer:    make(map[uint64]*block.Block),
		blockBufferIdx: make(map[crypto.Hash]uint64),
	}
}

// State returns the current sync state.
func (bs *BlockSyncer) State() SyncState {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	return bs.state
}

// syncPeerSafe returns the current sync peer (nil if none).
func (bs *BlockSyncer) syncPeerSafe() *Peer {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	return bs.syncPeer
}

// Start registers message handlers and begins sync if needed.
func (bs *BlockSyncer) Start() {
	bs.server.OnMessage(CmdGetBlocks, bs.handleGetBlocks)
	bs.server.OnMessage(CmdGetData, bs.handleGetData)
	bs.server.OnMessage(CmdBlock, bs.handleBlock)
	bs.server.OnMessage(CmdTx, bs.handleTx)
	bs.server.OnMessage(CmdInv, bs.handleInv)
	bs.server.OnMessage(CmdGetHeaders, bs.handleGetHeaders)
	bs.server.OnMessage(CmdHeaders, bs.handleHeaders)
}

// SyncFromPeer initiates block download from a specific peer.
func (bs *BlockSyncer) SyncFromPeer(peer *Peer) error {
	bs.mu.Lock()
	if bs.state == SyncInProgress {
		bs.mu.Unlock()
		return errors.New("sync: already in progress")
	}
	now := time.Now()
	bs.state = SyncInProgress
	bs.syncPeer = peer
	bs.syncStartedAt = now
	bs.lastProgressAt = now
	bs.lastProgressH = bs.chain.Height()
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
// After sync completes, a 30-second cooldown prevents redundant polling;
// new blocks arrive via relay during this period.
//
// Also detects stale sync sessions: if no chain-height progress has been
// made for SyncStaleTimeout while in SyncInProgress, the session is reset
// so a fresh getblocks can be sent.
func (bs *BlockSyncer) TriggerSync() {
	bs.mu.Lock()

	// Handle active v1 sync (SyncInProgress).
	if bs.state == SyncInProgress {
		if bs.syncPeer != nil && bs.server.peers.Get(bs.syncPeer.Addr) == nil {
			log.Printf("sync: peer %s disconnected, resetting sync state", bs.syncPeer.Addr)
			bs.state = SyncIdle
			bs.syncPeer = nil
			bs.evictStalePendingLocked()
		} else {
			currentH := bs.chain.Height()
			if currentH > bs.lastProgressH {
				bs.lastProgressH = currentH
				bs.lastProgressAt = time.Now()
			}
			if time.Since(bs.lastProgressAt) > SyncStaleTimeout {
				log.Printf("sync: stale — no progress for %s (height %d, %d orphans, %d pending), resetting",
					SyncStaleTimeout, currentH, len(bs.orphans), len(bs.pending))
				bs.state = SyncIdle
				bs.syncPeer = nil
				bs.evictStalePendingLocked()
				bs.clearOrphansLocked()
			} else if len(bs.orphans) > 0 && time.Since(bs.lastProgressAt) > OrphanRetryPeriod && time.Since(bs.lastOrphanRetry) > OrphanRetryPeriod {
				bs.orphanRetryCount++
				bs.lastOrphanRetry = time.Now()

				if bs.orphanRetryCount >= MaxOrphanRetries {
					badAddr := ""
					if bs.syncPeer != nil {
						badAddr = bs.syncPeer.Addr
						bs.badSyncPeers[badAddr] = time.Now()
					}
					log.Printf("sync: %d orphans after %d retries from %s, switching peer",
						len(bs.orphans), bs.orphanRetryCount, badAddr)
					bs.state = SyncIdle
					bs.syncPeer = nil
					bs.orphanRetryCount = 0
					bs.evictStalePendingLocked()
					bs.clearOrphansLocked()
				} else {
					peer := bs.syncPeer
					bs.evictStalePendingLocked()
					bs.mu.Unlock()
					if peer != nil {
						tipHash := bs.chain.TipHash()
						log.Printf("sync: %d orphans with no progress for %s, retrying getblocks from tip (attempt %d/%d)",
							len(bs.orphans), OrphanRetryPeriod, bs.orphanRetryCount, MaxOrphanRetries)
						peer.SendMessage(bs.server.config.Magic, &MsgGetBlocks{
							StartHash: tipHash,
							StopHash:  crypto.Hash{},
						})
					}
					return
				}
			} else {
				bs.mu.Unlock()
				return
			}
		}
	}

	// Handle active v2 headers phase.
	if bs.state == SyncHeadersPhase {
		if bs.headerPeer != nil && bs.server.peers.Get(bs.headerPeer.Addr) == nil {
			log.Printf("sync: header peer %s disconnected, resetting", bs.headerPeer.Addr)
			bs.resetHeadersSyncLocked()
		} else if time.Since(bs.lastHeaderAt) > HeadersStaleTimeout {
			bs.headerRetries++
			if bs.headerRetries >= MaxHeaderRetries {
				badAddr := ""
				if bs.headerPeer != nil {
					badAddr = bs.headerPeer.Addr
					bs.badSyncPeers[badAddr] = time.Now()
				}
				log.Printf("sync: headers stale after %d retries from %s, switching peer",
					bs.headerRetries, badAddr)
				bs.resetHeadersSyncLocked()
			} else {
				// Retry with same peer.
				peer := bs.headerPeer
				bs.lastHeaderAt = time.Now()
				bs.mu.Unlock()
				if peer != nil {
					lastHash := bs.chain.TipHash()
					if len(bs.headerChain) > 0 {
						lastHash = bs.headerChain[len(bs.headerChain)-1].Hash
					}
					log.Printf("sync: headers stale, retrying getheaders (attempt %d/%d)",
						bs.headerRetries, MaxHeaderRetries)
					peer.SendMessage(bs.server.config.Magic, &MsgGetHeaders{
						StartHash: lastHash,
						StopHash:  crypto.Hash{},
					})
				}
				return
			}
		} else {
			bs.mu.Unlock()
			return
		}
	}

	// Handle active v2 blocks phase.
	if bs.state == SyncBlocksPhase {
		bs.checkChunkTimeoutsLocked()
		if len(bs.downloadQueue) > 0 && bs.downloadNext < len(bs.downloadQueue) {
			// Still downloading — assign chunks to idle peers.
			bs.mu.Unlock()
			bs.assignChunks()
			return
		}
		// Check if all done.
		if bs.downloadNext >= len(bs.downloadQueue) && len(bs.activeChunks) == 0 && len(bs.blockBuffer) == 0 {
			bs.state = SyncComplete
			bs.syncCompletedAt = time.Now()
			log.Printf("sync: v2 complete at height %d", bs.chain.Height())
		}
		bs.mu.Unlock()
		return
	}

	// After sync completes, wait before re-polling.
	if bs.state == SyncComplete && time.Since(bs.syncCompletedAt) < 30*time.Second {
		bs.mu.Unlock()
		return
	}
	bs.mu.Unlock()

	// Clean up expired bad peer entries.
	bs.mu.Lock()
	for addr, t := range bs.badSyncPeers {
		if time.Since(t) > BadPeerCooldown {
			delete(bs.badSyncPeers, addr)
		}
	}
	badPeers := make(map[string]bool, len(bs.badSyncPeers))
	for addr := range bs.badSyncPeers {
		badPeers[addr] = true
	}
	bs.mu.Unlock()

	// Select best peer, skipping bad ones.
	var best *Peer
	for _, p := range bs.server.peers.All() {
		if !p.Handshaked || badPeers[p.Addr] {
			continue
		}
		if best == nil || p.BlockHeight > best.BlockHeight {
			best = p
		}
	}
	if best == nil {
		return
	}

	// If best peer supports v2, use headers-first sync.
	if best.Version >= 2 && best.BlockHeight > bs.chain.Height() {
		bs.startHeadersSync(best)
	} else {
		bs.SyncFromPeer(best)
	}
}

// resetHeadersSyncLocked resets headers-first sync state. Must hold bs.mu.
func (bs *BlockSyncer) resetHeadersSyncLocked() {
	bs.state = SyncIdle
	bs.headerPeer = nil
	bs.headerChain = nil
	bs.headerRetries = 0
	bs.downloadQueue = nil
	bs.downloadNext = 0
	bs.activeChunks = make(map[string]*chunkInfo)
	bs.blockBuffer = make(map[uint64]*block.Block)
	bs.blockBufferIdx = make(map[crypto.Hash]uint64)
}

// startHeadersSync begins the headers-first sync from a v2 peer.
func (bs *BlockSyncer) startHeadersSync(peer *Peer) {
	bs.mu.Lock()
	if bs.state != SyncIdle {
		bs.mu.Unlock()
		return
	}
	now := time.Now()
	bs.state = SyncHeadersPhase
	bs.headerPeer = peer
	bs.headerChain = nil
	bs.headerRetries = 0
	bs.lastHeaderAt = now
	bs.lastProgressAt = now
	bs.lastProgressH = bs.chain.Height()
	bs.mu.Unlock()

	log.Printf("sync: v2 headers-first starting from peer %s (height %d, our height %d)",
		peer.Addr, peer.BlockHeight, bs.chain.Height())

	tipHash := bs.chain.TipHash()
	peer.SendMessage(bs.server.config.Magic, &MsgGetHeaders{
		StartHash: tipHash,
		StopHash:  crypto.Hash{},
	})
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
	bs.mu.Lock()
	now := time.Now()
	for _, item := range inv.Items {
		if item.Type == InvTypeBlock && !bs.chain.HasBlock(item.Hash) {
			bs.pending[item.Hash] = &pendingEntry{AddedAt: now}
			needed = append(needed, item)
		}
	}
	bs.mu.Unlock()

	if len(needed) > 0 {
		peer.SendMessage(bs.server.config.Magic, &MsgGetData{Items: needed})
	} else if len(inv.Items) == 0 {
		// Peer sent an empty inv — no more blocks available. Sync is done.
		bs.mu.Lock()
		if bs.state == SyncInProgress {
			bs.state = SyncComplete
			bs.syncCompletedAt = time.Now()
			log.Printf("sync: complete at height %d", bs.chain.Height())
		}
		bs.mu.Unlock()
	}
	// else: inv had items but we already had them all — ignore
	// (duplicate response from a redundant getblocks).
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
		bs.server.protection.AddScore(peer.Addr, BanScoreBadMessage)
		return
	}

	blockHash := blk.Header.Hash()

	// Route to v2 handler if in parallel download phase.
	bs.mu.Lock()
	isBlocksPhase := bs.state == SyncBlocksPhase
	bs.mu.Unlock()
	if isBlocksPhase {
		bs.handleBlockV2(peer, blk, blockHash)
		return
	}

	// Skip blocks we already have (can arrive via relay + sync simultaneously).
	if bs.chain.HasBlock(blockHash) {
		bs.mu.Lock()
		_, wasPending := bs.pending[blockHash]
		delete(bs.pending, blockHash)
		pendingCount := len(bs.pending)
		syncing := bs.state == SyncInProgress
		bs.mu.Unlock()

		// Only request the next batch if this duplicate was actually pending
		// (a relay peer beat the sync peer to delivery). Without the wasPending
		// guard, every unsolicited relay duplicate at the end of a batch would
		// spam getblocks, flooding the sync peer and stalling sync.
		if wasPending && syncing && pendingCount == 0 {
			if sp := bs.syncPeerSafe(); sp != nil {
				tipHash := bs.chain.TipHash()
				sp.SendMessage(bs.server.config.Magic, &MsgGetBlocks{
					StartHash: tipHash,
					StopHash:  crypto.Hash{},
				})
			}
		}
		return
	}

	// Remove from pending. Track whether this block was actually requested
	// (part of a sync batch) vs. an unsolicited relay.
	bs.mu.Lock()
	_, wasBatchBlock := bs.pending[blockHash]
	delete(bs.pending, blockHash)
	pendingCount := len(bs.pending)
	bs.mu.Unlock()

	// Add to chain.
	accepted := false
	newHeight, err := bs.chain.AddBlock(blk)
	if err != nil {
		// If the block's parent is unknown, store it as an orphan.
		if errors.Is(err, consensus.ErrOrphanBlock) {
			bs.addOrphan(blk, peer)
		} else if errors.Is(err, consensus.ErrDuplicateBlock) {
			// Race between HasBlock check and AddBlock; silently ignore.
		} else {
			log.Printf("sync: reject block %x from %s: %v", blockHash[:8], peer.Addr, err)
			bs.server.protection.AddScore(peer.Addr, BanScoreInvalidBlock)
		}
	} else {
		accepted = true
		log.Printf("sync: accepted block %x at height %d from %s", blockHash[:8], newHeight, peer.Addr)

		// Track progress for stale-sync detection.
		bs.mu.Lock()
		bs.lastProgressH = newHeight
		bs.lastProgressAt = time.Now()
		bs.orphanRetryCount = 0 // reset on successful progress
		bs.mu.Unlock()

		// Update server's advertised height.
		bs.server.SetBlockHeight(newHeight)

		// Update peer's known height so getpeerinfo stays accurate.
		if newHeight > peer.BlockHeight {
			peer.BlockHeight = newHeight
		}

		// Remove confirmed transactions from mempool.
		bs.server.mempool.RemoveConfirmed(blk.Transactions)

		// Process any orphan blocks waiting for this parent.
		bs.processOrphans(blockHash, peer)
	}

	// If we were syncing and have no more pending blocks, request more or finish.
	bs.mu.Lock()
	syncing := bs.state == SyncInProgress
	bs.mu.Unlock()

	if syncing && wasBatchBlock && pendingCount == 0 {
		// Always request more blocks — peer.BlockHeight may be stale.
		// If there are no new blocks, we'll get an empty inv and mark sync complete.
		// Send to the sync peer (not the delivering peer, which may be a relay).
		if sp := bs.syncPeerSafe(); sp != nil {
			tipHash := bs.chain.TipHash()
			sp.SendMessage(bs.server.config.Magic, &MsgGetBlocks{
				StartHash: tipHash,
				StopHash:  crypto.Hash{},
			})
		}
	}

	// Relay accepted blocks to other peers (not the sender).
	if accepted {
		bs.relayBlock(peer, blkMsg)
	}
}

// addOrphan stores a block whose parent is not yet known and requests the parent.
func (bs *BlockSyncer) addOrphan(blk *block.Block, peer *Peer) {
	bs.mu.Lock()

	hash := blk.Header.Hash()
	parentHash := blk.Header.PrevBlockHash

	// Already in orphan pool?
	if _, exists := bs.orphans[hash]; exists {
		bs.mu.Unlock()
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

	needParent := bs.pending[parentHash] == nil

	log.Printf("sync: orphan block %x (parent %x) stored (%d orphans)",
		hash[:8], parentHash[:8], len(bs.orphans))

	bs.mu.Unlock()

	// Request the missing parent outside of the lock to avoid stalling
	// the sync protocol if SendMessage blocks.
	if needParent {
		peer.SendMessage(bs.server.config.Magic, &MsgGetData{
			Items: []InvItem{{Type: InvTypeBlock, Hash: parentHash}},
		})
	}
}

// processOrphans tries to accept orphan blocks that depend on the accepted block.
// Uses an iterative worklist to avoid unbounded recursion depth.
func (bs *BlockSyncer) processOrphans(acceptedHash crypto.Hash, peer *Peer) {
	worklist := []crypto.Hash{acceptedHash}

	for len(worklist) > 0 {
		current := worklist[0]
		worklist = worklist[1:]

		bs.mu.Lock()
		children, exists := bs.orphanByParent[current]
		if !exists || len(children) == 0 {
			bs.mu.Unlock()
			continue
		}
		// Collect orphans to process.
		toProcess := make([]*orphanBlock, 0, len(children))
		for childHash := range children {
			if orphan, ok := bs.orphans[childHash]; ok {
				toProcess = append(toProcess, orphan)
			}
		}
		// Remove from orphan maps before processing.
		delete(bs.orphanByParent, current)
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
			if newHeight > peer.BlockHeight {
				peer.BlockHeight = newHeight
			}
			bs.server.mempool.RemoveConfirmed(o.Block.Transactions)
			// Queue this block's hash so its children are processed next.
			worklist = append(worklist, o.Hash)
		}
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

// evictStalePendingLocked removes pending entries older than PendingExpiry.
// Must be called with bs.mu held.
func (bs *BlockSyncer) evictStalePendingLocked() {
	now := time.Now()
	for hash, entry := range bs.pending {
		if now.Sub(entry.AddedAt) > PendingExpiry {
			delete(bs.pending, hash)
		}
	}
}

// clearOrphansLocked drops all orphan blocks. Called during stale sync reset
// because orphans from relay peers at heights far above our tip cannot be
// resolved until we catch up; keeping them just wastes memory and pollutes
// future sync batches.
// Must be called with bs.mu held.
func (bs *BlockSyncer) clearOrphansLocked() {
	bs.orphans = make(map[crypto.Hash]*orphanBlock)
	bs.orphanByParent = make(map[crypto.Hash]map[crypto.Hash]struct{})
}

// handleTx processes a received transaction.
func (bs *BlockSyncer) handleTx(peer *Peer, msg Message) {
	txMsg := msg.(*MsgTx)

	// Deserialize the transaction.
	transaction, err := tx.Deserialize(txMsg.Payload)
	if err != nil {
		log.Printf("sync: invalid tx from %s: %v", peer.Addr, err)
		bs.server.protection.AddScore(peer.Addr, BanScoreBadMessage)
		return
	}

	txID := transaction.TxID()

	// Skip if already in mempool.
	if bs.server.mempool.Has(txID) {
		return
	}

	// Validate transaction against UTXO set before accepting into mempool.
	if err := bs.chain.ValidateTx(transaction); err != nil {
		log.Printf("sync: rejected tx %x from %s: %v", txID[:8], peer.Addr, err)
		bs.server.protection.AddScore(peer.Addr, BanScorePolicyTx)
		return
	}

	// Add to mempool.
	if !bs.server.mempool.Add(transaction) {
		return
	}

	log.Printf("sync: accepted tx %x from %s", txID[:8], peer.Addr)

	// Relay to other peers (exclude sender).
	for _, p := range bs.server.peers.All() {
		if p.Addr != peer.Addr && p.Handshaked {
			p.SendMessage(bs.server.config.Magic, txMsg)
		}
	}
}

// relayBlock forwards a block to all peers except the sender.
func (bs *BlockSyncer) relayBlock(sender *Peer, blkMsg *MsgBlock) {
	senderAddr := ""
	if sender != nil {
		senderAddr = sender.Addr
	}
	for _, p := range bs.server.peers.All() {
		if p.Addr != senderAddr && p.Handshaked {
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

// --- headers-first sync (v2) handlers ---

// handleGetHeaders responds with a headers message containing serialized block headers.
func (bs *BlockSyncer) handleGetHeaders(peer *Peer, msg Message) {
	gh := msg.(*MsgGetHeaders)

	// Find the start height by locating StartHash in our chain.
	startHeight := uint64(0)
	ourHeight := bs.chain.Height()

	for h := uint64(0); h <= ourHeight; h++ {
		hash, err := bs.chain.GetBlockHashByHeight(h)
		if err != nil {
			continue
		}
		if hash == gh.StartHash {
			startHeight = h + 1
			break
		}
	}

	// Build headers payload.
	var buf bytes.Buffer
	count := 0
	for h := startHeight; h <= ourHeight && count < MaxHeadersPerMsg; h++ {
		blk, err := bs.chain.GetBlockByHeight(h)
		if err != nil {
			break
		}
		if gh.StopHash != (crypto.Hash{}) && blk.Header.Hash() == gh.StopHash {
			break
		}
		buf.Write(blk.Header.Serialize())
		count++
	}

	peer.SendMessage(bs.server.config.Magic, &MsgHeaders{Headers: buf.Bytes()})
}

// handleHeaders processes received block headers during headers-first sync.
func (bs *BlockSyncer) handleHeaders(peer *Peer, msg Message) {
	hdrs := msg.(*MsgHeaders)

	bs.mu.Lock()
	if bs.state != SyncHeadersPhase || bs.headerPeer == nil || bs.headerPeer.Addr != peer.Addr {
		bs.mu.Unlock()
		return
	}

	data := hdrs.Headers
	headerCount := len(data) / block.HeaderSize
	if len(data)%block.HeaderSize != 0 || headerCount == 0 {
		// Empty headers = headers phase complete, transition to blocks phase.
		if len(data) == 0 {
			bs.mu.Unlock()
			bs.startBlocksPhase()
			return
		}
		log.Printf("sync: invalid headers payload size %d from %s", len(data), peer.Addr)
		bs.headerRetries++
		if bs.headerRetries >= MaxHeaderRetries {
			bs.badSyncPeers[peer.Addr] = time.Now()
			bs.resetHeadersSyncLocked()
		}
		bs.mu.Unlock()
		return
	}

	// Determine expected previous hash.
	var prevHash crypto.Hash
	if len(bs.headerChain) > 0 {
		prevHash = bs.headerChain[len(bs.headerChain)-1].Hash
	} else {
		prevHash = bs.chain.TipHash()
	}

	// Parse and verify header chain linkage.
	for i := 0; i < headerCount; i++ {
		hdrBytes := data[i*block.HeaderSize : (i+1)*block.HeaderSize]
		hdr, err := block.DeserializeHeader(hdrBytes)
		if err != nil {
			log.Printf("sync: malformed header %d from %s: %v", i, peer.Addr, err)
			bs.headerRetries++
			if bs.headerRetries >= MaxHeaderRetries {
				bs.badSyncPeers[peer.Addr] = time.Now()
				bs.resetHeadersSyncLocked()
			}
			bs.mu.Unlock()
			return
		}

		hash := hdr.Hash()

		// Verify chain linkage.
		if hdr.PrevBlockHash != prevHash {
			log.Printf("sync: header chain broken at %d from %s (expected parent %x, got %x)",
				i, peer.Addr, prevHash[:8], hdr.PrevBlockHash[:8])
			bs.headerRetries++
			if bs.headerRetries >= MaxHeaderRetries {
				bs.badSyncPeers[peer.Addr] = time.Now()
				bs.resetHeadersSyncLocked()
			}
			bs.mu.Unlock()
			return
		}

		bs.headerChain = append(bs.headerChain, headerEntry{Hash: hash, Header: *hdr})
		prevHash = hash
	}

	bs.lastHeaderAt = time.Now()
	bs.headerRetries = 0 // reset on success
	totalHeaders := len(bs.headerChain)
	bs.mu.Unlock()

	log.Printf("sync: received %d headers (total: %d) from %s", headerCount, totalHeaders, peer.Addr)

	// If we got a full batch, request more.
	if headerCount == MaxHeadersPerMsg {
		peer.SendMessage(bs.server.config.Magic, &MsgGetHeaders{
			StartHash: prevHash,
			StopHash:  crypto.Hash{},
		})
	} else {
		// Fewer than max = no more headers, transition to blocks phase.
		bs.startBlocksPhase()
	}
}

// startBlocksPhase transitions from headers to parallel block download.
func (bs *BlockSyncer) startBlocksPhase() {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	if len(bs.headerChain) == 0 {
		log.Printf("sync: no new headers, sync complete at height %d", bs.chain.Height())
		bs.state = SyncComplete
		bs.syncCompletedAt = time.Now()
		bs.headerPeer = nil
		return
	}

	// Build download queue from header chain.
	bs.downloadQueue = make([]crypto.Hash, len(bs.headerChain))
	for i, entry := range bs.headerChain {
		bs.downloadQueue[i] = entry.Hash
	}
	bs.downloadNext = 0
	bs.downloadBaseHeight = bs.chain.Height() + 1
	bs.nextProcessHeight = bs.downloadBaseHeight
	bs.activeChunks = make(map[string]*chunkInfo)
	bs.blockBuffer = make(map[uint64]*block.Block)
	bs.blockBufferIdx = make(map[crypto.Hash]uint64)
	bs.state = SyncBlocksPhase
	bs.lastProgressAt = time.Now()
	bs.lastProgressH = bs.chain.Height()

	log.Printf("sync: headers phase complete, %d blocks to download (height %d → %d)",
		len(bs.downloadQueue), bs.nextProcessHeight, bs.nextProcessHeight+uint64(len(bs.downloadQueue))-1)

	// Assign initial chunks (outside lock via goroutine since we hold mu).
	go bs.assignChunks()
}

// assignChunks assigns download chunks to available peers.
func (bs *BlockSyncer) assignChunks() {
	bs.mu.Lock()
	if bs.state != SyncBlocksPhase {
		bs.mu.Unlock()
		return
	}

	// Collect peers eligible for chunk assignment.
	busyPeers := make(map[string]bool)
	for addr := range bs.activeChunks {
		busyPeers[addr] = true
	}

	// Check buffer limit — pause assignment if buffer is full.
	if len(bs.blockBuffer) >= MaxBufferedBlocks {
		bs.mu.Unlock()
		return
	}

	badPeers := make(map[string]bool, len(bs.badSyncPeers))
	for addr := range bs.badSyncPeers {
		badPeers[addr] = true
	}
	bs.mu.Unlock()

	// Find idle peers.
	var idlePeers []*Peer
	for _, p := range bs.server.peers.All() {
		if p.Handshaked && !busyPeers[p.Addr] && !badPeers[p.Addr] {
			idlePeers = append(idlePeers, p)
		}
	}

	bs.mu.Lock()
	for _, peer := range idlePeers {
		if bs.downloadNext >= len(bs.downloadQueue) {
			break
		}
		if len(bs.blockBuffer) >= MaxBufferedBlocks {
			break
		}

		startIdx := bs.downloadNext
		endIdx := startIdx + BlocksPerChunk
		if endIdx > len(bs.downloadQueue) {
			endIdx = len(bs.downloadQueue)
		}

		// Build getdata items for this chunk.
		items := make([]InvItem, 0, endIdx-startIdx)
		for i := startIdx; i < endIdx; i++ {
			items = append(items, InvItem{Type: InvTypeBlock, Hash: bs.downloadQueue[i]})
		}

		bs.activeChunks[peer.Addr] = &chunkInfo{
			StartIdx:   startIdx,
			EndIdx:     endIdx,
			AssignedAt: time.Now(),
		}
		bs.downloadNext = endIdx

		// Send request outside of critical section concern — but peer.SendMessage is thread-safe.
		peer.SendMessage(bs.server.config.Magic, &MsgGetData{Items: items})

		log.Printf("sync: assigned chunk [%d..%d) to %s (%d blocks)",
			startIdx, endIdx, peer.Addr, endIdx-startIdx)
	}
	bs.mu.Unlock()
}

// checkChunkTimeoutsLocked checks for timed-out chunks and reassigns them.
// Must hold bs.mu.
func (bs *BlockSyncer) checkChunkTimeoutsLocked() {
	now := time.Now()
	for addr, chunk := range bs.activeChunks {
		if now.Sub(chunk.AssignedAt) > ChunkTimeout {
			log.Printf("sync: chunk [%d..%d) from %s timed out, reassigning",
				chunk.StartIdx, chunk.EndIdx, addr)
			// Rewind downloadNext to the first undelivered block in this chunk.
			for i := chunk.StartIdx; i < chunk.EndIdx; i++ {
				h := bs.downloadBaseHeight + uint64(i)
				if h < bs.nextProcessHeight {
					continue // already processed
				}
				if _, ok := bs.blockBuffer[h]; !ok {
					if i < bs.downloadNext {
						bs.downloadNext = i
					}
					break
				}
			}
			delete(bs.activeChunks, addr)
		}
	}
}

// handleBlockV2 processes a block received during parallel download phase.
// Called from handleBlock when state is SyncBlocksPhase.
func (bs *BlockSyncer) handleBlockV2(peer *Peer, blk *block.Block, blockHash crypto.Hash) {
	bs.mu.Lock()

	// Find this block's index in downloadQueue to compute expected height.
	idx := bs.indexOfHash(blockHash)
	if idx < 0 {
		// Not part of our download — might be a relay block. Process normally.
		bs.mu.Unlock()
		// Fall through to standard block processing for relay blocks.
		newHeight, err := bs.chain.AddBlock(blk)
		if err == nil {
			log.Printf("sync: v2 accepted relay block %x at height %d", blockHash[:8], newHeight)
			bs.server.SetBlockHeight(newHeight)
			bs.server.mempool.RemoveConfirmed(blk.Transactions)
		}
		return
	}

	expectedHeight := bs.downloadBaseHeight + uint64(idx)

	// Already processed?
	if expectedHeight < bs.nextProcessHeight {
		bs.mu.Unlock()
		return
	}

	// Store in buffer.
	bs.blockBuffer[expectedHeight] = blk
	bs.blockBufferIdx[blockHash] = expectedHeight

	// Check if peer's chunk is fully received.
	if chunk, ok := bs.activeChunks[peer.Addr]; ok {
		allReceived := true
		for i := chunk.StartIdx; i < chunk.EndIdx; i++ {
			h := bs.downloadBaseHeight + uint64(i)
			if h < bs.nextProcessHeight {
				continue // already processed
			}
			if _, ok := bs.blockBuffer[h]; !ok {
				allReceived = false
				break
			}
		}
		if allReceived {
			delete(bs.activeChunks, peer.Addr)
		}
	}

	bs.mu.Unlock()

	// Process buffered blocks in order.
	bs.processBlockBuffer()

	// Assign more chunks to idle peers.
	bs.assignChunks()
}

// indexOfHash returns the index in downloadQueue for the given hash, or -1.
func (bs *BlockSyncer) indexOfHash(hash crypto.Hash) int {
	for i, h := range bs.downloadQueue {
		if h == hash {
			return i
		}
	}
	return -1
}

// processBlockBuffer processes buffered blocks in height order.
func (bs *BlockSyncer) processBlockBuffer() {
	for {
		bs.mu.Lock()
		blk, ok := bs.blockBuffer[bs.nextProcessHeight]
		if !ok {
			bs.mu.Unlock()
			return
		}
		height := bs.nextProcessHeight
		delete(bs.blockBuffer, height)
		// Clean up reverse lookup.
		hash := blk.Header.Hash()
		delete(bs.blockBufferIdx, hash)
		bs.mu.Unlock()

		newHeight, err := bs.chain.AddBlock(blk)
		if err != nil {
			log.Printf("sync: v2 reject block %x at expected height %d: %v", hash[:8], height, err)
			// Block invalid — reset entire sync.
			bs.mu.Lock()
			bs.resetHeadersSyncLocked()
			bs.mu.Unlock()
			return
		}

		log.Printf("sync: v2 accepted block %x at height %d", hash[:8], newHeight)
		bs.server.SetBlockHeight(newHeight)
		bs.server.mempool.RemoveConfirmed(blk.Transactions)

		bs.mu.Lock()
		bs.nextProcessHeight = newHeight + 1
		bs.lastProgressH = newHeight
		bs.lastProgressAt = time.Now()
		bs.mu.Unlock()

		// Relay to other peers.
		payload, err := EncodeBlock(blk)
		if err == nil {
			bs.relayBlock(nil, &MsgBlock{Payload: payload})
		}
	}
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

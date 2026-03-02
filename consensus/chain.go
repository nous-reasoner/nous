package consensus

import (
	"errors"
	"fmt"
	"math/big"

	"nous/block"
	"nous/crypto"
	"nous/tx"
)

// blockNode represents a block in the block index tree.
type blockNode struct {
	Hash      crypto.Hash
	Height    uint64
	Header    block.Header
	Block     *block.Block // full block (needed for reorg apply)
	ChainWork *big.Int     // cumulative PoW work from genesis to this block
	Parent    *blockNode
}

// ancestor returns the ancestor at the given height, or nil.
func (n *blockNode) ancestor(height uint64) *blockNode {
	cur := n
	for cur != nil && cur.Height > height {
		cur = cur.Parent
	}
	if cur != nil && cur.Height == height {
		return cur
	}
	return nil
}

// ReorgCallback is called during a reorganization for each block that is
// disconnected from the main chain. The caller can use this to return
// transactions to the mempool.
type ReorgCallback func(disconnected *block.Block)

// ChainState holds the current state of the blockchain.
type ChainState struct {
	Tip        *block.Header
	Height     uint64
	UTXOSet    *tx.UTXOSet
	Difficulty *DifficultyParams
	// ASERT anchor point for per-block difficulty adjustment.
	Anchor *ASERTAnchor

	// Block index: all known blocks (main chain + side chains).
	blockIndex map[crypto.Hash]*blockNode
	tipNode    *blockNode
	// Undo data for each block on the main chain (keyed by block hash).
	undoMap map[crypto.Hash]*tx.UndoData
	// Optional callback invoked for each disconnected block during reorg.
	OnReorg ReorgCallback
}

// NewChainState creates a new chain state from a genesis block.
func NewChainState(genesis *block.Block) *ChainState {
	utxos := tx.NewUTXOSet()
	undo := utxos.ApplyBlockWithUndo(genesis.Transactions, 0)

	genesisHash := genesis.Header.Hash()
	genesisNode := &blockNode{
		Hash:      genesisHash,
		Height:    0,
		Header:    genesis.Header,
		Block:     genesis,
		ChainWork: big.NewInt(1), // genesis has minimal work
		Parent:    nil,
	}

	cs := &ChainState{
		Tip:        &genesis.Header,
		Height:     0,
		UTXOSet:    utxos,
		Difficulty: &DifficultyParams{PoWTarget: CompactToTarget(genesis.Header.DifficultyBits)},
		Anchor: &ASERTAnchor{
			Height:    0,
			Timestamp: genesis.Header.Timestamp,
			Target:    CompactToTarget(genesis.Header.DifficultyBits),
		},
		blockIndex: make(map[crypto.Hash]*blockNode),
		undoMap:    make(map[crypto.Hash]*tx.UndoData),
	}
	cs.blockIndex[genesisHash] = genesisNode
	cs.tipNode = genesisNode
	cs.undoMap[genesisHash] = undo
	return cs
}

// blockWork computes the PoW work for a single block from its difficulty bits.
// Work = 2^256 / (target+1). Simplified here to 1 + (maxTarget / target).
func blockWork(diffBits uint32) *big.Int {
	target := CompactToTarget(diffBits)
	t := new(big.Int).SetBytes(target[:])
	if t.Sign() <= 0 {
		return big.NewInt(1)
	}
	// work = 2^256 / (target + 1)
	maxVal := new(big.Int).Lsh(big.NewInt(1), 256)
	denom := new(big.Int).Add(t, big.NewInt(1))
	return new(big.Int).Div(maxVal, denom)
}

// AddBlock validates a new block and integrates it into the block index.
// If the new block creates a heavier chain than the current tip, a
// reorganization is performed automatically.
func (cs *ChainState) AddBlock(blk *block.Block) error {
	blkHash := blk.Header.Hash()

	// Already known?
	if _, exists := cs.blockIndex[blkHash]; exists {
		return fmt.Errorf("block %s already known", blkHash)
	}

	// Find parent in block index.
	parentNode, parentExists := cs.blockIndex[blk.Header.PrevBlockHash]
	if !parentExists {
		return fmt.Errorf("unknown parent block %s (orphan)", blk.Header.PrevBlockHash)
	}

	newHeight := parentNode.Height + 1

	// Is this block extending the current tip (main chain)?
	extendsMainChain := parentNode == cs.tipNode

	// Validate header format, SAT solution, and PoW against the parent header.
	// UTXO validation uses the main-chain UTXO set, which is only correct
	// for blocks extending the tip. Side-chain blocks that fail UTXO
	// validation are stored without it; full validation happens during reorg.
	if err := ValidateBlock(blk, &parentNode.Header, cs.Difficulty, cs.UTXOSet, newHeight); err != nil {
		if !extendsMainChain {
			// Side-chain block: store in index without UTXO validation.
			// Full validation will happen during reorganize().
			return cs.addSideChainBlock(blk, blkHash, parentNode, newHeight)
		}
		return err
	}

	// Create block node.
	work := blockWork(blk.Header.DifficultyBits)
	node := &blockNode{
		Hash:      blkHash,
		Height:    newHeight,
		Header:    blk.Header,
		Block:     blk,
		ChainWork: new(big.Int).Add(parentNode.ChainWork, work),
		Parent:    parentNode,
	}
	cs.blockIndex[blkHash] = node

	// Extends the current tip — normal append.
	if extendsMainChain {
		undo := cs.UTXOSet.ApplyBlockWithUndo(blk.Transactions, newHeight)
		cs.undoMap[blkHash] = undo
		cs.tipNode = node
		cs.Tip = &blk.Header
		cs.Height = newHeight
		cs.appendASERT(blk.Header.Timestamp, newHeight)
		return nil
	}

	// Side chain: check if it's heavier and trigger reorg.
	if node.ChainWork.Cmp(cs.tipNode.ChainWork) > 0 {
		return cs.reorganize(node)
	}

	// Lighter side chain — keep in index but don't switch.
	return nil
}

// addSideChainBlock stores a side-chain block in the index and checks for reorg.
func (cs *ChainState) addSideChainBlock(blk *block.Block, blkHash crypto.Hash, parentNode *blockNode, newHeight uint64) error {
	work := blockWork(blk.Header.DifficultyBits)
	node := &blockNode{
		Hash:      blkHash,
		Height:    newHeight,
		Header:    blk.Header,
		Block:     blk,
		ChainWork: new(big.Int).Add(parentNode.ChainWork, work),
		Parent:    parentNode,
	}
	cs.blockIndex[blkHash] = node

	if node.ChainWork.Cmp(cs.tipNode.ChainWork) > 0 {
		return cs.reorganize(node)
	}
	return nil
}

// reorganize switches the active chain from the current tip to newTip.
// Steps:
//  1. Find the fork point (common ancestor).
//  2. Roll back blocks from current tip to fork point.
//  3. Apply blocks from fork point to new tip.
func (cs *ChainState) reorganize(newTip *blockNode) error {
	forkPoint := findForkPoint(cs.tipNode, newTip)
	if forkPoint == nil {
		return errors.New("reorg: no common ancestor found")
	}

	// Collect blocks to disconnect (current tip → fork point, exclusive).
	var disconnect []*blockNode
	for n := cs.tipNode; n != forkPoint; n = n.Parent {
		disconnect = append(disconnect, n)
	}

	// Collect blocks to connect (fork point → new tip, in forward order).
	var connect []*blockNode
	for n := newTip; n != forkPoint; n = n.Parent {
		connect = append(connect, n)
	}
	// Reverse connect to get forward order.
	for i, j := 0, len(connect)-1; i < j; i, j = i+1, j-1 {
		connect[i], connect[j] = connect[j], connect[i]
	}

	// Step 2: Disconnect blocks (reverse order — newest first).
	for _, n := range disconnect {
		undo, ok := cs.undoMap[n.Hash]
		if !ok {
			return fmt.Errorf("reorg: missing undo data for block %s at height %d", n.Hash, n.Height)
		}
		if err := cs.UTXOSet.RollbackBlock(undo); err != nil {
			return fmt.Errorf("reorg: rollback block %d: %w", n.Height, err)
		}
		delete(cs.undoMap, n.Hash)

		// Notify caller (e.g., to return transactions to mempool).
		if cs.OnReorg != nil && n.Block != nil {
			cs.OnReorg(n.Block)
		}
	}

	// Step 3: Connect blocks (forward order — oldest first).
	for _, n := range connect {
		if n.Block == nil {
			return fmt.Errorf("reorg: missing block data for %s at height %d", n.Hash, n.Height)
		}
		undo := cs.UTXOSet.ApplyBlockWithUndo(n.Block.Transactions, n.Height)
		cs.undoMap[n.Hash] = undo
	}

	// Update chain state to new tip.
	cs.tipNode = newTip
	cs.Tip = &newTip.Header
	cs.Height = newTip.Height

	// Recalculate difficulty from ASERT for the new tip.
	cs.Difficulty = &DifficultyParams{
		PoWTarget: AdjustDifficultyASERT(cs.Anchor, cs.Height, cs.Tip.Timestamp),
	}

	return nil
}

// findForkPoint returns the common ancestor of two block nodes.
func findForkPoint(a, b *blockNode) *blockNode {
	// Bring both to the same height.
	for a.Height > b.Height {
		a = a.Parent
	}
	for b.Height > a.Height {
		b = b.Parent
	}
	// Walk both up until they meet.
	for a != b {
		if a == nil || b == nil {
			return nil
		}
		a = a.Parent
		b = b.Parent
	}
	return a
}

// appendASERT updates difficulty using ASERT after each accepted block.
func (cs *ChainState) appendASERT(timestamp uint32, height uint64) {
	cs.Difficulty = &DifficultyParams{
		PoWTarget: AdjustDifficultyASERT(cs.Anchor, height, timestamp),
	}
}

// GetDifficulty returns the current difficulty parameters.
func (cs *ChainState) GetDifficulty() *DifficultyParams {
	return cs.Difficulty
}

// GetMainChainBlock returns the block at the given height on the current main chain.
// Returns nil if no block exists at that height on the main chain.
func (cs *ChainState) GetMainChainBlock(height uint64) *block.Block {
	node := cs.tipNode.ancestor(height)
	if node == nil {
		return nil
	}
	return node.Block
}

// HasBlock returns true if the block hash is in the block index.
func (cs *ChainState) HasBlock(hash crypto.Hash) bool {
	_, ok := cs.blockIndex[hash]
	return ok
}

// GetBlockNode returns the block node for a given hash.
func (cs *ChainState) GetBlockNode(hash crypto.Hash) *blockNode {
	return cs.blockIndex[hash]
}

// AddBlockUnchecked applies a block without validation (for testing / genesis).
func (cs *ChainState) AddBlockUnchecked(blk *block.Block) error {
	if blk == nil {
		return errors.New("nil block")
	}
	blkHash := blk.Header.Hash()
	newHeight := cs.Height + 1

	// Build block node.
	work := blockWork(blk.Header.DifficultyBits)
	parentWork := big.NewInt(0)
	if cs.tipNode != nil {
		parentWork = cs.tipNode.ChainWork
	}
	node := &blockNode{
		Hash:      blkHash,
		Height:    newHeight,
		Header:    blk.Header,
		Block:     blk,
		ChainWork: new(big.Int).Add(parentWork, work),
		Parent:    cs.tipNode,
	}
	cs.blockIndex[blkHash] = node

	undo := cs.UTXOSet.ApplyBlockWithUndo(blk.Transactions, newHeight)
	cs.undoMap[blkHash] = undo

	cs.tipNode = node
	cs.Tip = &blk.Header
	cs.Height = newHeight
	cs.appendASERT(blk.Header.Timestamp, newHeight)
	return nil
}

// Package storage provides file-based block persistence for the NOUS node.
package storage

import (
	"encoding/binary"
	"encoding/gob"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"nous/block"
	"nous/crypto"
)

// ChainTip holds the persisted chain tip metadata.
type ChainTip struct {
	Hash   crypto.Hash
	Height uint64
}

// BlockStore provides file-based block storage.
// Layout:
//
//	<datadir>/blocks/NNNNNNNN.blk  — gob-encoded block.Block
//	<datadir>/chainstate.dat       — binary chain tip (hash + height)
type BlockStore struct {
	mu       sync.RWMutex
	dataDir  string
	blockDir string
}

// NewBlockStore opens or creates a block store at the given data directory.
func NewBlockStore(dataDir string) (*BlockStore, error) {
	blockDir := filepath.Join(dataDir, "blocks")
	if err := os.MkdirAll(blockDir, 0755); err != nil {
		return nil, fmt.Errorf("storage: create block dir: %w", err)
	}
	return &BlockStore{dataDir: dataDir, blockDir: blockDir}, nil
}

// SaveBlock persists a block at the given height.
func (bs *BlockStore) SaveBlock(blk *block.Block, height uint64) error {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	// Ensure block directory exists. On Windows, a prior "rmdir /S /Q" may
	// defer actual deletion until file handles are released, causing the
	// directory to vanish mid-run. MkdirAll is a no-op if it already exists.
	if err := os.MkdirAll(bs.blockDir, 0755); err != nil {
		return fmt.Errorf("storage: ensure block dir: %w", err)
	}

	path := bs.blockPath(height)
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("storage: create block file: %w", err)
	}
	defer f.Close()

	if err := gob.NewEncoder(f).Encode(blk); err != nil {
		return fmt.Errorf("storage: encode block: %w", err)
	}
	return nil
}

// LoadBlockByHeight reads a block from disk by its height.
func (bs *BlockStore) LoadBlockByHeight(height uint64) (*block.Block, error) {
	bs.mu.RLock()
	defer bs.mu.RUnlock()

	path := bs.blockPath(height)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("storage: block at height %d not found", height)
		}
		return nil, fmt.Errorf("storage: open block file: %w", err)
	}
	defer f.Close()

	var blk block.Block
	if err := gob.NewDecoder(f).Decode(&blk); err != nil {
		return nil, fmt.Errorf("storage: decode block: %w", err)
	}
	return &blk, nil
}

// SaveChainTip persists the current chain tip.
func (bs *BlockStore) SaveChainTip(tip ChainTip) error {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	path := filepath.Join(bs.dataDir, "chainstate.dat")
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("storage: create chainstate: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(tip.Hash[:]); err != nil {
		return err
	}
	return binary.Write(f, binary.LittleEndian, tip.Height)
}

// GetChainTip reads the persisted chain tip. Returns an error if no tip exists.
func (bs *BlockStore) GetChainTip() (ChainTip, error) {
	bs.mu.RLock()
	defer bs.mu.RUnlock()

	path := filepath.Join(bs.dataDir, "chainstate.dat")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ChainTip{}, errors.New("storage: no chain tip found")
		}
		return ChainTip{}, fmt.Errorf("storage: open chainstate: %w", err)
	}
	defer f.Close()

	var tip ChainTip
	if _, err := f.Read(tip.Hash[:]); err != nil {
		return ChainTip{}, fmt.Errorf("storage: read tip hash: %w", err)
	}
	if err := binary.Read(f, binary.LittleEndian, &tip.Height); err != nil {
		return ChainTip{}, fmt.Errorf("storage: read tip height: %w", err)
	}
	return tip, nil
}

// DeleteBlock removes the block file at the given height.
func (bs *BlockStore) DeleteBlock(height uint64) error {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	path := bs.blockPath(height)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("storage: delete block %d: %w", height, err)
	}
	return nil
}

// HasBlock checks if a block file exists at the given height.
func (bs *BlockStore) HasBlock(height uint64) bool {
	bs.mu.RLock()
	defer bs.mu.RUnlock()
	_, err := os.Stat(bs.blockPath(height))
	return err == nil
}

func (bs *BlockStore) blockPath(height uint64) string {
	return filepath.Join(bs.blockDir, fmt.Sprintf("%08d.blk", height))
}

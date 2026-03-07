// Package block defines the NOUS block structure, header serialization,
// Merkle tree computation, and genesis block construction.
package block

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"time"

	"nous/crypto"
	"nous/tx"
)

// HeaderSize is the fixed size of a serialized Cogito Consensus header.
// 4 (Version) + 32 (PrevBlockHash) + 32 (MerkleRoot) + 4 (Timestamp) +
// 4 (DifficultyBits) + 8 (Seed) + 32 (SATSolutionHash) + 32 (UTXOSetHash) = 148 bytes.
const HeaderSize = 148

// Header contains the metadata of a block.
// All fields are fixed-size; serialization uses little-endian byte order.
type Header struct {
	Version         uint32
	PrevBlockHash   crypto.Hash
	MerkleRoot      crypto.Hash
	Timestamp       uint32 // Unix epoch seconds
	DifficultyBits  uint32 // compact difficulty representation
	Seed            uint64 // SAT formula seed nonce
	SATSolutionHash crypto.Hash // SHA256 of serialized SAT assignment
	UTXOSetHash     crypto.Hash // commitment to UTXO set state
}

// Block represents a full block: header + body.
type Block struct {
	Header       Header
	Transactions []*tx.Transaction
	SATSolution  []bool // SAT assignment (256 booleans)
}

// Block size limits use a two-tier design:
//   - SoftBlockSize (4 MB): default relay/mining policy limit.
//   - MaxBlockSize (16 MB): hard consensus limit; blocks above this are invalid.
// Miners may produce blocks up to MaxBlockSize, but default policy targets SoftBlockSize
// to leave headroom for burst traffic without hitting the hard cap.
const (
	SoftBlockSize = 4_000_000
	MaxBlockSize  = 16_000_000
)

// MaxBlockTransactions is the maximum number of transactions in a single block.
const MaxBlockTransactions = 10_000

// WireSize returns the approximate serialized size of the block in bytes.
// It sums the header size, all transaction sizes, and the SAT solution size.
func (b *Block) WireSize() int {
	size := HeaderSize
	for _, t := range b.Transactions {
		size += len(t.Serialize())
	}
	// SAT solution: 4-byte length prefix + ceil(len/8) bytes.
	if len(b.SATSolution) > 0 {
		size += 4 + (len(b.SATSolution)+7)/8
	}
	return size
}

// Serialize encodes the block header into a deterministic 148-byte slice.
// All fields are fixed-size and written in little-endian order.
func (h *Header) Serialize() []byte {
	buf := make([]byte, HeaderSize)
	off := 0

	binary.LittleEndian.PutUint32(buf[off:], h.Version)
	off += 4
	copy(buf[off:], h.PrevBlockHash[:])
	off += 32
	copy(buf[off:], h.MerkleRoot[:])
	off += 32
	binary.LittleEndian.PutUint32(buf[off:], h.Timestamp)
	off += 4
	binary.LittleEndian.PutUint32(buf[off:], h.DifficultyBits)
	off += 4
	binary.LittleEndian.PutUint64(buf[off:], h.Seed)
	off += 8
	copy(buf[off:], h.SATSolutionHash[:])
	off += 32
	copy(buf[off:], h.UTXOSetHash[:])

	return buf
}

// Hash computes the double-SHA-256 hash of the serialized block header.
func (h *Header) Hash() crypto.Hash {
	return crypto.DoubleSha256(h.Serialize())
}

// DeserializeHeader decodes a block header from its serialized form.
// Expects exactly 148 bytes.
func DeserializeHeader(data []byte) (*Header, error) {
	if len(data) < HeaderSize {
		return nil, fmt.Errorf("block: header data too short (%d bytes, need %d)", len(data), HeaderSize)
	}
	r := bytes.NewReader(data)
	h := &Header{}

	if err := binary.Read(r, binary.LittleEndian, &h.Version); err != nil {
		return nil, err
	}
	if _, err := r.Read(h.PrevBlockHash[:]); err != nil {
		return nil, err
	}
	if _, err := r.Read(h.MerkleRoot[:]); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &h.Timestamp); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &h.DifficultyBits); err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &h.Seed); err != nil {
		return nil, err
	}
	if _, err := r.Read(h.SATSolutionHash[:]); err != nil {
		return nil, err
	}
	if _, err := r.Read(h.UTXOSetHash[:]); err != nil {
		return nil, err
	}

	return h, nil
}

// GenesisBlock creates the genesis block for the NOUS chain.
// The genesis block has a zero PrevBlockHash and contains a single
// coinbase transaction paying the initial reward to the given public key hash.
//
// For mainnet, the coinbase output is an OP_RETURN with a genesis message,
// making it provably unspendable (fair launch — no premine).
//
// If timestamp is 0, the current wall-clock time minus one block interval is used.
// isTestnet selects the chain ID embedded in the coinbase transaction.
func GenesisBlock(pubKeyHash []byte, timestamp uint32, difficultyBits uint32, isTestnet bool) *Block {
	if timestamp == 0 {
		timestamp = uint32(time.Now().Unix()) - 150
	}

	var coinbase *tx.Transaction
	if isTestnet {
		const genesisReward int64 = 1_00000000 // 1 NOUS in nou
		coinbase = tx.NewCoinbaseTx(0, genesisReward, tx.CreateP2PKHLockScript(pubKeyHash), tx.ChainIDFor(true))
	} else {
		// Mainnet: OP_RETURN genesis message, 0 reward (unspendable).
		msg := []byte("NOUS Genesis 2026-03-07 / The beginning of wisdom is to call things by their proper name - Confucius / Cogito, ergo sum")
		coinbase = tx.NewCoinbaseTx(0, 0, tx.CreateOpReturnScript(msg), tx.ChainIDFor(false))
	}
	txIDs := []crypto.Hash{coinbase.TxID()}
	merkleRoot := ComputeMerkleRoot(txIDs)

	return &Block{
		Header: Header{
			Version:        1,
			PrevBlockHash:  crypto.Hash{},
			MerkleRoot:     merkleRoot,
			Timestamp:      timestamp,
			DifficultyBits: difficultyBits,
		},
		Transactions: []*tx.Transaction{coinbase},
	}
}

// Package block defines the NOUS block structure, header serialization,
// Merkle tree computation, and genesis block construction.
package block

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/nous-chain/nous/crypto"
	"github.com/nous-chain/nous/csp"
	"github.com/nous-chain/nous/tx"
)

// Header contains the metadata of a block.
// Serialization uses little-endian for fixed-size fields and
// length-prefixed encoding for variable-length fields.
type Header struct {
	Version        uint32
	PrevBlockHash  crypto.Hash
	MerkleRoot     crypto.Hash
	Timestamp      uint32 // Unix epoch seconds
	DifficultyBits uint32 // compact difficulty representation

	// VDF fields
	VDFOutput      []byte // y = g^(2^T) mod N
	VDFProof       []byte // Wesolowski proof π
	VDFIterations  uint64 // T parameter

	// CSP fields
	CSPSolutionHash crypto.Hash // hash of the standard CSP solution

	// Miner identity
	MinerPubKey []byte // compressed secp256k1 public key (33 bytes)

	// PoW
	Nonce uint32
}

// Block represents a full block: header + body.
type Block struct {
	Header       Header
	Transactions []*tx.Transaction
	CSPSolution  *csp.Solution // standard-tier solution
}

// MaxBlockSize is the maximum allowed serialized block size (1 MB).
const MaxBlockSize = 1_000_000

// MaxBlockTransactions is the maximum number of transactions in a single block.
// A minimal transaction is ~100 bytes, so 10,000 txs ≈ 1 MB.
const MaxBlockTransactions = 10_000

// WireSize returns the approximate serialized size of the block in bytes.
// It sums the header size, all transaction sizes, and the CSP solution size.
func (b *Block) WireSize() int {
	size := len(b.Header.Serialize())
	for _, t := range b.Transactions {
		size += len(t.Serialize())
	}
	if b.CSPSolution != nil {
		// 4 bytes for value count + 4 bytes per value.
		size += 4 + len(b.CSPSolution.Values)*4
	}
	return size
}

// Serialize encodes the block header into a deterministic byte slice.
// Fixed-size fields are written in little-endian order.
// Variable-length fields (VDFOutput, VDFProof, MinerPubKey) are
// length-prefixed with a uint16 LE length.
func (h *Header) Serialize() []byte {
	var buf bytes.Buffer

	// Fixed-size fields
	binary.Write(&buf, binary.LittleEndian, h.Version)
	buf.Write(h.PrevBlockHash[:])
	buf.Write(h.MerkleRoot[:])
	binary.Write(&buf, binary.LittleEndian, h.Timestamp)
	binary.Write(&buf, binary.LittleEndian, h.DifficultyBits)

	// Variable-length VDF fields
	writeVarField(&buf, h.VDFOutput)
	writeVarField(&buf, h.VDFProof)
	binary.Write(&buf, binary.LittleEndian, h.VDFIterations)

	// CSP hash
	buf.Write(h.CSPSolutionHash[:])

	// Miner pubkey (variable length)
	writeVarField(&buf, h.MinerPubKey)

	// Nonce
	binary.Write(&buf, binary.LittleEndian, h.Nonce)

	return buf.Bytes()
}

// Hash computes the double-SHA-256 hash of the serialized block header.
func (h *Header) Hash() crypto.Hash {
	return crypto.DoubleSha256(h.Serialize())
}

// DeserializeHeader decodes a block header from its serialized form.
// Returns an error if the data is too short or malformed.
//
// The minimum header size is 4 (version) + 32 (prev) + 32 (merkle) +
// 4 (timestamp) + 4 (difficulty) + 2+0 (VDF output) + 2+0 (VDF proof) +
// 8 (VDF iterations) + 32 (CSP hash) + 2+0 (miner pubkey) + 4 (nonce) = 126 bytes.
func DeserializeHeader(data []byte) (*Header, error) {
	if len(data) < 126 {
		return nil, fmt.Errorf("block: header data too short (%d bytes, min 126)", len(data))
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

	var err error
	h.VDFOutput, err = readVarField(r)
	if err != nil {
		return nil, err
	}
	h.VDFProof, err = readVarField(r)
	if err != nil {
		return nil, err
	}
	if err := binary.Read(r, binary.LittleEndian, &h.VDFIterations); err != nil {
		return nil, err
	}

	if _, err := r.Read(h.CSPSolutionHash[:]); err != nil {
		return nil, err
	}

	h.MinerPubKey, err = readVarField(r)
	if err != nil {
		return nil, err
	}

	if err := binary.Read(r, binary.LittleEndian, &h.Nonce); err != nil {
		return nil, err
	}

	return h, nil
}

// GenesisBlock creates the genesis block for the NOUS chain.
// The genesis block has a zero PrevBlockHash and contains a single
// coinbase transaction paying the initial reward to the given public key hash.
//
// If timestamp is 0, the current wall-clock time is used. For mainnet launch
// this will be replaced with a hardcoded timestamp and embedded news headline.
func GenesisBlock(pubKeyHash []byte, timestamp uint32) *Block {
	const genesisReward int64 = 10_0000_0000 // 10 NOUS in nou

	if timestamp == 0 {
		timestamp = uint32(time.Now().Unix())
	}

	coinbase := tx.NewCoinbase(0, genesisReward, pubKeyHash, "NOUS genesis")
	txIDs := []crypto.Hash{coinbase.TxID()}
	merkleRoot := ComputeMerkleRoot(txIDs)

	return &Block{
		Header: Header{
			Version:        1,
			PrevBlockHash:  crypto.Hash{},
			MerkleRoot:     merkleRoot,
			Timestamp:      timestamp,
			DifficultyBits: 0x1d00ffff, // initial difficulty (Bitcoin-style compact)
			VDFOutput:      []byte{},
			VDFProof:       []byte{},
			VDFIterations:  0,
			MinerPubKey:    []byte{},
			Nonce:          0,
		},
		Transactions: []*tx.Transaction{coinbase},
	}
}

// --- encoding helpers ---

// writeVarField writes a uint16 LE length prefix followed by the data.
func writeVarField(buf *bytes.Buffer, data []byte) {
	binary.Write(buf, binary.LittleEndian, uint16(len(data)))
	buf.Write(data)
}

// readVarField reads a uint16 LE length prefix followed by that many bytes.
func readVarField(r *bytes.Reader) ([]byte, error) {
	var length uint16
	if err := binary.Read(r, binary.LittleEndian, &length); err != nil {
		return nil, err
	}
	data := make([]byte, length)
	if _, err := r.Read(data); err != nil {
		return nil, err
	}
	return data, nil
}

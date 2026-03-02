package network

import (
	"nous/crypto"
)

// Wire protocol constants.
const (
	// MainNetMagic identifies the NOUS mainnet.
	MainNetMagic uint32 = 0x4E4F5553 // "NOUS" in ASCII

	// TestNetMagic identifies the NOUS testnet.
	TestNetMagic uint32 = 0x4E545354 // "NTST"

	// CommandSize is the fixed length of the command field in the message header.
	CommandSize = 12

	// HeaderSize is the total wire header size: magic(4) + command(12) + length(4) + checksum(4).
	HeaderSize = 24

	// MaxPayloadSize limits the maximum message payload (4 MB).
	MaxPayloadSize = 4 * 1024 * 1024

	// MaxInvItems is the maximum number of inventory items in a single inv/getdata message.
	MaxInvItems = 50_000

	// MaxAddrCount is the maximum number of addresses in a single addr message.
	MaxAddrCount = 1_000

	// ProtocolVersion is the current protocol version.
	ProtocolVersion uint32 = 1

	// DefaultPort is the default TCP port for the NOUS P2P network.
	DefaultPort = 9333
)

// Command strings for each message type.
const (
	CmdVersion   = "version"
	CmdVerAck    = "verack"
	CmdGetBlocks = "getblocks"
	CmdInv       = "inv"
	CmdGetData   = "getdata"
	CmdBlock     = "block"
	CmdTx        = "tx"
	CmdPing      = "ping"
	CmdPong      = "pong"
	CmdAddr      = "addr"
)

// InvType identifies the kind of inventory item.
type InvType uint32

const (
	InvTypeBlock InvType = 1
	InvTypeTx   InvType = 2
)

// InvItem is a single inventory vector: type + hash.
type InvItem struct {
	Type InvType
	Hash crypto.Hash
}

// Message is the interface that all network messages implement.
type Message interface {
	Command() string
}

// MsgVersion is sent during the initial handshake.
type MsgVersion struct {
	Version     uint32
	BlockHeight uint64
	Timestamp   uint64
	Nonce       uint64
	UserAgent   string
	ListenPort  uint16
}

func (m *MsgVersion) Command() string { return CmdVersion }

// MsgVerAck acknowledges a version handshake.
type MsgVerAck struct{}

func (m *MsgVerAck) Command() string { return CmdVerAck }

// MsgGetBlocks requests block hashes starting from a given hash.
type MsgGetBlocks struct {
	StartHash crypto.Hash
	StopHash  crypto.Hash // zero hash = get as many as possible
}

func (m *MsgGetBlocks) Command() string { return CmdGetBlocks }

// MsgInv advertises known block or transaction hashes.
type MsgInv struct {
	Items []InvItem
}

func (m *MsgInv) Command() string { return CmdInv }

// MsgGetData requests specific blocks or transactions by hash.
type MsgGetData struct {
	Items []InvItem
}

func (m *MsgGetData) Command() string { return CmdGetData }

// MsgBlock carries a serialized block header (full block sent as raw bytes).
type MsgBlock struct {
	Payload []byte // serialized block data
}

func (m *MsgBlock) Command() string { return CmdBlock }

// MsgTx carries a serialized transaction.
type MsgTx struct {
	Payload []byte // serialized transaction data
}

func (m *MsgTx) Command() string { return CmdTx }

// MsgPing is a heartbeat request.
type MsgPing struct {
	Nonce uint64
}

func (m *MsgPing) Command() string { return CmdPing }

// MsgPong is a heartbeat response.
type MsgPong struct {
	Nonce uint64
}

func (m *MsgPong) Command() string { return CmdPong }

// NetAddress represents a peer address for the addr message.
type NetAddress struct {
	IP   string
	Port uint16
}

// MsgAddr carries a list of known peer addresses.
type MsgAddr struct {
	Addresses []NetAddress
}

func (m *MsgAddr) Command() string { return CmdAddr }

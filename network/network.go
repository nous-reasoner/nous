// Package network provides P2P networking for NOUS nodes.
//
// Architecture:
//
//   - protocol.go: Message types and wire format constants
//   - message.go:  EncodeMessage / DecodeMessage (wire serialization)
//   - peer.go:     Peer, PeerManager, AddressBook
//   - mempool.go:  Transaction memory pool with fee-rate ordering
//   - server.go:   TCP server, connection management, message routing
//
// Wire format (per message):
//
//	magic(4) + command(12) + payload_len(4) + checksum(4) + payload
//
// The checksum is the first 4 bytes of DoubleSha256(payload).
package network

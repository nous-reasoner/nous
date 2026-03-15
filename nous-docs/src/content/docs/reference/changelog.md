---
title: Changelog
description: Protocol versions, network upgrades, and notable changes to the NOUS node software.
---

## Protocol v3 (2026-03-15)

**Backward compatible** — v3 nodes communicate seamlessly with v2 and v1 nodes. No manual action required; simply update your node binary.

### Peer Discovery (addr protocol)

Nodes now actively exchange peer addresses using `getaddr`/`addr` messages. Previously, nodes only discovered peers through hardcoded seed nodes. With v3:

- Your node learns about other nodes from its peers and shares its own address
- Peer addresses are persisted in `peers.json`, so your node reconnects faster after restarts
- Address relay limits prevent network flooding (max 1000 per message, 100 per source)

### Anti-Eclipse Protection

The new **AddrManager** uses a dual-bucket system (new + tried) with randomized SHA256-based hashing to prevent [eclipse attacks](https://en.wikipedia.org/wiki/Eclipse_attack):

- Addresses from the same source are distributed across different buckets
- An attacker cannot predict which bucket an address will land in
- Successfully connected peers are promoted to "tried" buckets, separating proven peers from unverified ones
- 1024 new buckets + 256 tried buckets, each holding up to 64 addresses

### Higher Peer Limits

| | v2 | v3 |
|---|---|---|
| Max inbound | 64 | 117 |
| Max outbound | 8 | 8 |
| Max total | 72 | 125 |

### Inbound Peer Eviction

When inbound slots are full, v3 nodes intelligently evict the least valuable peer instead of rejecting all new connections. Protected peers include:

- Peers that recently sent valid blocks
- Peers with the lowest network latency
- Peers that recently relayed valid transactions
- Longest-connected peers

Eviction targets the most-represented subnet to maintain network diversity.

### Unknown Message Tolerance

Nodes now gracefully handle unrecognized message types — reading and discarding them instead of penalizing the sender. This is the foundation for future protocol extensions without breaking older nodes.

---

## Protocol v2 (2025-12-01)

- **Headers-first sync**: Faster initial block download by fetching headers first, then downloading blocks in parallel from multiple peers
- `getheaders`/`headers` message types

## Protocol v1 (2025-10-01)

- Initial release
- Basic block relay, transaction relay, and peer management
- `version`/`verack` handshake, `getblocks`/`inv`/`getdata`/`block`/`tx` messages

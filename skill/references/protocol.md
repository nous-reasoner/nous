# NOUS Protocol Reference

## Consensus: Cogito Consensus

Each block requires:
1. **3-SAT solution** — solve a random Boolean satisfiability instance
2. **SHA-256 binding** — hash(header + SAT_solution) < target
3. **ASERT difficulty** — adjusts target continuously per block

### Block Validation Formula

```
header_hash = SHA256d(version | prev_hash | merkle_root | timestamp | difficulty_bits | seed)
block_valid = SAT_valid(seed, solution) AND header_hash < target(difficulty_bits)
```

## Parameters

| Parameter | Value | Notes |
|---|---|---|
| Block time target | 150 seconds | 2.5 minutes |
| Block reward | 1.00000000 NOUS | No halving, constant forever |
| SAT variables | 256 | Per block attempt |
| SAT clauses | 986 | Ratio = 3.85 (phase transition) |
| SAT clause length | 3 | 3-SAT (NP-complete) |
| ASERT half-life | 43,200 seconds | 12 hours |
| Coinbase maturity | 100 blocks | ~4.2 hours |
| Max block size | 16 MB | Hard consensus limit |
| Soft block size | 4 MB | Default relay/mining policy |
| Max transactions/block | 10,000 | |
| Dust limit | 546 nou | Minimum non-coinbase output |
| Base unit | 1 NOUS = 10^8 nou | |

## Network

| Parameter | Value |
|---|---|
| P2P port | 9333 |
| RPC port | 9332 |
| Protocol version | 1 |
| MainNet magic | 0x4E4F5553 ("NOUS") |
| TestNet magic | 0x4E545354 ("NTST") |
| Address format | Bech32m, prefix `nous1q` |
| Chain ID | 0x4E4F5553 ("NOUS") |
| Transaction version | 2 |

## Testnet Seeds

| Address | Location |
|---|---|
| 80.78.26.7:9333 | Seed node 1 |
| 80.78.25.211:9333 | Seed node 2 |

## ASERT Difficulty Adjustment

Adjusts difficulty continuously (not epoch-based like Bitcoin).

```
new_target = anchor_target * 2^((time_delta - ideal_delta) / halflife)
```

Where:
- `anchor_target`: target of a reference block
- `time_delta`: actual time since anchor block
- `ideal_delta`: expected time (blocks_since_anchor * 150s)
- `halflife`: 43,200 seconds (12 hours)

If blocks are too fast, target decreases (harder). If too slow, target increases (easier).

## RPC Methods

All methods use JSON-RPC 2.0 over HTTP POST to `/rpc`.

| Method | Params | Returns |
|---|---|---|
| `getblockcount` | none | `uint64` — current chain height |
| `getblock` | `[height]` | block object (hash, height, version, timestamp, prev_hash, merkle_root, difficulty, seed, tx_count, transactions) |
| `getblockhash` | `[height]` | `string` — hex block hash |
| `gettx` | `[txid_hex]` | transaction object (txid, version, inputs, outputs, block_height or mempool flag) |
| `sendrawtx` | `[raw_tx_hex]` | `string` — hex txid |
| `getbalance` | `[address]` | `int64` — balance in nou |
| `getmininginfo` | none | object (height, difficulty_bits, mempool_size, reasoning) |
| `getpeerinfo` | none | array of peer objects (addr, inbound, version, block_height, handshaked) |
| `listunspent` | `[address]` | array of UTXO objects (txid, index, value, script, height) |

### Example RPC Call

```bash
curl -s -X POST http://localhost:9332/rpc \
  -d '{"jsonrpc":"2.0","method":"getblockcount","params":[],"id":1}'
```

Response:
```json
{"jsonrpc":"2.0","result":1234,"id":1}
```

## Script System

P2PKH (Pay-to-Public-Key-Hash):

```
Lock:   OP_DUP OP_HASH160 <20-byte-pubkey-hash> OP_EQUALVERIFY OP_CHECKSIG
Unlock: <signature> <public-key>
```

| Opcode | Hex | Description |
|---|---|---|
| OP_DUP | 0x76 | Duplicate top stack item |
| OP_HASH160 | 0xa9 | SHA256 then RIPEMD160 |
| OP_EQUALVERIFY | 0x88 | Check equality, fail if not |
| OP_CHECKSIG | 0xac | Verify ECDSA signature |

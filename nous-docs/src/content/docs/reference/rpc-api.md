---
title: RPC API
description: JSON-RPC API reference for NOUS nodes.
---

The NOUS node exposes a JSON-RPC API for interacting with the blockchain.

## Endpoint

Default: `http://localhost:8332/rpc`
Public: `http://rpc.nouschain.org/api`

## Request Format

```json
{
  "method": "methodname",
  "params": [...],
  "id": 1
}
```

## Methods

### getblockcount
Returns the current block height.

```bash
curl -X POST http://rpc.nouschain.org/api \
  -H "Content-Type: application/json" \
  -d '{"method":"getblockcount","params":[],"id":1}'
```

Response:
```json
{"result": 4800, "id": 1}
```

### getblockhash
Returns the block hash at the given height.

**Params**: `[height]`

```json
{"method": "getblockhash", "params": [100], "id": 1}
```

### getblock
Returns block data for a given hash.

**Params**: `[hash]`

### gettx
Returns transaction data for a given txid.

**Params**: `[txid]`

### getbalance
Returns balance for an address.

**Params**: `[address]`

Response:
```json
{
  "result": {
    "balance": 100000000,
    "immature": 500000000
  }
}
```

- `balance`: Spendable balance in base units (1 NOUS = 100,000,000)
- `immature`: Coinbase rewards waiting for 100 confirmations

### listunspent
Returns unspent transaction outputs for an address.

**Params**: `[address]`

Response:
```json
{
  "result": [
    {
      "txid": "abc123...",
      "index": 0,
      "value": 100000000,
      "height": 4500,
      "is_coinbase": false
    }
  ]
}
```

### sendrawtx
Broadcasts a signed raw transaction.

**Params**: `[hex_encoded_tx]`

Response:
```json
{"result": "txid_hash_here"}
```

### getwork
Returns a mining work template for the given address.

**Params**: `[address]`

Response:
```json
{
  "height": 6200,
  "prev_hash": "0000063dbc5334ed...",
  "difficulty_bits": 503747976,
  "header_hex": "0100000000000..."
}
```

| Field | Description |
|-------|-------------|
| `height` | Block height being mined |
| `prev_hash` | Previous block hash (hex) |
| `difficulty_bits` | Compact difficulty target |
| `header_hex` | 148-byte header template (hex). Miner fills in seed (bytes 76-84) and solution hash (bytes 84-116) |

Miners generate the 3-SAT formula locally from `SHA256(prev_hash || seed)` — the formula is deterministic and does not need to be transmitted.

### submitblock
Submits a solved block.

**Params**: `[hex_encoded_block]`

### getpeerinfo
Returns connected peer information.

### getmininginfo
Returns current mining difficulty and network stats.

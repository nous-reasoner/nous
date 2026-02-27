# NOUS - Proof of Intelligence Cryptocurrency

NOUS is a cryptocurrency that replaces brute-force hash grinding with AI reasoning as proof of work. Miners solve deterministically generated Constraint Satisfaction Problems (CSPs) — a task where large language models excel but conventional hardware cannot shortcut. Combined with Verifiable Delay Functions (VDF) for time-fairness and a lightweight PoW layer for Sybil resistance, NOUS creates a three-layer consensus mechanism that rewards intelligence over raw computation.

## Quick Start

Download the latest binary for your platform from [Releases], then:

```bash
# 1. Create a wallet
./nous-cli createwallet

# 2. Start mining (testnet)
./nousd -testnet -mine -rpc 127.0.0.1:9332 \
        -peers SEED_NODE_1:9333,SEED_NODE_2:9333

# 3. Check your balance
./nous-cli -rpc 127.0.0.1:9332 getbalance
```

## Build from Source

Requires Go 1.22+.

```bash
# Build the full node daemon
go build -o nousd ./cmd/nousd

# Build the CLI client
go build -o nous-cli ./cmd/nous-cli
```

Cross-compile for all platforms:

```bash
make build-all    # outputs to build/{linux,darwin,windows}/
make release      # creates release/*.tar.gz and *.zip
```

## Architecture

NOUS uses a three-layer consensus mechanism:

| Layer | Purpose | Mechanism |
|-------|---------|-----------|
| **VDF** | Time-fairness | Wesolowski sequential squarings — ensures minimum elapsed time per block |
| **CSP** | Proof of Intelligence | Deterministic constraint satisfaction problems solved by AI/LLM miners |
| **PoW** | Sybil resistance | Lightweight SHA-256 nonce search with adaptive difficulty |

Block time target: **30 seconds**.

## Network

| Parameter | Value |
|-----------|-------|
| Default P2P port | 9333 |
| Default RPC port | 9332 |
| Testnet magic | `0x4E4F5553` |
| Block reward | 10 NOUS (halving every 1,050,000 blocks) |
| Difficulty adjustment | Every 144 blocks |

Seed nodes (testnet):

```
SEED_NODE_1:9333
SEED_NODE_2:9333
```

## Documentation

- [Whitepaper][whitepaper link]

## License

All rights reserved. See LICENSE for details.

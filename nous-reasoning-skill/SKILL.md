---
name: nous-reasoner
description: >
  Reason on the NOUS blockchain by solving NP-complete 3-SAT problems.
  Use when the user wants to mine/reason NOUS, check balance, send NOUS,
  or manage a NOUS wallet.
  Requires Go 1.24+ and network access to NOUS seed nodes.
metadata:
  openclaw:
    emoji: "🧠"
    requires:
      bins: ["go"]
    install:
      - id: go-build
        kind: shell
        command: |
          cd /tmp && rm -rf nous-build &&
          git clone https://github.com/nous-reasoner/nous.git nous-build &&
          cd nous-build &&
          go build -o miner ./cmd/miner/ &&
          go build -o nous-cli ./cmd/nous-cli/ &&
          mkdir -p ~/.nous/bin &&
          cp miner nous-cli ~/.nous/bin/ &&
          rm -rf /tmp/nous-build
        bins: ["~/.nous/bin/miner", "~/.nous/bin/nous-cli"]
        label: "Build NOUS miner and CLI from source"
allowed-tools: Bash Read Write
---

# NOUS Reasoner

Mine NOUS — the first cryptocurrency where every block requires
solving an NP-complete problem. No full node required; the miner
connects to a public RPC endpoint and solves 3-SAT formulas locally.

## What is NOUS

NOUS is a proof-of-work blockchain where mining = solving randomly
generated 3-SAT formulas. Instead of brute-force hashing (Bitcoin),
each block attempt requires genuine logical reasoning.

- Block time: 150 seconds
- Reward: 1 NOUS per block, forever (no halving)
- Consensus: Cogito Consensus (3-SAT + SHA-256 + ASERT difficulty)
- SAT parameters: 256 variables, 986 clauses (ratio 3.85)
- Solver: ProbSAT (local, no GPU required)

**Networks:**
- Mainnet: RPC port 8332, P2P port 8333
- Testnet: RPC port 9332, P2P port 9333

**Public RPC endpoints:**
- https://seed1.nouschain.org/rpc
- https://seed2.nouschain.org/rpc
- https://seed3.nouschain.org/rpc

## Setup

### First-time installation

1. Check if miner exists:

```bash
~/.nous/bin/miner --help 2>/dev/null || echo "NOT_INSTALLED"
```

2. If not installed, build from source:

```bash
cd /tmp && rm -rf nous-build && \
  git clone https://github.com/nous-reasoner/nous.git nous-build && \
  cd nous-build && \
  go build -o miner ./cmd/miner/ && \
  go build -o nous-cli ./cmd/nous-cli/ && \
  mkdir -p ~/.nous/bin && \
  cp miner nous-cli ~/.nous/bin/ && \
  rm -rf /tmp/nous-build
```

3. Create a wallet:

```bash
mkdir -p ~/.nous/mainnet
~/.nous/bin/nous-cli --walletfile ~/.nous/mainnet/wallet.dat --walletpass default createwallet
```

4. Note the address printed (starts with `nous1q...`). This is the reasoning reward address.

## Commands

### Create wallet

```bash
# Mainnet
~/.nous/bin/nous-cli --walletfile ~/.nous/mainnet/wallet.dat --walletpass default createwallet

# Testnet
mkdir -p ~/.nous/testnet
~/.nous/bin/nous-cli --walletfile ~/.nous/testnet/wallet.dat --walletpass default createwallet
```

### Get address

```bash
# Mainnet
~/.nous/bin/nous-cli --walletfile ~/.nous/mainnet/wallet.dat --walletpass default getaddress

# Testnet
~/.nous/bin/nous-cli --walletfile ~/.nous/testnet/wallet.dat --walletpass default getaddress
```

### Start reasoning (mining)

The miner connects to a public RPC node — no full chain sync needed.

```bash
# Mainnet (using public RPC)
~/.nous/bin/miner \
  --node https://seed1.nouschain.org \
  --address nous1q... \
  > ~/.nous/mainnet/miner.log 2>&1 &
echo $! > ~/.nous/mainnet/miner.pid

# Testnet
~/.nous/bin/miner \
  --node http://localhost:9332 \
  --address nous1q... \
  > ~/.nous/testnet/miner.log 2>&1 &
echo $! > ~/.nous/testnet/miner.pid
```

After starting, check logs:

```bash
tail -5 ~/.nous/mainnet/miner.log
```

### Stop reasoning

```bash
# Mainnet
kill $(cat ~/.nous/mainnet/miner.pid 2>/dev/null) 2>/dev/null && echo "Stopped" || echo "Not running"

# Testnet
kill $(cat ~/.nous/testnet/miner.pid 2>/dev/null) 2>/dev/null && echo "Stopped" || echo "Not running"
```

### Check balance

```bash
# Mainnet (via public RPC)
~/.nous/bin/nous-cli --rpchost seed1.nouschain.org --rpcport 8332 getbalance nous1q...

# Testnet
~/.nous/bin/nous-cli --rpcport 9332 getbalance nous1q...
```

### Check mining status

```bash
# Mainnet
~/.nous/bin/nous-cli --rpchost seed1.nouschain.org --rpcport 8332 getmininginfo

# Testnet
~/.nous/bin/nous-cli --rpcport 9332 getmininginfo
```

### Send NOUS

```bash
# Mainnet
~/.nous/bin/nous-cli \
  --rpchost seed1.nouschain.org --rpcport 8332 \
  --walletfile ~/.nous/mainnet/wallet.dat \
  --walletpass default \
  send <to_address> <amount>

# Testnet
~/.nous/bin/nous-cli \
  --rpcport 9332 \
  --walletfile ~/.nous/testnet/wallet.dat \
  --walletpass default \
  send <to_address> <amount>
```

## Display Format

When reasoning starts, tell the user:

```
NOUS Reasoner started!
  Network: mainnet
  Mode: RPC-based (no full node required)
  Each block attempt solves a 256-variable, 986-clause 3-SAT formula.
  Reward: 1 NOUS per block (150 second target).
  Node: seed1.nouschain.org

  Say "check mining status" or "stop reasoning".
```

When showing balance:

```
nous1q...: X.XXXXXXXX NOUS
```

## Miner CLI Reference

```
miner [flags]

Flags:
  --node <url>       Node RPC URL (default: http://localhost:8332)
  --address <addr>   Reasoning reward address (required)
  --threads <n>      Number of solver threads (default: all CPUs)
  --seeds <n>        Seeds to try per round (default: 10000)
```

## Troubleshooting

- Miner can't connect: verify the node URL is reachable: `curl -s <node_url>/rpc -d '{"method":"getmininginfo","params":[],"id":1}'`
- No blocks found: this is normal. With higher network difficulty, blocks take longer. Check logs for "SAT solved" progress.
- Check logs: `tail -20 ~/.nous/mainnet/miner.log`

## Safety

- **Mainnet**: Real NOUS with value. Back up your wallet.
- **Testnet**: For testing only, no real value.
- Private keys stored in wallet.dat — do not share.
- CPU-based solver. Uses all available cores by default (configurable with --threads).
- Coinbase maturity: mined coins require 100 block confirmations before spending (testnet: 10).

---
name: nous-reasoner
description: >
  Mine NOUS cryptocurrency by solving NP-complete 3-SAT problems.
  Use when the user wants to mine NOUS, check balance, send NOUS,
  or learn about the NOUS blockchain.
  Requires Go 1.21+ and network access to testnet seed nodes.
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
          go build -o nousd ./cmd/nousd/ &&
          go build -o nous-cli ./cmd/nous-cli/ &&
          mkdir -p ~/.nous/bin &&
          cp nousd nous-cli ~/.nous/bin/ &&
          rm -rf /tmp/nous-build
        bins: ["~/.nous/bin/nousd", "~/.nous/bin/nous-cli"]
        label: "Build NOUS daemon and CLI from source"
allowed-tools: Bash Read Write
---

# NOUS Reasoner

Mine NOUS — the first cryptocurrency where every block requires
solving an NP-complete problem.

## What is NOUS

NOUS is a proof-of-work blockchain where mining = solving randomly
generated 3-SAT formulas. Instead of brute-force hashing (Bitcoin),
each block attempt requires genuine logical reasoning.

- Block time: 150 seconds
- Reward: 1 NOUS per block, forever (no halving)
- Consensus: Cogito Consensus (3-SAT + SHA-256 + ASERT difficulty)
- SAT parameters: 256 variables, 986 clauses (ratio 3.85)

**Networks:**
- Mainnet: Ports 8333 (P2P) / 8332 (RPC), launched 2026-03-07 15:00 CST
- Testnet: Ports 9333 (P2P) / 9332 (RPC), for testing only

**Seed Nodes:**
- seed1.nouschain.org (80.78.26.7)
- seed2.nouschain.org (80.78.25.211)
- seed3.nouschain.org (80.78.26.244)

## Setup

### First-time installation

1. Check if nousd exists:

```bash
~/.nous/bin/nousd --version 2>/dev/null || echo "NOT_INSTALLED"
```

2. If not installed, build from source:

```bash
cd /tmp && rm -rf nous-build && \
  git clone https://github.com/nous-reasoner/nous.git nous-build && \
  cd nous-build && \
  go build -o nousd ./cmd/nousd/ && \
  go build -o nous-cli ./cmd/nous-cli/ && \
  mkdir -p ~/.nous/bin && \
  cp nousd nous-cli ~/.nous/bin/ && \
  rm -rf /tmp/nous-build
```

3. Create data directory and wallet:

```bash
# Mainnet
mkdir -p ~/.nous/mainnet
~/.nous/bin/nous-cli --walletfile ~/.nous/mainnet/wallet.dat --walletpass default createwallet

# Testnet
mkdir -p ~/.nous/testnet
~/.nous/bin/nous-cli --walletfile ~/.nous/testnet/wallet.dat --walletpass default createwallet
```

4. Note the address printed (starts with `nous1q...`). This is the mining reward address.

## Commands

### Create wallet

```bash
# Mainnet
~/.nous/bin/nous-cli --walletfile ~/.nous/mainnet/wallet.dat --walletpass default createwallet

# Testnet
~/.nous/bin/nous-cli --walletfile ~/.nous/testnet/wallet.dat --walletpass default createwallet
```

### Get address

```bash
# Mainnet
~/.nous/bin/nous-cli --walletfile ~/.nous/mainnet/wallet.dat --walletpass default getaddress

# Testnet
~/.nous/bin/nous-cli --walletfile ~/.nous/testnet/wallet.dat --walletpass default getaddress
```

### Start mining

```bash
# Mainnet (default)
~/.nous/bin/nousd \
  --reason \
  --datadir ~/.nous/mainnet \
  --key ~/.nous/mainnet/wallet.dat \
  --password default \
  --seeds seed1.nouschain.org:8333,seed2.nouschain.org:8333,seed3.nouschain.org:8333 \
  > ~/.nous/mainnet/nousd.log 2>&1 &
echo $! > ~/.nous/mainnet/nousd.pid

# Testnet
~/.nous/bin/nousd \
  --testnet \
  --reason \
  --datadir ~/.nous/testnet \
  --key ~/.nous/testnet/wallet.dat \
  --password default \
  --seeds seed1.nouschain.org:9333,seed2.nouschain.org:9333,seed3.nouschain.org:9333 \
  > ~/.nous/testnet/nousd.log 2>&1 &
echo $! > ~/.nous/testnet/nousd.pid
```

After starting, wait 10 seconds then verify:

```bash
# Mainnet
sleep 10 && ~/.nous/bin/nous-cli --rpcport 8332 getmininginfo

# Testnet
sleep 10 && ~/.nous/bin/nous-cli --rpcport 9332 getmininginfo
```

### Stop mining

```bash
# Mainnet
kill $(cat ~/.nous/mainnet/nousd.pid 2>/dev/null) 2>/dev/null && echo "Stopped" || echo "Not running"

# Testnet
kill $(cat ~/.nous/testnet/nousd.pid 2>/dev/null) 2>/dev/null && echo "Stopped" || echo "Not running"
```

### Check balance

```bash
# Mainnet
~/.nous/bin/nous-cli --rpcport 8332 getbalance

# Testnet
~/.nous/bin/nous-cli --rpcport 9332 getbalance
```

Or check a specific address:

```bash
# Mainnet
~/.nous/bin/nous-cli --rpcport 8332 getbalance nous1q...

# Testnet
~/.nous/bin/nous-cli --rpcport 9332 getbalance nous1q...
```

### Check mining status

```bash
# Mainnet
~/.nous/bin/nous-cli --rpcport 8332 getmininginfo

# Testnet
~/.nous/bin/nous-cli --rpcport 9332 getmininginfo
```

### Get block count

```bash
# Mainnet
~/.nous/bin/nous-cli --rpcport 8332 getblockcount

# Testnet
~/.nous/bin/nous-cli --rpcport 9332 getblockcount
```

### Get block info

```bash
# Mainnet
~/.nous/bin/nous-cli --rpcport 8332 getblock <height>

# Testnet
~/.nous/bin/nous-cli --rpcport 9332 getblock <height>
```

### Get peer info

```bash
# Mainnet
~/.nous/bin/nous-cli --rpcport 8332 getpeerinfo

# Testnet
~/.nous/bin/nous-cli --rpcport 9332 getpeerinfo
```

### Send NOUS

```bash
# Mainnet
~/.nous/bin/nous-cli \
  --rpcport 8332 \
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

When mining starts, tell the user:

```
NOUS Reasoner started!
  Network: mainnet (or testnet)
  Each block attempt solves a 256-variable, 986-clause 3-SAT formula.
  Reward: 1 NOUS per block (150 second target).
  Seeds: seed1.nouschain.org, seed2.nouschain.org, seed3.nouschain.org

  Say "check mining status" or "stop mining".
```

When showing balance:

```
nous1q...: X.XXXXXXXX NOUS
```

When showing mining status:

```
Mining Status:
  Height:       1234
  Difficulty:   0x1f2288e4
  Mempool:      0 txs
  Reasoning:    active
```

## Troubleshooting

- Port in use: `lsof -i :8333` or `lsof -i :8332` (mainnet), `lsof -i :9333` or `lsof -i :9332` (testnet)
- Another instance: `ps aux | grep nousd`
- Logs: `tail -20 ~/.nous/testnet/nousd.log`
- No peers: `nc -zv seed1.nouschain.org 8333` (mainnet) or `nc -zv seed1.nouschain.org 9333` (testnet)
- Sync issues: delete data and restart:
  `rm -rf ~/.nous/testnet/blocks ~/.nous/testnet/chaintip.dat`

## CLI Reference

```
nous-cli [flags] <command> [args]

Flags:
  --testnet              Use testnet instead of mainnet (ports 9333/9332)
  --rpchost <host>       RPC server host (default: localhost)
  --rpcport <port>       RPC server port (default: 8332, testnet: 9332)
  --walletfile <path>    wallet file path (default: ~/.nous/wallet.dat)
  --walletpass <pass>    wallet password
  --json                 output in JSON format

Commands:
  createwallet           create a new wallet
  getaddress             show primary address
  newaddress             generate a new address
  getbalance [address]   get balance (wallet or address)
  send <address> <amount> send NOUS to address
  getblockcount          get current block height
  getblock <height>      get block by height
  getmininginfo          get mining information
  getpeerinfo            get connected peers
  version                show version
```

## Safety

- **Mainnet**: Real NOUS with value. Use carefully. Back up your wallet.
- **Testnet**: For testing only, no real value.
- Private keys stored in wallet.dat — do not share.
- Uses 1 CPU core, ~50MB memory. Low system impact.
- Coinbase maturity: mined coins require 100 block confirmations before spending (testnet: 10).

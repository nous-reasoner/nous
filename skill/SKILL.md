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
- Testnet seeds: 80.78.26.7:9333 and 80.78.25.211:9333

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
mkdir -p ~/.nous/testnet
~/.nous/bin/nous-cli --walletfile ~/.nous/testnet/wallet.dat --walletpass default createwallet
```

4. Note the address printed (starts with `nous1q...`). This is the mining reward address.

## Commands

### Create wallet

```bash
~/.nous/bin/nous-cli --walletfile ~/.nous/testnet/wallet.dat --walletpass default createwallet
```

### Get address

```bash
~/.nous/bin/nous-cli --walletfile ~/.nous/testnet/wallet.dat --walletpass default getaddress
```

### Start mining

```bash
~/.nous/bin/nousd \
  --testnet \
  --reason \
  --datadir ~/.nous/testnet \
  --key ~/.nous/testnet/wallet.dat \
  --password default \
  --seeds 80.78.26.7:9333,80.78.25.211:9333 \
  > ~/.nous/testnet/nousd.log 2>&1 &
echo $! > ~/.nous/testnet/nousd.pid
```

After starting, wait 10 seconds then verify:

```bash
sleep 10 && ~/.nous/bin/nous-cli --rpcport 9332 getmininginfo
```

### Stop mining

```bash
kill $(cat ~/.nous/testnet/nousd.pid 2>/dev/null) 2>/dev/null && echo "Stopped" || echo "Not running"
```

### Check balance

```bash
~/.nous/bin/nous-cli --rpcport 9332 getbalance
```

Or check a specific address:

```bash
~/.nous/bin/nous-cli --rpcport 9332 getbalance nous1q...
```

### Check mining status

```bash
~/.nous/bin/nous-cli --rpcport 9332 getmininginfo
```

### Get block count

```bash
~/.nous/bin/nous-cli --rpcport 9332 getblockcount
```

### Get block info

```bash
~/.nous/bin/nous-cli --rpcport 9332 getblock <height>
```

### Get peer info

```bash
~/.nous/bin/nous-cli --rpcport 9332 getpeerinfo
```

### Send NOUS

```bash
~/.nous/bin/nous-cli \
  --rpcport 9332 \
  --walletfile ~/.nous/testnet/wallet.dat \
  --walletpass default \
  send <to_address> <amount>
```

Example: send 0.5 NOUS:

```bash
~/.nous/bin/nous-cli \
  --rpcport 9332 \
  --walletfile ~/.nous/testnet/wallet.dat \
  --walletpass default \
  send nous1qxyz... 0.5
```

### List unspent outputs

```bash
~/.nous/bin/nous-cli --rpcport 9332 --json getbalance nous1q...
```

## Display Format

When mining starts, tell the user:

```
NOUS Reasoner started!
  Network: testnet
  Each block attempt solves a 256-variable, 986-clause 3-SAT formula.
  Reward: 1 NOUS per block (150 second target).
  Seeds: 80.78.26.7:9333, 80.78.25.211:9333

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

- Port in use: `lsof -i :9333` or `lsof -i :9332`
- Another instance: `ps aux | grep nousd`
- Logs: `tail -20 ~/.nous/testnet/nousd.log`
- No peers: `nc -zv 80.78.26.7 9333`
- Sync issues: delete data and restart:
  `rm -rf ~/.nous/testnet/blocks ~/.nous/testnet/chaintip.dat`

## CLI Reference

```
nous-cli [flags] <command> [args]

Flags:
  --rpchost <host>       RPC server host (default: localhost)
  --rpcport <port>       RPC server port (default: 9332)
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

- Testnet only. NOUS on testnet have no real value.
- Private keys stored in wallet.dat — do not share.
- Uses 1 CPU core, ~50MB memory. Low system impact.
- Coinbase maturity: mined coins require 100 block confirmations before spending.

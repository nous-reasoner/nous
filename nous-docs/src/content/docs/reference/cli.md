---
title: CLI Flags
description: Command-line options for NOUS node and miner.
---

## Node (`nousd`)

```bash
nousd [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--port` | `8333` | P2P network port |
| `--rpcport` | `8332` | JSON-RPC API port |
| `--datadir` | `~/.nous` | Data directory |
| `--seeds` | | Comma-separated seed node addresses |
| `--testnet` | `false` | Run on testnet |

### Example

```bash
nousd --port 8333 --rpcport 8332 \
  --seeds seed1.nouschain.org:8333,seed2.nouschain.org:8333,seed3.nouschain.org:8333
```

## Miner

The miner is bundled inside the NOUS Reasoner app and is not typically run standalone. For reference:

```bash
miner [flags]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--node` | | Node RPC URL |
| `--address` | | Mining reward address |
| `--solver` | `probsat` | Solver: `probsat`, `ai-guided`, `pure-ai`, `custom` |
| `--ai-provider` | `anthropic` | AI provider: `anthropic`, `openai` |
| `--api-key` | | API key for AI provider |
| `--model` | `claude-sonnet-4-6` | AI model name |
| `--base-url` | | Custom API base URL |
| `--script` | | Path to custom solver script |

### Example

```bash
miner --node http://rpc.nouschain.org/api \
  --address nous1q8xsvw4sjn4880snr9h24yk4vwflav5fja24dhn \
  --solver probsat
```

## Wallet Backend

The wallet backend communicates via stdin/stdout JSON-RPC with the Electron app. It is not intended to be used directly.

```bash
wallet-backend [-node URL]
```

| Flag | Default | Description |
|------|---------|-------------|
| `-node` | `http://rpc.nouschain.org/api` | Node RPC URL |

Wallet data is stored at `~/.nous-wallet/wallet.dat`.

---
title: Run a Full Node
description: Build and run a NOUS full node to support the network, validate blocks, and sync the blockchain.
---

Running a full node strengthens the NOUS network by independently validating every block and transaction. You don't need to mine — a full node helps with decentralization, block relay, and lets you query the blockchain directly.

## Requirements

- **OS**: Linux (x64), macOS, or Windows 10+ (x64)
- **Go**: 1.24 or later
- **RAM**: 2 GB minimum
- **Disk**: 1 GB (grows over time)
- **Network**: Open port `8333` (TCP) for P2P connections

## Build from Source

```bash
# Clone the repository
git clone https://github.com/nous-reasoner/nous.git
cd nous

# Build the node binary
go build -o nousd ./cmd/nousd/
```

This produces a `nousd` binary in the current directory.

## Run the Node

Start `nousd` with seed nodes so it can discover and connect to the network:

```bash
./nousd \
  --port 8333 \
  --rpcport 8332 \
  --datadir ~/.nous \
  --seeds seed1.nouschain.org:8333,seed2.nouschain.org:8333,seed3.nouschain.org:8333
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--port` | `8333` | P2P listen port. Other nodes connect to this port |
| `--rpcport` | `8332` | JSON-RPC API port (local queries) |
| `--datadir` | `~/.nous` | Directory for blockchain data |
| `--seeds` | — | Comma-separated seed node addresses (required for first sync) |

### What happens on first run

1. The node connects to seed nodes via P2P
2. Block download begins from the genesis block
3. Every block and transaction is independently verified
4. Once synced, the node relays new blocks to other peers

You'll see output like:
```
sync: starting from peer seed1.nouschain.org:8333 (height 5400, our height 0)
sync: accepted block 0000003a... at height 1
sync: accepted block 00000127... at height 2
...
```

Initial sync may take some time depending on your connection speed. The node automatically catches up to the latest block.

## Verify Your Node

Once running, check your node status via RPC:

```bash
# Check current block height
curl -s -X POST http://127.0.0.1:8332/rpc \
  -H "Content-Type: application/json" \
  -d '{"method":"getmininginfo","params":[],"id":1}'

# Check connected peers
curl -s -X POST http://127.0.0.1:8332/rpc \
  -H "Content-Type: application/json" \
  -d '{"method":"getpeerinfo","params":null,"id":1}'
```

A healthy node should show:
- **Block height** matching the network (check [explorer.nouschain.org](https://explorer.nouschain.org))
- **At least 1 peer** connected

## Run as a Background Service

To keep your node running after closing the terminal:

```bash
nohup ./nousd \
  --port 8333 \
  --rpcport 8332 \
  --datadir ~/.nous \
  --seeds seed1.nouschain.org:8333,seed2.nouschain.org:8333,seed3.nouschain.org:8333 \
  > nousd.log 2>&1 &
```

Check logs anytime with:
```bash
tail -f nousd.log
```

### Systemd service (recommended for Linux)

Create `/etc/systemd/system/nousd.service`:

```ini
[Unit]
Description=NOUS Full Node
After=network.target

[Service]
Type=simple
User=nous
ExecStart=/usr/local/bin/nousd \
  --port 8333 \
  --rpcport 8332 \
  --datadir /home/nous/.nous \
  --seeds seed1.nouschain.org:8333,seed2.nouschain.org:8333,seed3.nouschain.org:8333
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

Then enable and start:
```bash
sudo systemctl enable nousd
sudo systemctl start nousd
sudo journalctl -u nousd -f  # view logs
```

## Firewall

Your node needs port **8333** open for inbound P2P connections. Without this, your node can connect out to others but others cannot connect to you.

```bash
# UFW (Ubuntu/Debian)
sudo ufw allow 8333/tcp

# firewalld (CentOS/RHEL)
sudo firewall-cmd --permanent --add-port=8333/tcp
sudo firewall-cmd --reload
```

The RPC port (8332) should **NOT** be exposed to the public internet — it is for local use only.

## Mine with Your Own Node

Once your node is synced, you can point the NOUS Reasoner app or CLI miner to your local node instead of the public RPC:

```bash
# CLI miner example
./miner --node http://127.0.0.1:8332/rpc \
  --address nous1q... \
  --solver probsat
```

In the Reasoner GUI, set the node URL to `http://127.0.0.1:8332/rpc` in settings.

## Troubleshooting

### Node stuck syncing / not making progress

- Check that port 8333 is open and reachable
- Verify you have at least 1 peer: use the `getpeerinfo` RPC call above
- Restart the node — it will resume sync from where it left off (blockchain data is persisted)
- If your log shows many `orphan block` messages, your node may be syncing from a peer with an incompatible chain. The node will automatically switch to a better peer after a few retries

### Disconnected from seed nodes

Seed nodes may occasionally restart for maintenance. Your node will **automatically reconnect** — no action needed. If you use systemd with `Restart=always`, your node recovers from any interruption automatically.

If you stay disconnected for more than a few minutes:

```bash
# Check your peers
curl -s -X POST http://127.0.0.1:8332/rpc \
  -H "Content-Type: application/json" \
  -d '{"method":"getpeerinfo","params":null,"id":1}'
```

If you have 0 peers, restart your node. It will reconnect to seeds on startup.

### No peers connecting

- Make sure `--seeds` is set correctly with all three seeds:
  `seed1.nouschain.org:8333,seed2.nouschain.org:8333,seed3.nouschain.org:8333`
- Check your firewall allows outbound connections on port 8333
- If behind NAT, configure port forwarding for port 8333

### Node crashes or unexpected behavior

- Check `nousd.log` for error messages
- Make sure you're running the latest version: `git pull && go build -o nousd ./cmd/nousd/`

### Need help?

Join the NOUS community on [Discord](https://discord.gg/naun4tbD) to ask questions, report issues, or discuss development. You can also open an issue on [GitHub](https://github.com/nous-reasoner/nous/issues).

## Updating Your Node

When a new version is released:

```bash
cd nous
git pull
go build -o nousd ./cmd/nousd/

# Restart your node
kill $(pgrep -x nousd)
# Then start again with the same command as above
```

Non-consensus updates (RPC changes, display fixes, P2P optimizations) are backward compatible — your node will continue to work with older versions on the network. Consensus updates will be announced in advance on Discord with clear upgrade instructions.

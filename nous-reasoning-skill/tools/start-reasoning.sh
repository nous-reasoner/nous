#!/bin/bash
# Start the NOUS reasoner (miner)
# Usage: start-reasoning.sh <address> [node_url] [mainnet|testnet]

set -e

ADDRESS="$1"
NODE_URL="$2"
NETWORK="${3:-mainnet}"
MINER="$HOME/.nous/bin/miner"

if [ -z "$ADDRESS" ]; then
    echo "Error: Address required. Usage: start-reasoning.sh <nous1q...> [node_url] [mainnet|testnet]"
    exit 1
fi

if [ ! -f "$MINER" ]; then
    echo "Error: miner not found at $MINER. Run install first."
    exit 1
fi

if [ "$NETWORK" = "testnet" ]; then
    DATA_DIR="$HOME/.nous/testnet"
    NODE_URL="${NODE_URL:-http://localhost:9332}"
else
    DATA_DIR="$HOME/.nous/mainnet"
    NODE_URL="${NODE_URL:-https://seed1.nouschain.org}"
fi

mkdir -p "$DATA_DIR"

# Check if already running
if [ -f "$DATA_DIR/miner.pid" ]; then
    PID=$(cat "$DATA_DIR/miner.pid")
    if kill -0 "$PID" 2>/dev/null; then
        echo "Reasoner already running (PID $PID)"
        exit 0
    fi
fi

"$MINER" \
    --node "$NODE_URL" \
    --address "$ADDRESS" \
    > "$DATA_DIR/miner.log" 2>&1 &

echo $! > "$DATA_DIR/miner.pid"
echo "Reasoner started (PID $!)"
echo "  Node: $NODE_URL"
echo "  Address: $ADDRESS"
echo "  Log: $DATA_DIR/miner.log"

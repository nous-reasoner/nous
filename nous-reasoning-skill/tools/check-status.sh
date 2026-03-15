#!/bin/bash
# Check NOUS network and reasoning status
# Usage: check-status.sh [mainnet|testnet]

set -e

NETWORK="${1:-mainnet}"
CLI="$HOME/.nous/bin/nous-cli"

if [ ! -f "$CLI" ]; then
    echo "Error: nous-cli not found at $CLI. Run install first."
    exit 1
fi

if [ "$NETWORK" = "testnet" ]; then
    RPC_HOST="localhost"
    RPC_PORT="9332"
    DATA_DIR="$HOME/.nous/testnet"
else
    RPC_HOST="seed1.nouschain.org"
    RPC_PORT="8332"
    DATA_DIR="$HOME/.nous/mainnet"
fi

echo "=== Network Status ==="
"$CLI" --rpchost "$RPC_HOST" --rpcport "$RPC_PORT" getmininginfo

echo ""
echo "=== Reasoner Status ==="
if [ -f "$DATA_DIR/miner.pid" ]; then
    PID=$(cat "$DATA_DIR/miner.pid")
    if kill -0 "$PID" 2>/dev/null; then
        echo "Running (PID $PID)"
        echo "Recent log:"
        tail -5 "$DATA_DIR/miner.log" 2>/dev/null || echo "(no log)"
    else
        echo "Not running (stale PID file)"
    fi
else
    echo "Not running"
fi

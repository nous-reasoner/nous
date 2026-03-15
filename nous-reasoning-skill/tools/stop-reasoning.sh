#!/bin/bash
# Stop the NOUS reasoner
# Usage: stop-reasoning.sh [mainnet|testnet]

NETWORK="${1:-mainnet}"

if [ "$NETWORK" = "testnet" ]; then
    DATA_DIR="$HOME/.nous/testnet"
else
    DATA_DIR="$HOME/.nous/mainnet"
fi

if [ -f "$DATA_DIR/miner.pid" ]; then
    PID=$(cat "$DATA_DIR/miner.pid")
    if kill "$PID" 2>/dev/null; then
        rm -f "$DATA_DIR/miner.pid"
        echo "Reasoner stopped (PID $PID)"
    else
        rm -f "$DATA_DIR/miner.pid"
        echo "Reasoner not running (stale PID file removed)"
    fi
else
    echo "Reasoner not running (no PID file)"
fi

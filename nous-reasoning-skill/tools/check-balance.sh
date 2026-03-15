#!/bin/bash
# Check NOUS balance for an address
# Usage: check-balance.sh <address> [mainnet|testnet]

set -e

ADDRESS="$1"
NETWORK="${2:-mainnet}"
CLI="$HOME/.nous/bin/nous-cli"

if [ ! -f "$CLI" ]; then
    echo "Error: nous-cli not found at $CLI. Run install first."
    exit 1
fi

if [ "$NETWORK" = "testnet" ]; then
    RPC_HOST="localhost"
    RPC_PORT="9332"
else
    RPC_HOST="seed1.nouschain.org"
    RPC_PORT="8332"
fi

if [ -z "$ADDRESS" ]; then
    "$CLI" --rpchost "$RPC_HOST" --rpcport "$RPC_PORT" getbalance
else
    "$CLI" --rpchost "$RPC_HOST" --rpcport "$RPC_PORT" getbalance "$ADDRESS"
fi

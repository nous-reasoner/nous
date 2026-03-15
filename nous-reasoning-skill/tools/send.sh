#!/bin/bash
# Send NOUS to an address
# Usage: send.sh <to_address> <amount> [mainnet|testnet]

set -e

TO_ADDRESS="$1"
AMOUNT="$2"
NETWORK="${3:-mainnet}"
CLI="$HOME/.nous/bin/nous-cli"

if [ -z "$TO_ADDRESS" ] || [ -z "$AMOUNT" ]; then
    echo "Usage: send.sh <to_address> <amount> [mainnet|testnet]"
    exit 1
fi

if [ ! -f "$CLI" ]; then
    echo "Error: nous-cli not found at $CLI. Run install first."
    exit 1
fi

if [ "$NETWORK" = "testnet" ]; then
    RPC_HOST="localhost"
    RPC_PORT="9332"
    WALLET_DIR="$HOME/.nous/testnet"
else
    RPC_HOST="seed1.nouschain.org"
    RPC_PORT="8332"
    WALLET_DIR="$HOME/.nous/mainnet"
fi

if [ ! -f "$WALLET_DIR/wallet.dat" ]; then
    echo "Error: No wallet found at $WALLET_DIR/wallet.dat. Run create-wallet first."
    exit 1
fi

"$CLI" \
    --rpchost "$RPC_HOST" --rpcport "$RPC_PORT" \
    --walletfile "$WALLET_DIR/wallet.dat" \
    --walletpass default \
    send "$TO_ADDRESS" "$AMOUNT"

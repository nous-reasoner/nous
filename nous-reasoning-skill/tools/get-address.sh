#!/bin/bash
# Get the primary wallet address
# Usage: get-address.sh [mainnet|testnet]

set -e

NETWORK="${1:-mainnet}"
CLI="$HOME/.nous/bin/nous-cli"

if [ ! -f "$CLI" ]; then
    echo "Error: nous-cli not found at $CLI. Run install first."
    exit 1
fi

if [ "$NETWORK" = "testnet" ]; then
    WALLET_DIR="$HOME/.nous/testnet"
else
    WALLET_DIR="$HOME/.nous/mainnet"
fi

if [ ! -f "$WALLET_DIR/wallet.dat" ]; then
    echo "Error: No wallet found. Run create-wallet first."
    exit 1
fi

"$CLI" --walletfile "$WALLET_DIR/wallet.dat" --walletpass default getaddress

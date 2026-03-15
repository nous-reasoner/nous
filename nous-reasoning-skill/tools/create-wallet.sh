#!/bin/bash
# Create a new NOUS wallet
# Usage: create-wallet.sh [mainnet|testnet]

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

mkdir -p "$WALLET_DIR"

if [ -f "$WALLET_DIR/wallet.dat" ]; then
    echo "Wallet already exists at $WALLET_DIR/wallet.dat"
    "$CLI" --walletfile "$WALLET_DIR/wallet.dat" --walletpass default getaddress
    exit 0
fi

"$CLI" --walletfile "$WALLET_DIR/wallet.dat" --walletpass default createwallet

#!/bin/bash
# Install NOUS Reasoner: builds miner and CLI from source
set -e

echo "Building NOUS Reasoner..."

cd /tmp && rm -rf nous-build
git clone https://github.com/nous-reasoner/nous.git nous-build
cd nous-build
go build -o miner ./cmd/miner/
go build -o nous-cli ./cmd/nous-cli/
mkdir -p ~/.nous/bin
cp miner nous-cli ~/.nous/bin/
rm -rf /tmp/nous-build

echo "Installed to ~/.nous/bin/"
echo "  miner:    $(~/.nous/bin/miner --help 2>&1 | head -1)"
echo "  nous-cli: $(~/.nous/bin/nous-cli version 2>/dev/null || echo 'ready')"

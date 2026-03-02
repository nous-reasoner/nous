#!/bin/bash
# NOUS testnet seed node deployment script
# Usage: ssh root@<VPS_IP> 'bash -s' < scripts/deploy.sh

set -e

echo "=== NOUS Testnet Seed Node Deployment ==="

# Install Go if not present
if ! command -v go &> /dev/null; then
    echo "Installing Go 1.24..."
    wget -q https://go.dev/dl/go1.24.3.linux-amd64.tar.gz
    rm -rf /usr/local/go
    tar -C /usr/local -xzf go1.24.3.linux-amd64.tar.gz
    rm go1.24.3.linux-amd64.tar.gz
    echo 'export PATH=$PATH:/usr/local/go/bin' >> /etc/profile
    export PATH=$PATH:/usr/local/go/bin
fi

echo "Go version: $(go version)"

# Create nous user
if ! id -u nous &>/dev/null; then
    useradd -m -s /bin/bash nous
fi

# Clone or update repo
NOUS_DIR=/home/nous/nous
if [ -d "$NOUS_DIR" ]; then
    cd $NOUS_DIR
    git pull origin main
else
    git clone https://github.com/nous-reasoner/nous.git $NOUS_DIR
    cd $NOUS_DIR
fi

# Build
export PATH=$PATH:/usr/local/go/bin
go build -o /usr/local/bin/nousd ./cmd/nousd/
go build -o /usr/local/bin/nous-cli ./cmd/nous-cli/

echo "Build complete: $(nousd --version)"

# Create data directory
mkdir -p /home/nous/.nous
chown -R nous:nous /home/nous

# Create wallet
if [ ! -f /home/nous/.nous/wallet.dat ]; then
    nous-cli --walletfile /home/nous/.nous/wallet.dat --walletpass "$(openssl rand -hex 16)" createwallet
    echo "Wallet created. SAVE THE PASSWORD!"
fi

# Create systemd service
cat > /etc/systemd/system/nousd.service << 'SVCEOF'
[Unit]
Description=NOUS Testnet Seed Node
After=network.target

[Service]
Type=simple
User=nous
ExecStart=/usr/local/bin/nousd \
    --testnet \
    --reason \
    --port 9333 \
    --rpcport 9332 \
    --datadir /home/nous/.nous \
    --key /home/nous/.nous/wallet.dat \
    --password WALLET_PASSWORD_HERE
Restart=always
RestartSec=10
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
SVCEOF

echo ""
echo "=== Deployment complete ==="
echo "Next steps:"
echo "  1. Edit /etc/systemd/system/nousd.service — set wallet password"
echo "  2. systemctl daemon-reload"
echo "  3. systemctl enable nousd"
echo "  4. systemctl start nousd"
echo "  5. journalctl -u nousd -f  (watch logs)"
echo "  6. Open firewall: ufw allow 9333/tcp && ufw allow 9332/tcp"

---
title: Download & Install
description: Get the NOUS Reasoner app.
---

## Download

Get the latest release from GitHub:

**[Download NOUS Reasoner](https://github.com/nous-reasoner/nous/releases/latest)**

| Platform | File |
|----------|------|
| macOS (Apple Silicon) | `NOUS Reasoner-x.x.x-arm64.dmg` |
| Linux x64 | `NOUS Reasoner-x.x.x.AppImage` |

## macOS Installation

1. Download the `.dmg` file
2. Open the DMG and drag **NOUS Reasoner** to Applications
3. On first launch, macOS will block it (unsigned app). Right-click the app and select **Open**, then click **Open** in the dialog
4. The app will start with the Reasoning tab active

## Linux Installation

1. Download the `.AppImage` file
2. Make it executable:
   ```bash
   chmod +x "NOUS Reasoner-*.AppImage"
   ```
3. Run it:
   ```bash
   ./"NOUS Reasoner-*.AppImage"
   ```

## Build from Source

Requirements: Go 1.24+, Node.js 20+

```bash
# Clone the repository
git clone https://github.com/nous-reasoner/nous.git
cd nous

# Build the miner backend
cd nous-miner-gui/backend
go build -o miner .

# Build the wallet backend
cd ../../nous-wallet/backend
go build -o wallet-backend .
cp wallet-backend ../../nous-miner-gui/backend/

# Install and run
cd ../../nous-miner-gui
npm install
npm start
```

## System Requirements

- macOS 12+ (Apple Silicon) or Linux x64
- 4GB RAM minimum
- Internet connection for RPC communication

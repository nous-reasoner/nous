---
title: Create a Wallet
description: Set up your NOUS wallet to send, receive, and manage funds.
---

The NOUS Reasoner app includes a built-in HD wallet.

## Create a New Wallet

1. Open NOUS Reasoner and go to the **Wallet** tab
2. Click **Create New Wallet**
3. Set a password (minimum 6 characters)
4. Choose word count: **12 words** (standard) or **24 words** (extra security)
5. **Write down your recovery phrase** on paper. Never store it digitally.
6. Verify 3 random words to confirm
7. Done! Your wallet is created with one address

## Wallet Features

### Multiple Addresses
- Click **Addresses** on the home screen
- Click **+ New Address** to derive additional addresses
- Click **Use** to switch the active address
- Each address has its own balance and transaction history

### HD Derivation
Your wallet uses BIP44 hierarchical deterministic derivation:
```
m/44'/999'/0'/0/{index}
```

All addresses are derived from your recovery phrase. If you restore from the phrase, you get the same addresses back.

### Encryption
Your wallet file is encrypted with:
- **scrypt** key derivation (N=32768, r=8, p=1)
- **AES-256-GCM** authenticated encryption
- Stored at `~/.nous-wallet/wallet.dat`

### Lock & Unlock
- The wallet locks when you close the app
- Enter your password to unlock
- While locked, your keys are not in memory

## Import an Existing Wallet

If you have a recovery phrase from another NOUS wallet:

1. Open the Wallet tab
2. Click **Import Wallet**
3. Enter your 12 or 24 word recovery phrase
4. Set a password
5. Your first address is automatically derived

## Address Format

NOUS uses **Bech32m** encoding with the `nous` prefix:
```
nous1q8xsvw4sjn4880snr9h24yk4vwflav5fja24dhn
```

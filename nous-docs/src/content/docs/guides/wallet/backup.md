---
title: Backup & Recovery
description: How to back up and restore your NOUS wallet.
---

## Your Recovery Phrase

Your 12 or 24 word recovery phrase is the master key to your wallet. With it, you can restore all HD-derived addresses and funds.

### Best Practices

- Write it on paper, not digitally
- Store in a secure location (safe, safety deposit box)
- Never share it with anyone
- Never enter it on a website
- Consider splitting it across multiple locations

## View Your Recovery Phrase

1. Go to **Wallet** tab
2. Click **Backup**
3. Your recovery phrase is displayed
4. Click **Copy All** if needed

## Restore a Wallet

1. Open NOUS Reasoner (fresh install or after deleting wallet data)
2. Click **Import Wallet**
3. Enter your recovery phrase
4. Set a new password
5. Your first address is automatically derived
6. Derive additional addresses as needed

## Wallet File Location

Your encrypted wallet is stored at:
```
~/.nous-wallet/wallet.dat
```

This file is encrypted with AES-256-GCM. Without your password, it cannot be read.

## What Gets Backed Up

| Data | Backed up by mnemonic? | Backed up in wallet file? |
|------|----------------------|--------------------------|
| HD addresses | Yes | Yes |
| Imported keys | No | Yes |
| Transaction history | No | Yes |
| Settings | No | No |

**Important**: Imported private keys are only in the wallet file. If you lose the file and only have the mnemonic, imported keys are lost.

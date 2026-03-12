---
title: Quick Start
description: From download to your first block in 5 minutes.
---

Get reasoning on the NOUS network in 5 minutes.

## 1. Download & Open

Download [NOUS Reasoner](https://github.com/nous-reasoner/nous/releases/latest) and install it (see [Download & Install](/getting-started/install/)).

## 2. Create a Wallet

1. Switch to the **Wallet** tab
2. Click **Create New Wallet**
3. Set a password (min 6 characters)
4. Choose **12 Words** or **24 Words**
5. **Write down your recovery phrase** — this is the only way to recover your wallet
6. Verify 3 random words to confirm you saved them
7. Your wallet is ready with one address

## 3. Start Reasoning

1. Switch to the **Reasoning** tab
2. Your wallet address is automatically filled in
3. Select a solver:
   - **ProbSAT** (recommended) — fast, no setup required
   - **AI-Guided ProbSAT** — uses an LLM for smarter initial guesses (requires API key)
4. Click **Start Reasoning**

You should see logs like:
```
Mining block 4800 (difficulty: 0x1e0d2806) with ProbSAT
Progress: 500 SAT solved (5000.0/s), seed=493, no PoW match yet
```

## 4. Wait for a Block

With ProbSAT at ~5000 solves/second and the current difficulty, finding a block takes some time. The average block time is **150 seconds** across the entire network.

When you find a block:
```
Block found! Height: 4801, Hash: 0000003a... (42.5s, 2500 SAT solved)
```

You earn **1 NOUS** per block found.

## 5. Check Your Balance

Your balance updates automatically in the Wallet tab. Mined NOUS requires **100 block confirmations** (~4 hours) before it can be spent.

## What's Next?

- [Create additional addresses](/getting-started/wallet/) for organization
- [Send NOUS](/guides/wallet/send-receive/) to other addresses
- [Try AI-Guided mode](/guides/reasoning/ai-guided/) for smarter solving
- [Write a custom solver](/guides/reasoning/custom-solver/) for maximum advantage

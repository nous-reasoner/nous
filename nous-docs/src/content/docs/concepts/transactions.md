---
title: Transactions & Fees
description: How NOUS transactions work.
---

## Transaction Model

NOUS uses a **UTXO model** (Unspent Transaction Output), similar to Bitcoin:

- Each transaction consumes one or more inputs (previous unspent outputs)
- Each transaction creates one or more outputs
- The difference between inputs and outputs is the fee

## Transaction Structure

```
Transaction {
  Version:  1
  ChainID:  mainnet
  Inputs:   [previous outputs being spent]
  Outputs:  [new outputs being created]
}
```

Each input includes a signature proving ownership. Each output locks funds to a public key hash.

## Fees

- Fees = total input value - total output value
- The block producer collects all fees
- There is no protocol-enforced minimum fee
- A **dust limit** of 546 base units applies to regular outputs
- **OP_RETURN** outputs (data-only, unspendable) are exempt from the dust limit

## OP_RETURN Messages

Transactions can include an OP_RETURN output to store arbitrary data on-chain:
- Maximum size determined by transaction size limits
- Value must be 0 (unspendable)
- Used for on-chain messaging and data anchoring

## Confirmation

- Regular transactions: spendable after **1 confirmation**
- Coinbase (mining reward): spendable after **100 confirmations** (~4 hours)

## Scripts

NOUS uses Pay-to-Public-Key-Hash (P2PKH) scripts:
- Lock script: `OP_DUP OP_HASH160 <pubkey_hash> OP_EQUALVERIFY OP_CHECKSIG`
- Unlock script: `<signature> <pubkey>`

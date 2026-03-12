---
title: Tokenomics
description: NOUS supply schedule, emission, and economic design.
---

## Emission

NOUS has the simplest possible emission schedule:

> **1 NOUS per block, forever.**

There is no halving, no cap, no premine, no founder allocation.

## Supply Growth

| Timeframe | Blocks | NOUS Produced |
|-----------|--------|---------------|
| Per hour | ~24 | ~24 NOUS |
| Per day | ~576 | ~576 NOUS |
| Per year | ~210,240 | ~210,240 NOUS |
| Per century | ~21,024,000 | ~21M NOUS |

Every century of operation produces approximately **21 million NOUS** — a deliberate echo of Bitcoin's total supply, recast as a recurring measure rather than a terminal limit.

## Why No Halving?

Bitcoin's halving creates urgency and scarcity but also creates structural inequality: early miners receive exponentially more reward per block than future miners. NOUS rejects this:

- A human reasoner in 2025 earns 1 NOUS per block
- An AI reasoner in 2525 earns 1 NOUS per block
- Every generation stands on equal ground

## Inflation Rate

The inflation rate starts relatively high and approaches zero asymptotically, but never reaches it:

| Year | Total Supply | Annual Inflation |
|------|-------------|-----------------|
| 1 | ~210K | — |
| 10 | ~2.1M | ~10% |
| 50 | ~10.5M | ~2% |
| 100 | ~21M | ~1% |
| 1000 | ~210M | ~0.1% |

This creates a system that is deflationary in practice (as adoption grows faster than emission) while remaining fair to future participants.

## Transaction Fees

Transaction fees are paid by the sender and collected by the block producer. Fees are market-determined — there is no minimum fee at the protocol level (dust limit of 546 base units applies to outputs).

Fee revenue supplements the block reward, providing additional incentive for miners as the relative value of 1 NOUS per block changes over time.

## Base Units

1 NOUS = 100,000,000 base units (similar to Bitcoin's satoshis).

---
title: Difficulty Adjustment
description: How NOUS maintains consistent block times.
---

## Overview

NOUS targets a **150-second average block time**. The difficulty adjusts automatically to maintain this rate regardless of how much reasoning power joins or leaves the network.

## Mechanism

NOUS uses a single **hash target** — identical in concept to Bitcoin's difficulty:

- Lower target = harder (hash must be smaller)
- Higher target = easier (hash can be larger)

The 3-SAT parameters (256 variables, 986 clauses) **never change**. Only the hash target adjusts.

## Adjustment Algorithm

The difficulty adjusts based on the time taken to produce recent blocks:

- If blocks are produced **too fast** (< 150s average), the target decreases (harder)
- If blocks are produced **too slow** (> 150s average), the target increases (easier)

This creates a self-regulating system where adding more miners doesn't produce blocks faster — it only increases the difficulty.

## Implications

- **Hardware upgrades** make the network harder, not faster
- **Better algorithms** make the network harder, not faster
- **AI breakthroughs** make the network harder, not faster
- The block rate stays constant at ~150 seconds

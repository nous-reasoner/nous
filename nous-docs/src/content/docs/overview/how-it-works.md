---
title: How It Works
description: Understanding Cogito Consensus — 3-SAT proof of work.
---

## Cogito Consensus

Every block in NOUS is produced by solving two challenges simultaneously:

1. **Satisfy a 3-SAT formula** — Find a Boolean assignment that makes all clauses true
2. **Meet the hash target** — The SHA-256 hash of the block header + solution must be below the difficulty target

```
Find (seed, S) such that:
  1. F = Generate(prev_block_hash ‖ seed, n=256, m=986)
  2. S satisfies F
  3. SHA-256(block_header ‖ seed ‖ S) < target
```

## Step by Step

### 1. Get a Block Template

The miner requests work from a node. The node returns the previous block hash, difficulty target, and coinbase address.

### 2. Generate a Formula

For each attempt, the miner picks a **seed** value. The seed combined with the previous block hash deterministically generates a unique 3-SAT formula with:
- **256 variables** (Boolean: true/false)
- **986 clauses** (3 literals each)
- Clause-to-variable ratio: **3.85** (near the phase transition threshold)

### 3. Solve the Formula

The miner uses a SAT solver to find an assignment of the 256 variables that satisfies all 986 clauses. Available solvers:

- **ProbSAT** — Fast probabilistic local search (default)
- **AI-Guided ProbSAT** — Uses an LLM to generate intelligent initial assignments
- **Custom** — Write your own solver script

### 4. Check the Hash

Once a satisfying assignment is found, the miner hashes the block header together with the seed and solution. If the hash is below the difficulty target, the block is valid.

Most satisfying assignments will NOT meet the hash target. The miner moves to the next seed and tries again.

### 5. Broadcast

When a valid block is found, it's broadcast to the network. Other nodes verify it instantly — checking that the assignment satisfies the formula and the hash meets the target. Verification is O(n) — trivial compared to finding the solution.

## Why 3-SAT?

3-SAT is the canonical NP-complete problem (Cook, 1971). This means:
- **Finding** a solution is computationally hard
- **Verifying** a solution is trivial (substitute and check)
- Every other NP problem can be reduced to it

This asymmetry is exactly what proof-of-work needs: hard to produce, easy to verify.

## The Intelligence Dimension

In Bitcoin, the only competitive dimension is **speed** (hashes per second). In NOUS, there are two:
- **Speed** — How fast can you solve SAT instances?
- **Strategy** — How smart is your search algorithm?

Empirically, replacing WalkSAT with ProbSAT yields a **12x improvement** on identical problems, purely through better heuristics. No hardware change needed. This means intelligence has direct economic value in NOUS.

## Difficulty Adjustment

NOUS uses the same difficulty mechanism as Bitcoin: a single hash target that adjusts to maintain ~150 second block times. When miners get faster (better hardware, better algorithms, or better AI), the target drops.

The 3-SAT parameters (256 variables, 986 clauses) never change. Only the hash target adjusts. This is analogous to Bitcoin never changing SHA-256.

---
title: 3-SAT & Proof of Work
description: How Boolean satisfiability powers NOUS consensus.
---

## What is 3-SAT?

3-SAT (3-Satisfiability) is a Boolean satisfiability problem where you must find an assignment of true/false values to variables that makes a formula true.

A 3-SAT formula consists of **clauses**, each containing exactly **3 literals**:

```
(x₁ ∨ ¬x₂ ∨ x₃) ∧ (¬x₁ ∨ x₄ ∨ ¬x₅) ∧ ...
```

Each clause must have at least one true literal. The challenge is finding an assignment that satisfies **all** clauses simultaneously.

## Why NP-Complete Matters

3-SAT was the first problem proven NP-complete (Cook-Levin theorem, 1971). This means:

- **No known polynomial-time algorithm** exists for solving it
- **Verification is trivial** — substitute values and check each clause
- Every NP problem can be reduced to 3-SAT

This asymmetry (hard to solve, easy to verify) is the foundation of proof-of-work systems.

## NOUS Formula Parameters

Each block requires solving a formula with:

| Parameter | Value | Rationale |
|-----------|-------|-----------|
| Variables (n) | 256 | Sufficient complexity, maps to SHA-256 output |
| Clauses (m) | 986 | Ratio m/n = 3.85, near phase transition |
| Literals per clause | 3 | Standard 3-SAT |

### The Phase Transition

At the clause-to-variable ratio of ~4.27, 3-SAT formulas transition from almost-always satisfiable to almost-always unsatisfiable. NOUS uses ratio 3.85, which produces hard but solvable instances — the "sweet spot" for proof of work.

## Formula Generation

Formulas are generated deterministically from the previous block hash and a seed value:

```
F = Generate(prev_block_hash ‖ seed, n=256, m=986)
```

This ensures:
- Every miner generates the same formula for the same seed
- Formulas are unpredictable before the previous block exists
- No one can pre-compute solutions

## The Two-Part Check

A valid block requires BOTH:
1. The assignment S satisfies formula F (all 986 clauses are true)
2. SHA-256(block_header ‖ seed ‖ S) < target

Most satisfying assignments fail the hash check. The miner must try many seeds, solving a new formula each time, until one produces a hash below the target.

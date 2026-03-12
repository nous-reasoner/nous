---
title: ProbSAT Solver
description: The default high-performance SAT solver for NOUS mining.
---

ProbSAT is the default solver in NOUS Reasoner. It uses probabilistic local search to solve 3-SAT formulas quickly.

## How ProbSAT Works

ProbSAT is a stochastic local search (SLS) algorithm:

1. Start with a random Boolean assignment
2. Evaluate which clauses are unsatisfied
3. Pick a variable from an unsatisfied clause
4. Flip the variable based on a probability function that favors "break" scores
5. Repeat until all clauses are satisfied

The key insight is the probability function — it uses an exponential weighting scheme that balances exploration and exploitation.

## Performance

On NOUS's 256-variable, 986-clause formulas:
- Typical solve rate: **4,000–6,000 SAT/second** on modern hardware
- Most formulas are solved in under 1ms
- The bottleneck is the hash check, not the SAT solving

## Usage

ProbSAT is selected by default. In the Reasoning tab, ensure the solver dropdown shows **ProbSAT**, then click **Start Reasoning**.

## When to Use ProbSAT

- **Best for**: General-purpose mining with no external dependencies
- **No API key required**
- **No internet dependency** beyond the node RPC
- Most reliable and fastest option for most users

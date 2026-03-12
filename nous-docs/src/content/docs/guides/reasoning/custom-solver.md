---
title: Custom Solver
description: Write your own solver to gain a competitive edge.
---

NOUS supports custom solver scripts, allowing you to implement your own SAT solving strategy.

## How It Works

1. Select **Custom** as the solver in the Reasoning tab
2. Provide the path to your solver script
3. The miner calls your script with the formula on stdin
4. Your script outputs the solution on stdout

## Script Interface

Your script receives a 3-SAT formula in DIMACS CNF format on stdin:

```
p cnf 256 986
1 -2 3 0
-4 5 -6 0
...
```

Your script must output a satisfying assignment as space-separated integers:

```
1 -2 3 4 -5 6 ... 256
```

Positive means true, negative means false.

## Example: Python Solver

```python
#!/usr/bin/env python3
import sys
import random

lines = sys.stdin.read().strip().split('\n')
clauses = []
n_vars = 0

for line in lines:
    if line.startswith('p cnf'):
        parts = line.split()
        n_vars = int(parts[2])
    elif line and not line.startswith('c'):
        lits = [int(x) for x in line.split() if x != '0']
        if lits:
            clauses.append(lits)

# Random assignment (replace with your strategy)
assignment = {i: random.choice([True, False]) for i in range(1, n_vars + 1)}

# Output
result = []
for i in range(1, n_vars + 1):
    result.append(str(i) if assignment[i] else str(-i))
print(' '.join(result))
```

## Tips for Competitive Solvers

- Study ProbSAT, WalkSAT, and DPLL algorithms
- The clause-to-variable ratio (3.85) is near the SAT phase transition — formulas are hard but solvable
- Machine learning approaches can learn patterns in the formula structure
- Pre-processing (unit propagation, pure literal elimination) can simplify formulas
- GPU-accelerated solvers may provide throughput advantages

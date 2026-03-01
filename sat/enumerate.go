package sat

import (
	"crypto/sha256"
	"math/rand"
	"time"
)

// WalkSATEnumerate uses WalkSAT local search to enumerate distinct satisfying
// assignments of f (n variables) within the given timeout.
// Returns the number of distinct solutions found and up to the first 1000 solutions.
func WalkSATEnumerate(f Formula, n int, timeout time.Duration) (count int, solutions []Assignment) {
	if len(f) == 0 || n == 0 {
		return 0, nil
	}

	deadline := time.Now().Add(timeout)
	seen := make(map[[32]byte]struct{})
	const maxStored = 1000
	const maxFlips = 100000
	const noise = 0.3 // probability of random walk vs greedy

	// Build variable→clause index for fast unsatisfied clause lookup.
	varClauses := make([][]int, n)
	for ci, c := range f {
		for _, lit := range c {
			if lit.Var < n {
				varClauses[lit.Var] = append(varClauses[lit.Var], ci)
			}
		}
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	for time.Now().Before(deadline) {
		// Random initial assignment.
		a := make(Assignment, n)
		for i := range a {
			a[i] = rng.Intn(2) == 1
		}

		for flip := 0; flip < maxFlips && time.Now().Before(deadline); flip++ {
			// Find unsatisfied clauses.
			var unsat []int
			for ci, c := range f {
				if !clauseSat(c, a) {
					unsat = append(unsat, ci)
				}
			}

			if len(unsat) == 0 {
				// Found a solution — record if new.
				h := hashAssignment(a)
				if _, dup := seen[h]; !dup {
					seen[h] = struct{}{}
					count++
					if len(solutions) < maxStored {
						cp := make(Assignment, n)
						copy(cp, a)
						solutions = append(solutions, cp)
					}
				}
				// Perturb: flip a few random variables to continue searching.
				flips := 1 + rng.Intn(3)
				for k := 0; k < flips; k++ {
					a[rng.Intn(n)] = !a[rng.Intn(n)]
				}
				continue
			}

			// Pick a random unsatisfied clause.
			ci := unsat[rng.Intn(len(unsat))]
			c := f[ci]

			if rng.Float64() < noise {
				// Random walk: flip a random variable in the clause.
				lit := c[rng.Intn(len(c))]
				if lit.Var < n {
					a[lit.Var] = !a[lit.Var]
				}
			} else {
				// Greedy: flip the variable in the clause that minimizes break count.
				bestVar := -1
				bestBreak := len(f) + 1
				for _, lit := range c {
					v := lit.Var
					if v >= n {
						continue
					}
					// Count how many currently satisfied clauses would break.
					a[v] = !a[v]
					breaks := 0
					for _, ci2 := range varClauses[v] {
						if !clauseSat(f[ci2], a) {
							breaks++
						}
					}
					a[v] = !a[v] // undo
					if breaks < bestBreak {
						bestBreak = breaks
						bestVar = v
					}
				}
				if bestVar >= 0 {
					a[bestVar] = !a[bestVar]
				}
			}
		}
	}

	return count, solutions
}

func clauseSat(c Clause, a Assignment) bool {
	for _, lit := range c {
		if lit.Var >= len(a) {
			continue
		}
		val := a[lit.Var]
		if lit.Neg {
			val = !val
		}
		if val {
			return true
		}
	}
	return false
}

func hashAssignment(a Assignment) [32]byte {
	b := make([]byte, (len(a)+7)/8)
	for i, v := range a {
		if v {
			b[i/8] |= 1 << (uint(i) % 8)
		}
	}
	return sha256.Sum256(b)
}

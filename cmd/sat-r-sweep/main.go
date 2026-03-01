package main

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/nous-chain/nous/sat"
)

const N = 128

var ratios = []float64{3.5, 3.8, 4.0, 4.2}

func main() {
	for i, r := range ratios {
		if i > 0 {
			fmt.Println()
		}
		m := int(math.Ceil(float64(N) * r))
		fmt.Printf("=== r=%.1f  (n=%d, m=%d) ===\n", r, N, m)
		fmt.Println()

		runSingleSolve(r, m)
		fmt.Println()
		runEnumerate(r)
	}
}

// --- Part 1: Single solve (10 seeds, 5s timeout) ---

func runSingleSolve(r float64, m int) {
	fmt.Printf("  --- Single Solve (10 seeds, timeout=5s) ---\n")
	fmt.Printf("  %-6s | %-12s | %-8s | %s\n", "Seed", "Time", "Result", "Flips")
	fmt.Printf("  %s\n", strings.Repeat("-", 50))

	solved := 0
	for i := 0; i < 10; i++ {
		seed := makeSeed(N, r, i)
		f := sat.GenerateFormula(seed, N, r)

		start := time.Now()
		a, flips, ok := walkSATSolve(f, N, 5*time.Second)
		elapsed := time.Since(start)

		result := "TIMEOUT"
		if ok {
			if sat.Verify(f, a) {
				result = "OK"
				solved++
			} else {
				result = "WRONG"
			}
		}

		fmt.Printf("  %-6d | %s | %-8s | %d\n", i, fmtDur(elapsed), result, flips)
	}
	fmt.Printf("  %s\n", strings.Repeat("-", 50))
	fmt.Printf("  Solved: %d/10\n", solved)
}

// --- Part 2: Enumeration (2 seeds, 30s each) ---

func runEnumerate(r float64) {
	fmt.Printf("  --- Enumeration (2 seeds, 30s each) ---\n")
	fmt.Printf("  %-6s | %-12s | %-12s | %s\n", "Seed", "Unique Sols", "Sol/sec", "Verified")
	fmt.Printf("  %s\n", strings.Repeat("-", 55))

	for i := 0; i < 2; i++ {
		seed := makeSeed(N, r, i)
		f := sat.GenerateFormula(seed, N, r)

		start := time.Now()
		count, solutions := sat.WalkSATEnumerate(f, N, 30*time.Second)
		elapsed := time.Since(start)

		ver := "n/a"
		if count > 0 && len(solutions) > 0 {
			check := len(solutions)
			if check > 50 {
				check = 50
			}
			allOK := true
			for j := 0; j < check; j++ {
				if !sat.Verify(f, solutions[j]) {
					allOK = false
					break
				}
			}
			if allOK {
				ver = "YES"
			} else {
				ver = "NO"
			}
		}

		rate := float64(count) / elapsed.Seconds()
		fmt.Printf("  %-6d | %-12d | %-12.1f | %s\n", i, count, rate, ver)
	}
}

// --- Local WalkSAT (first solution only) ---

func walkSATSolve(f sat.Formula, n int, timeout time.Duration) (sat.Assignment, int, bool) {
	deadline := time.Now().Add(timeout)
	const maxFlips = 1000000
	const noise = 0.3

	varClauses := make([][]int, n)
	for ci, c := range f {
		for _, lit := range c {
			if lit.Var < n {
				varClauses[lit.Var] = append(varClauses[lit.Var], ci)
			}
		}
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	totalFlips := 0

	for time.Now().Before(deadline) {
		a := make(sat.Assignment, n)
		for i := range a {
			a[i] = rng.Intn(2) == 1
		}

		for flip := 0; flip < maxFlips && time.Now().Before(deadline); flip++ {
			totalFlips++

			var unsat []int
			for ci, c := range f {
				if !clauseSat(c, a) {
					unsat = append(unsat, ci)
				}
			}

			if len(unsat) == 0 {
				return a, totalFlips, true
			}

			ci := unsat[rng.Intn(len(unsat))]
			c := f[ci]

			if rng.Float64() < noise {
				lit := c[rng.Intn(len(c))]
				if lit.Var < n {
					a[lit.Var] = !a[lit.Var]
				}
			} else {
				bestVar := -1
				bestBreak := len(f) + 1
				for _, lit := range c {
					v := lit.Var
					if v >= n {
						continue
					}
					a[v] = !a[v]
					breaks := 0
					for _, ci2 := range varClauses[v] {
						if !clauseSat(f[ci2], a) {
							breaks++
						}
					}
					a[v] = !a[v]
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
	return nil, totalFlips, false
}

func clauseSat(c sat.Clause, a sat.Assignment) bool {
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

func fmtDur(d time.Duration) string {
	if d < time.Millisecond {
		return fmt.Sprintf("%10.0fus", float64(d.Microseconds()))
	}
	if d < time.Second {
		return fmt.Sprintf("%10.2fms", float64(d.Microseconds())/1000.0)
	}
	return fmt.Sprintf("%10.3fs", d.Seconds())
}

func makeSeed(n int, r float64, idx int) [32]byte {
	buf := make([]byte, 16)
	binary.BigEndian.PutUint32(buf[0:4], uint32(n))
	binary.BigEndian.PutUint64(buf[4:12], math.Float64bits(r))
	binary.BigEndian.PutUint32(buf[12:16], uint32(idx))
	return sha256.Sum256(buf)
}

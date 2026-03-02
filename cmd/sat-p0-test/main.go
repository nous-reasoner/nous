package main

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"
	"time"

	"nous/sat"
)

const (
	N = 128
	R = 3.0
)

func main() {
	test1SingleSolve()
	fmt.Println()
	test2EnumerateRate()
	fmt.Println()
	test3Variance()
	fmt.Println()
	test4UnitPropagation()
}

// ============================================================
// Test 1: Single solve time (20 seeds)
// ============================================================
func test1SingleSolve() {
	fmt.Println("=== Test 1: Single Solve Time (n=128, r=3.0, 20 seeds) ===")
	fmt.Println()
	fmt.Printf("%-6s | %-12s | %-8s | %s\n", "Seed", "Time", "Verified", "Flips")
	fmt.Println(strings.Repeat("-", 50))

	var times []float64
	solved := 0

	for i := 0; i < 20; i++ {
		seed := makeSeed(N, R, i)
		f := sat.GenerateFormula(seed, N, R)

		start := time.Now()
		a, flips, ok := walkSATSolve(f, N, 60*time.Second)
		elapsed := time.Since(start)

		ver := "-"
		if ok {
			if sat.Verify(f, a) {
				ver = "YES"
				solved++
			} else {
				ver = "NO"
			}
		} else {
			ver = "TIMEOUT"
		}

		ms := float64(elapsed.Microseconds()) / 1000.0
		times = append(times, ms)
		fmt.Printf("%-6d | %10.3fms | %-8s | %d\n", i, ms, ver, flips)
	}

	fmt.Println(strings.Repeat("-", 50))
	fmt.Printf("Solved: %d/20\n", solved)
	if len(times) > 0 {
		sort.Float64s(times)
		fmt.Printf("Min: %.3fms  Max: %.3fms  Median: %.3fms\n",
			times[0], times[len(times)-1], median(times))
	}
}

// ============================================================
// Test 2: Enumeration rate (3 seeds, 30s each)
// ============================================================
func test2EnumerateRate() {
	fmt.Println("=== Test 2: Solution Enumeration Rate (n=128, r=3.0, 30s each) ===")
	fmt.Println()
	fmt.Printf("%-6s | %-7s | %-12s | %-12s | %s\n",
		"Seed", "Clauses", "Unique Sols", "Sol/sec", "Verified")
	fmt.Println(strings.Repeat("-", 60))

	for i := 0; i < 3; i++ {
		seed := makeSeed(N, R, i)
		f := sat.GenerateFormula(seed, N, R)

		start := time.Now()
		count, solutions := sat.WalkSATEnumerate(f, N, 30*time.Second)
		elapsed := time.Since(start)

		ver := "n/a"
		if count > 0 && len(solutions) > 0 {
			allOK := true
			check := len(solutions)
			if check > 100 {
				check = 100
			}
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
		fmt.Printf("%-6d | %-7d | %-12d | %-12.1f | %s\n",
			i, len(f), count, rate, ver)
	}
}

// ============================================================
// Test 3: Variance test (1 seed, 100 runs)
// ============================================================
func test3Variance() {
	fmt.Println("=== Test 3: Solve Time Variance (n=128, r=3.0, 1 seed, 100 runs) ===")
	fmt.Println()

	seed := makeSeed(N, R, 0)
	f := sat.GenerateFormula(seed, N, R)

	var times []float64
	solved := 0

	for i := 0; i < 100; i++ {
		start := time.Now()
		_, _, ok := walkSATSolve(f, N, 10*time.Second)
		elapsed := time.Since(start)

		ms := float64(elapsed.Microseconds()) / 1000.0
		times = append(times, ms)
		if ok {
			solved++
		}
	}

	sort.Float64s(times)

	avg := mean(times)
	med := median(times)
	sd := stddev(times, avg)
	p10 := percentile(times, 10)
	p90 := percentile(times, 90)
	p99 := percentile(times, 99)

	fmt.Printf("Solved: %d/100\n", solved)
	fmt.Println()
	fmt.Printf("%-12s | %s\n", "Stat", "Value")
	fmt.Println(strings.Repeat("-", 30))
	fmt.Printf("%-12s | %.3fms\n", "Min", times[0])
	fmt.Printf("%-12s | %.3fms\n", "P10", p10)
	fmt.Printf("%-12s | %.3fms\n", "Median", med)
	fmt.Printf("%-12s | %.3fms\n", "Mean", avg)
	fmt.Printf("%-12s | %.3fms\n", "P90", p90)
	fmt.Printf("%-12s | %.3fms\n", "P99", p99)
	fmt.Printf("%-12s | %.3fms\n", "Max", times[len(times)-1])
	fmt.Printf("%-12s | %.3fms\n", "StdDev", sd)

	// Distribution histogram
	fmt.Println()
	fmt.Println("Distribution (10 buckets):")
	printHistogram(times)
}

// ============================================================
// Test 4: Unit propagation analysis (5 seeds)
// ============================================================
func test4UnitPropagation() {
	fmt.Println("=== Test 4: Unit Propagation Analysis (n=128, r=3.0, 5 seeds) ===")
	fmt.Println()
	fmt.Printf("%-6s | %-7s | %-12s | %-12s | %s\n",
		"Seed", "Clauses", "Unit Clauses", "Pure Lits", "UP Solves?")
	fmt.Println(strings.Repeat("-", 60))

	for i := 0; i < 5; i++ {
		seed := makeSeed(N, R, i)
		f := sat.GenerateFormula(seed, N, R)

		unitCount := 0
		for _, c := range f {
			if len(c) == 1 {
				unitCount++
			}
		}

		// Count pure literals: variables that appear only positive or only negative.
		posAppear := make([]bool, N)
		negAppear := make([]bool, N)
		for _, c := range f {
			for _, lit := range c {
				if lit.Var < N {
					if lit.Neg {
						negAppear[lit.Var] = true
					} else {
						posAppear[lit.Var] = true
					}
				}
			}
		}
		pureCount := 0
		for v := 0; v < N; v++ {
			if posAppear[v] != negAppear[v] {
				pureCount++
			}
		}

		// Try unit propagation: assign unit clauses, propagate, check if solved.
		upSolves := tryUnitProp(f, N)

		fmt.Printf("%-6d | %-7d | %-12d | %-12d | %s\n",
			i, len(f), unitCount, pureCount, boolStr(upSolves))
	}
}

// ============================================================
// Local WalkSAT: stops after finding the first solution
// ============================================================
func walkSATSolve(f sat.Formula, n int, timeout time.Duration) (sat.Assignment, int, bool) {
	deadline := time.Now().Add(timeout)
	const maxFlips = 1000000
	const noise = 0.3

	// Build var→clause index
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
		// Random initial assignment
		a := make(sat.Assignment, n)
		for i := range a {
			a[i] = rng.Intn(2) == 1
		}

		for flip := 0; flip < maxFlips && time.Now().Before(deadline); flip++ {
			totalFlips++

			// Find unsatisfied clauses
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

// tryUnitProp attempts to solve via unit propagation + pure literal elimination only.
func tryUnitProp(f sat.Formula, n int) bool {
	// Work on a copy of clauses represented as sets of literals.
	type litKey struct {
		v   int
		neg bool
	}

	assigned := make(map[int]bool) // var → value
	decided := make(map[int]bool)  // var → has been decided

	// Iteratively propagate
	changed := true
	clauses := make([][]litKey, len(f))
	for i, c := range f {
		cl := make([]litKey, len(c))
		for j, lit := range c {
			cl[j] = litKey{lit.Var, lit.Neg}
		}
		clauses[i] = cl
	}

	for changed {
		changed = false

		// Unit propagation
		for _, cl := range clauses {
			if cl == nil {
				continue
			}
			// Count unresolved literals
			var unresolved []litKey
			satisfied := false
			for _, lit := range cl {
				if decided[lit.v] {
					val := assigned[lit.v]
					if lit.neg {
						val = !val
					}
					if val {
						satisfied = true
						break
					}
				} else {
					unresolved = append(unresolved, lit)
				}
			}
			if satisfied {
				continue
			}
			if len(unresolved) == 1 {
				// Unit clause: force assignment
				lit := unresolved[0]
				if !decided[lit.v] {
					if lit.neg {
						assigned[lit.v] = false
					} else {
						assigned[lit.v] = true
					}
					decided[lit.v] = true
					changed = true
				}
			}
			if len(unresolved) == 0 && !satisfied {
				return false // conflict
			}
		}

		// Pure literal elimination
		posCount := make(map[int]int)
		negCount := make(map[int]int)
		for _, cl := range clauses {
			if cl == nil {
				continue
			}
			satisfied := false
			for _, lit := range cl {
				if decided[lit.v] {
					val := assigned[lit.v]
					if lit.neg {
						val = !val
					}
					if val {
						satisfied = true
						break
					}
				}
			}
			if satisfied {
				continue
			}
			for _, lit := range cl {
				if !decided[lit.v] {
					if lit.neg {
						negCount[lit.v]++
					} else {
						posCount[lit.v]++
					}
				}
			}
		}
		for v := 0; v < n; v++ {
			if decided[v] {
				continue
			}
			p, ng := posCount[v], negCount[v]
			if p > 0 && ng == 0 {
				assigned[v] = true
				decided[v] = true
				changed = true
			} else if ng > 0 && p == 0 {
				assigned[v] = false
				decided[v] = true
				changed = true
			}
		}
	}

	// Check if all variables decided
	if len(decided) < n {
		return false
	}

	// Verify
	a := make(sat.Assignment, n)
	for v := 0; v < n; v++ {
		a[v] = assigned[v]
	}
	return sat.Verify(f, a)
}

// ============================================================
// Stats helpers
// ============================================================
func mean(v []float64) float64 {
	s := 0.0
	for _, x := range v {
		s += x
	}
	return s / float64(len(v))
}

func median(v []float64) float64 {
	n := len(v)
	if n%2 == 0 {
		return (v[n/2-1] + v[n/2]) / 2
	}
	return v[n/2]
}

func stddev(v []float64, avg float64) float64 {
	s := 0.0
	for _, x := range v {
		d := x - avg
		s += d * d
	}
	return math.Sqrt(s / float64(len(v)))
}

func percentile(sorted []float64, p int) float64 {
	idx := float64(p) / 100.0 * float64(len(sorted)-1)
	lo := int(math.Floor(idx))
	hi := int(math.Ceil(idx))
	if lo == hi || hi >= len(sorted) {
		return sorted[lo]
	}
	frac := idx - float64(lo)
	return sorted[lo]*(1-frac) + sorted[hi]*frac
}

func printHistogram(sorted []float64) {
	lo, hi := sorted[0], sorted[len(sorted)-1]
	buckets := 10
	width := (hi - lo) / float64(buckets)
	if width == 0 {
		fmt.Printf("  All values: %.3fms\n", lo)
		return
	}
	counts := make([]int, buckets)
	for _, v := range sorted {
		b := int((v - lo) / width)
		if b >= buckets {
			b = buckets - 1
		}
		counts[b]++
	}
	maxCount := 0
	for _, c := range counts {
		if c > maxCount {
			maxCount = c
		}
	}
	for i := 0; i < buckets; i++ {
		bLo := lo + float64(i)*width
		bHi := bLo + width
		barLen := 0
		if maxCount > 0 {
			barLen = counts[i] * 40 / maxCount
		}
		fmt.Printf("  [%8.2f-%8.2f) %3d %s\n",
			bLo, bHi, counts[i], strings.Repeat("#", barLen))
	}
}

func boolStr(b bool) string {
	if b {
		return "YES"
	}
	return "NO"
}

func makeSeed(n int, r float64, idx int) [32]byte {
	buf := make([]byte, 16)
	binary.BigEndian.PutUint32(buf[0:4], uint32(n))
	binary.BigEndian.PutUint64(buf[4:12], math.Float64bits(r))
	binary.BigEndian.PutUint32(buf[12:16], uint32(idx))
	return sha256.Sum256(buf)
}

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

	"github.com/nous-chain/nous/sat"
)

const N = 128

func main() {
	// Part 1: Large-scale solve test (r=4.0, 200 seeds)
	fmt.Println("================================================================")
	fmt.Println("  Part 1: Large-scale Solve (n=128, r=4.0, 200 seeds, 10s timeout)")
	fmt.Println("================================================================")
	fmt.Println()
	runLargeSolve(4.0, 200, 10*time.Second)

	// Part 2: Repeated solve stability (r=4.0, 1 seed, 200 runs)
	fmt.Println()
	fmt.Println("================================================================")
	fmt.Println("  Part 2: Repeated Solve Stability (r=4.0, seed=0, 200 runs)")
	fmt.Println("================================================================")
	fmt.Println()
	runRepeatedSolve(4.0, 0, 200, 10*time.Second)

	// Part 3: Comparison r=3.85 vs r=3.9 vs r=4.0 (100 seeds each)
	fmt.Println()
	fmt.Println("================================================================")
	fmt.Println("  Part 3: Ratio Comparison (100 seeds each, 10s timeout)")
	fmt.Println("================================================================")
	fmt.Println()
	for _, r := range []float64{3.85, 3.9, 4.0} {
		runComparison(r, 100, 10*time.Second)
		fmt.Println()
	}

	// Summary table
	fmt.Println("================================================================")
	fmt.Println("  Summary")
	fmt.Println("================================================================")
	fmt.Println()
	fmt.Printf("  %-6s | %-7s | %-8s | %-10s | %-10s | %-10s | %-10s | %-10s\n",
		"Ratio", "Clauses", "Success", "P50", "P90", "P95", "P99", "Max")
	fmt.Printf("  %s\n", strings.Repeat("-", 90))
	for _, r := range []float64{3.85, 3.9, 4.0} {
		m := int(math.Ceil(float64(N) * r))
		times, solved := collectTimes(r, 100, 10*time.Second)
		sort.Float64s(times)
		fmt.Printf("  %-6.2f | %-7d | %3d/100  | %-10s | %-10s | %-10s | %-10s | %-10s\n",
			r, m, solved,
			fmtMs(pct(times, 50)), fmtMs(pct(times, 90)),
			fmtMs(pct(times, 95)), fmtMs(pct(times, 99)),
			fmtMs(pct(times, 100)))
	}
}

// ============================================================
// Part 1: Large-scale solve
// ============================================================
func runLargeSolve(r float64, count int, timeout time.Duration) {
	m := int(math.Ceil(float64(N) * r))
	fmt.Printf("  r=%.2f, m=%d\n\n", r, m)

	var times []float64
	solved := 0
	over1s := 0
	timeouts := 0

	for i := 0; i < count; i++ {
		seed := makeSeed(N, r, i)
		f := sat.GenerateFormula(seed, N, r)

		start := time.Now()
		a, _, ok := walkSATSolve(f, N, timeout)
		elapsed := time.Since(start)
		ms := float64(elapsed.Microseconds()) / 1000.0

		if ok && sat.Verify(f, a) {
			solved++
			times = append(times, ms)
			if elapsed > time.Second {
				over1s++
			}
		} else {
			timeouts++
			times = append(times, float64(timeout.Milliseconds()))
		}
	}

	sort.Float64s(times)

	fmt.Printf("  Solved:   %d/%d\n", solved, count)
	fmt.Printf("  Timeout:  %d\n", timeouts)
	fmt.Printf("  Over 1s:  %d\n", over1s)
	fmt.Println()
	printStats("  ", times)
	fmt.Println()
	printHistogram("  ", times)
}

// ============================================================
// Part 2: Repeated solve stability
// ============================================================
func runRepeatedSolve(r float64, seedIdx int, runs int, timeout time.Duration) {
	seed := makeSeed(N, r, seedIdx)
	f := sat.GenerateFormula(seed, N, r)
	m := len(f)
	fmt.Printf("  r=%.2f, m=%d, seed=%d\n\n", r, m, seedIdx)

	var times []float64
	solved := 0

	for i := 0; i < runs; i++ {
		start := time.Now()
		a, _, ok := walkSATSolve(f, N, timeout)
		elapsed := time.Since(start)
		ms := float64(elapsed.Microseconds()) / 1000.0

		if ok && sat.Verify(f, a) {
			solved++
			times = append(times, ms)
		} else {
			times = append(times, float64(timeout.Milliseconds()))
		}
	}

	sort.Float64s(times)

	fmt.Printf("  Solved: %d/%d\n\n", solved, runs)
	printStats("  ", times)
	fmt.Println()
	printHistogram("  ", times)
}

// ============================================================
// Part 3: Comparison per ratio
// ============================================================
func runComparison(r float64, count int, timeout time.Duration) {
	m := int(math.Ceil(float64(N) * r))
	fmt.Printf("  --- r=%.2f (m=%d, %d seeds) ---\n", r, m, count)

	var times []float64
	solved := 0

	for i := 0; i < count; i++ {
		seed := makeSeed(N, r, i)
		f := sat.GenerateFormula(seed, N, r)

		start := time.Now()
		a, _, ok := walkSATSolve(f, N, timeout)
		elapsed := time.Since(start)
		ms := float64(elapsed.Microseconds()) / 1000.0

		if ok && sat.Verify(f, a) {
			solved++
			times = append(times, ms)
		} else {
			times = append(times, float64(timeout.Milliseconds()))
		}
	}

	sort.Float64s(times)

	fmt.Printf("  Solved: %d/%d\n", solved, count)
	printStats("  ", times)
}

// collectTimes reruns the same seeds for the summary table.
// Uses cached-friendly deterministic seeds.
func collectTimes(r float64, count int, timeout time.Duration) ([]float64, int) {
	var times []float64
	solved := 0
	for i := 0; i < count; i++ {
		seed := makeSeed(N, r, i)
		f := sat.GenerateFormula(seed, N, r)
		start := time.Now()
		a, _, ok := walkSATSolve(f, N, timeout)
		elapsed := time.Since(start)
		ms := float64(elapsed.Microseconds()) / 1000.0
		if ok && sat.Verify(f, a) {
			solved++
			times = append(times, ms)
		} else {
			times = append(times, float64(timeout.Milliseconds()))
		}
	}
	return times, solved
}

// ============================================================
// WalkSAT (first solution)
// ============================================================
func walkSATSolve(f sat.Formula, n int, timeout time.Duration) (sat.Assignment, int, bool) {
	deadline := time.Now().Add(timeout)
	const maxFlips = 2000000
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

// ============================================================
// Stats & display
// ============================================================
func printStats(prefix string, sorted []float64) {
	if len(sorted) == 0 {
		fmt.Printf("%sNo data\n", prefix)
		return
	}
	avg := mean(sorted)
	sd := stddev(sorted, avg)
	fmt.Printf("%s%-8s %-12s %-12s %-12s %-12s %-12s %-12s %-12s\n",
		prefix, "P50", "P90", "P95", "P99", "Max", "Mean", "StdDev", "Min")
	fmt.Printf("%s%-8s %-12s %-12s %-12s %-12s %-12s %-12s %-12s\n",
		prefix,
		fmtMs(pct(sorted, 50)), fmtMs(pct(sorted, 90)),
		fmtMs(pct(sorted, 95)), fmtMs(pct(sorted, 99)),
		fmtMs(sorted[len(sorted)-1]),
		fmtMs(avg), fmtMs(sd), fmtMs(sorted[0]))
}

func printHistogram(prefix string, sorted []float64) {
	if len(sorted) == 0 {
		return
	}
	// Fixed buckets for readability
	boundaries := []float64{0.1, 1, 5, 10, 50, 100, 500, 1000, 5000, 10000}
	counts := make([]int, len(boundaries)+1)
	for _, v := range sorted {
		placed := false
		for bi, b := range boundaries {
			if v < b {
				counts[bi]++
				placed = true
				break
			}
		}
		if !placed {
			counts[len(boundaries)]++
		}
	}

	labels := make([]string, len(boundaries)+1)
	labels[0] = fmt.Sprintf("< %.0fms", boundaries[0])
	for i := 1; i < len(boundaries); i++ {
		labels[i] = fmt.Sprintf("%.0f-%.0fms", boundaries[i-1], boundaries[i])
	}
	labels[len(boundaries)] = fmt.Sprintf(">= %.0fms", boundaries[len(boundaries)-1])

	maxCount := 0
	for _, c := range counts {
		if c > maxCount {
			maxCount = c
		}
	}

	fmt.Printf("%sDistribution:\n", prefix)
	for i, c := range counts {
		if c == 0 {
			continue
		}
		barLen := 0
		if maxCount > 0 {
			barLen = c * 40 / maxCount
		}
		fmt.Printf("%s  %-16s %4d %s\n", prefix, labels[i], c, strings.Repeat("#", barLen))
	}
}

func mean(v []float64) float64 {
	s := 0.0
	for _, x := range v {
		s += x
	}
	return s / float64(len(v))
}

func stddev(v []float64, avg float64) float64 {
	s := 0.0
	for _, x := range v {
		d := x - avg
		s += d * d
	}
	return math.Sqrt(s / float64(len(v)))
}

func pct(sorted []float64, p int) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if p >= 100 {
		return sorted[len(sorted)-1]
	}
	idx := float64(p) / 100.0 * float64(len(sorted)-1)
	lo := int(math.Floor(idx))
	hi := int(math.Ceil(idx))
	if lo == hi || hi >= len(sorted) {
		return sorted[lo]
	}
	frac := idx - float64(lo)
	return sorted[lo]*(1-frac) + sorted[hi]*frac
}

func fmtMs(ms float64) string {
	if ms < 1 {
		return fmt.Sprintf("%.0fus", ms*1000)
	}
	if ms < 1000 {
		return fmt.Sprintf("%.1fms", ms)
	}
	return fmt.Sprintf("%.2fs", ms/1000)
}

func makeSeed(n int, r float64, idx int) [32]byte {
	buf := make([]byte, 16)
	binary.BigEndian.PutUint32(buf[0:4], uint32(n))
	binary.BigEndian.PutUint64(buf[4:12], math.Float64bits(r))
	binary.BigEndian.PutUint32(buf[12:16], uint32(idx))
	return sha256.Sum256(buf)
}

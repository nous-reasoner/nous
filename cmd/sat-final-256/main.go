package main

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/nous-chain/nous/sat"
)

const (
	N = 256
	R = 3.85
)

func main() {
	test1MassiveSolve()
	fmt.Println()
	test2Stability()
}

// ============================================================
// Test 1: 10000 seeds
// ============================================================
func test1MassiveSolve() {
	const seeds = 10000
	const to = 10 * time.Second
	m := int(math.Ceil(float64(N) * R))

	fmt.Println("================================================================")
	fmt.Printf("  Test 1: Mass Solve (n=%d, r=%.2f, m=%d, %d seeds, timeout=%s)\n", N, R, m, seeds, to)
	fmt.Println("================================================================")
	fmt.Println()

	var times []float64
	solved := 0
	timeouts := 0

	reportInterval := 1000
	start := time.Now()

	for i := 0; i < seeds; i++ {
		seed := makeSeed(N, R, i)
		f := sat.GenerateFormula(seed, N, R)

		t0 := time.Now()
		a, err := sat.ProbSATSolve(f, N, to)
		elapsed := time.Since(t0)
		ms := float64(elapsed.Microseconds()) / 1000.0

		if err == nil && sat.Verify(f, a) {
			solved++
			times = append(times, ms)
		} else {
			timeouts++
		}

		if (i+1)%reportInterval == 0 {
			wallElapsed := time.Since(start)
			fmt.Printf("  [%5d/%d] solved=%d timeout=%d  (wall: %s)\n",
				i+1, seeds, solved, timeouts, wallElapsed.Round(time.Second))
		}
	}

	sort.Float64s(times)
	wallTotal := time.Since(start)

	fmt.Println()
	fmt.Printf("  Total wall time: %s\n", wallTotal.Round(time.Millisecond))
	fmt.Printf("  Solved:   %d/%d (%.2f%%)\n", solved, seeds, float64(solved)/float64(seeds)*100)
	fmt.Printf("  Timeout:  %d\n", timeouts)
	fmt.Println()

	if len(times) > 0 {
		avg := mean(times)
		sd := stddev(times, avg)
		fmt.Printf("  %-8s | %-8s | %-8s | %-8s | %-10s | %-10s | %-10s | %-10s\n",
			"P50", "P90", "P95", "P99", "Max", "Mean", "StdDev", "Min")
		fmt.Printf("  %s\n", strings.Repeat("-", 88))
		fmt.Printf("  %-8s | %-8s | %-8s | %-8s | %-10s | %-10s | %-10s | %-10s\n",
			fmtMs(pct(times, 50)), fmtMs(pct(times, 90)),
			fmtMs(pct(times, 95)), fmtMs(pct(times, 99)),
			fmtMs(times[len(times)-1]), fmtMs(avg), fmtMs(sd), fmtMs(times[0]))
	}
}

// ============================================================
// Test 2: Repeated solve stability (seed=0, 200 runs)
// ============================================================
func test2Stability() {
	const runs = 200
	const to = 10 * time.Second

	seed := makeSeed(N, R, 0)
	f := sat.GenerateFormula(seed, N, R)
	m := len(f)

	fmt.Println("================================================================")
	fmt.Printf("  Test 2: Stability (n=%d, r=%.2f, m=%d, seed=0, %d runs)\n", N, R, m, runs)
	fmt.Println("================================================================")
	fmt.Println()

	var times []float64
	solved := 0

	for i := 0; i < runs; i++ {
		start := time.Now()
		a, err := sat.ProbSATSolve(f, N, to)
		elapsed := time.Since(start)
		ms := float64(elapsed.Microseconds()) / 1000.0

		if err == nil && sat.Verify(f, a) {
			solved++
			times = append(times, ms)
		}
	}

	sort.Float64s(times)

	fmt.Printf("  Solved: %d/%d\n\n", solved, runs)

	if len(times) > 0 {
		avg := mean(times)
		sd := stddev(times, avg)
		fmt.Printf("  %-8s | %-8s | %-8s | %-10s | %-10s | %-10s\n",
			"P50", "P90", "P95", "Max", "Mean", "StdDev")
		fmt.Printf("  %s\n", strings.Repeat("-", 64))
		fmt.Printf("  %-8s | %-8s | %-8s | %-10s | %-10s | %-10s\n",
			fmtMs(pct(times, 50)), fmtMs(pct(times, 90)),
			fmtMs(pct(times, 95)), fmtMs(times[len(times)-1]),
			fmtMs(avg), fmtMs(sd))
	}
}

// ============================================================
// Helpers
// ============================================================
func mean(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
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
	if ms < 0.001 {
		return "0us"
	}
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

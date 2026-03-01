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
	N       = 128
	seeds   = 50
	timeout = 10 * time.Second
)

var ratios = []float64{3.9, 4.0, 4.05, 4.1, 4.15, 4.2}

type result struct {
	r        float64
	m        int
	solved   int
	timeouts int
	times    []float64 // ms, only successful solves
	allTimes []float64 // ms, all (timeout = 10000)
}

func main() {
	var results []result

	for _, r := range ratios {
		m := int(math.Ceil(float64(N) * r))
		fmt.Printf("=== r=%.2f (m=%d, %d seeds, timeout=%s) ===\n", r, m, seeds, timeout)
		fmt.Printf("  %-6s | %-12s | %s\n", "Seed", "Time", "Result")
		fmt.Printf("  %s\n", strings.Repeat("-", 35))

		res := result{r: r, m: m}

		for i := 0; i < seeds; i++ {
			seed := makeSeed(N, r, i)
			f := sat.GenerateFormula(seed, N, r)

			start := time.Now()
			a, err := sat.ProbSATSolve(f, N, timeout)
			elapsed := time.Since(start)
			ms := float64(elapsed.Microseconds()) / 1000.0

			if err == nil && sat.Verify(f, a) {
				res.solved++
				res.times = append(res.times, ms)
				res.allTimes = append(res.allTimes, ms)
				fmt.Printf("  %-6d | %12s | OK\n", i, fmtDur(elapsed))
			} else {
				res.timeouts++
				res.allTimes = append(res.allTimes, float64(timeout.Milliseconds()))
				fmt.Printf("  %-6d | %12s | TIMEOUT\n", i, fmtDur(elapsed))
			}
		}

		sort.Float64s(res.times)
		sort.Float64s(res.allTimes)

		fmt.Printf("  %s\n", strings.Repeat("-", 35))
		fmt.Printf("  Solved: %d/%d  Timeout: %d\n", res.solved, seeds, res.timeouts)
		if len(res.times) > 0 {
			fmt.Printf("  (solved only) P50=%s  P90=%s  P95=%s  Max=%s  Mean=%s\n",
				fmtMs(pct(res.times, 50)), fmtMs(pct(res.times, 90)),
				fmtMs(pct(res.times, 95)), fmtMs(res.times[len(res.times)-1]),
				fmtMs(mean(res.times)))
		}
		fmt.Println()

		results = append(results, res)
	}

	// Summary table
	fmt.Println(strings.Repeat("=", 95))
	fmt.Println("  Summary (n=128, ProbSAT, 50 seeds each, timeout=10s)")
	fmt.Println(strings.Repeat("=", 95))
	fmt.Println()
	fmt.Printf("  %-6s | %-5s | %-8s | %-8s | %-10s | %-10s | %-10s | %-10s | %-10s\n",
		"Ratio", "m", "Solved", "Timeout", "P50", "P90", "P95", "Max", "Mean")
	fmt.Printf("  %s\n", strings.Repeat("-", 91))

	for _, res := range results {
		if len(res.times) == 0 {
			fmt.Printf("  %-6.2f | %-5d | %3d/50   | %-8d | %-10s | %-10s | %-10s | %-10s | %-10s\n",
				res.r, res.m, res.solved, res.timeouts,
				"-", "-", "-", "-", "-")
		} else {
			fmt.Printf("  %-6.2f | %-5d | %3d/50   | %-8d | %-10s | %-10s | %-10s | %-10s | %-10s\n",
				res.r, res.m, res.solved, res.timeouts,
				fmtMs(pct(res.times, 50)), fmtMs(pct(res.times, 90)),
				fmtMs(pct(res.times, 95)), fmtMs(res.times[len(res.times)-1]),
				fmtMs(mean(res.times)))
		}
	}
	fmt.Println()
}

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

func fmtDur(d time.Duration) string {
	if d < time.Millisecond {
		return fmt.Sprintf("%.0fus", float64(d.Microseconds()))
	}
	if d < time.Second {
		return fmt.Sprintf("%.2fms", float64(d.Microseconds())/1000.0)
	}
	return fmt.Sprintf("%.3fs", d.Seconds())
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

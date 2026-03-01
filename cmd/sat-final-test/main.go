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

func main() {
	test1RatioSweep()
	fmt.Println()
	test2ScaleN()
}

// ============================================================
// Test 1: ProbSAT r sweep (n=128)
// ============================================================
func test1RatioSweep() {
	const n = 128
	const seeds = 50
	const to = 10 * time.Second
	ratios := []float64{3.8, 3.85, 3.9, 4.0, 4.1, 4.15, 4.2}

	fmt.Println("================================================================")
	fmt.Println("  Test 1: ProbSAT r sweep (n=128, 50 seeds, timeout=10s)")
	fmt.Println("================================================================")
	fmt.Println()

	type row struct {
		r       float64
		m       int
		solved  int
		total   int
		times   []float64
	}
	var rows []row

	for _, r := range ratios {
		m := int(math.Ceil(float64(n) * r))
		res := row{r: r, m: m, total: seeds}

		for i := 0; i < seeds; i++ {
			seed := makeSeed(n, r, i)
			f := sat.GenerateFormula(seed, n, r)

			start := time.Now()
			a, err := sat.ProbSATSolve(f, n, to)
			elapsed := time.Since(start)
			ms := float64(elapsed.Microseconds()) / 1000.0

			if err == nil && sat.Verify(f, a) {
				res.solved++
				res.times = append(res.times, ms)
			}
		}
		sort.Float64s(res.times)
		rows = append(rows, res)

		fmt.Printf("  r=%.2f done: %d/%d solved\n", r, res.solved, seeds)
	}

	fmt.Println()
	fmt.Printf("  %-6s | %-5s | %-8s | %-10s | %-10s | %-10s | %-10s\n",
		"Ratio", "m", "Solved", "P50", "P90", "Max", "Mean")
	fmt.Printf("  %s\n", strings.Repeat("-", 72))
	for _, r := range rows {
		if len(r.times) == 0 {
			fmt.Printf("  %-6.2f | %-5d | %3d/%-3d  | %-10s | %-10s | %-10s | %-10s\n",
				r.r, r.m, r.solved, r.total, "-", "-", "-", "-")
		} else {
			fmt.Printf("  %-6.2f | %-5d | %3d/%-3d  | %-10s | %-10s | %-10s | %-10s\n",
				r.r, r.m, r.solved, r.total,
				fmtMs(pct(r.times, 50)), fmtMs(pct(r.times, 90)),
				fmtMs(r.times[len(r.times)-1]), fmtMs(mean(r.times)))
		}
	}
}

// ============================================================
// Test 2: ProbSAT n scaling (r=3.85)
// ============================================================
func test2ScaleN() {
	const r = 3.85
	const seeds = 30
	const to = 30 * time.Second
	ns := []int{128, 192, 256, 384, 512, 768, 1024}

	fmt.Println("================================================================")
	fmt.Println("  Test 2: ProbSAT n scaling (r=3.85, 30 seeds, timeout=30s)")
	fmt.Println("================================================================")
	fmt.Println()

	type row struct {
		n       int
		m       int
		solved  int
		total   int
		times   []float64
	}
	var rows []row

	for _, n := range ns {
		m := int(math.Ceil(float64(n) * r))
		res := row{n: n, m: m, total: seeds}

		for i := 0; i < seeds; i++ {
			seed := makeSeed(n, r, i)
			f := sat.GenerateFormula(seed, n, r)

			start := time.Now()
			a, err := sat.ProbSATSolve(f, n, to)
			elapsed := time.Since(start)
			ms := float64(elapsed.Microseconds()) / 1000.0

			if err == nil && sat.Verify(f, a) {
				res.solved++
				res.times = append(res.times, ms)
			}
		}
		sort.Float64s(res.times)
		rows = append(rows, res)

		p50 := "-"
		if len(res.times) > 0 {
			p50 = fmtMs(pct(res.times, 50))
		}
		fmt.Printf("  n=%-5d done: %d/%d solved  (P50=%s)\n", n, res.solved, seeds, p50)
	}

	fmt.Println()
	fmt.Printf("  %-6s | %-6s | %-8s | %-10s | %-10s | %-10s | %-10s\n",
		"N", "m", "Solved", "P50", "P90", "Max", "Mean")
	fmt.Printf("  %s\n", strings.Repeat("-", 72))
	for _, r := range rows {
		if len(r.times) == 0 {
			fmt.Printf("  %-6d | %-6d | %3d/%-3d  | %-10s | %-10s | %-10s | %-10s\n",
				r.n, r.m, r.solved, r.total, "-", "-", "-", "-")
		} else {
			fmt.Printf("  %-6d | %-6d | %3d/%-3d  | %-10s | %-10s | %-10s | %-10s\n",
				r.n, r.m, r.solved, r.total,
				fmtMs(pct(r.times, 50)), fmtMs(pct(r.times, 90)),
				fmtMs(r.times[len(r.times)-1]), fmtMs(mean(r.times)))
		}
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

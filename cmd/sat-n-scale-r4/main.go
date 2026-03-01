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
	R     = 4.0
	seeds = 30
	to    = 60 * time.Second
)

var ns = []int{128, 256, 512, 1024, 2048, 4096}

func main() {
	fmt.Println("================================================================")
	fmt.Println("  ProbSAT n scaling (r=4.0, 30 seeds, timeout=60s)")
	fmt.Println("================================================================")
	fmt.Println()

	type row struct {
		n, m, solved, timeouts int
		times                  []float64
	}
	var rows []row

	for _, n := range ns {
		m := int(math.Ceil(float64(n) * R))
		res := row{n: n, m: m}

		for i := 0; i < seeds; i++ {
			seed := makeSeed(n, R, i)
			f := sat.GenerateFormula(seed, n, R)

			start := time.Now()
			a, err := sat.ProbSATSolve(f, n, to)
			elapsed := time.Since(start)
			ms := float64(elapsed.Microseconds()) / 1000.0

			status := "TIMEOUT"
			if err == nil && sat.Verify(f, a) {
				res.solved++
				res.times = append(res.times, ms)
				status = fmt.Sprintf("%s", fmtMs(ms))
			} else {
				res.timeouts++
			}
			fmt.Printf("  n=%-5d seed=%-3d %s\n", n, i, status)
		}

		sort.Float64s(res.times)
		rows = append(rows, res)
		fmt.Println()
	}

	fmt.Println("================================================================")
	fmt.Println("  Summary")
	fmt.Println("================================================================")
	fmt.Println()
	fmt.Printf("  %-6s | %-6s | %-8s | %-4s | %-10s | %-10s | %-10s | %-10s\n",
		"N", "m", "Solved", "T/O", "P50", "P90", "Max", "Mean")
	fmt.Printf("  %s\n", strings.Repeat("-", 78))

	for _, r := range rows {
		if len(r.times) == 0 {
			fmt.Printf("  %-6d | %-6d | %3d/%-3d  | %-4d | %-10s | %-10s | %-10s | %-10s\n",
				r.n, r.m, r.solved, seeds, r.timeouts, "-", "-", "-", "-")
		} else {
			fmt.Printf("  %-6d | %-6d | %3d/%-3d  | %-4d | %-10s | %-10s | %-10s | %-10s\n",
				r.n, r.m, r.solved, seeds, r.timeouts,
				fmtMs(pct(r.times, 50)), fmtMs(pct(r.times, 90)),
				fmtMs(r.times[len(r.times)-1]), fmtMs(mean(r.times)))
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

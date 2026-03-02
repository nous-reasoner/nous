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
	N       = 128
	timeout = 10 * time.Second
)

var ratios = []float64{3.8, 3.85}

func main() {
	for ri, r := range ratios {
		if ri > 0 {
			fmt.Println()
		}
		m := int(math.Ceil(float64(N) * r))
		fmt.Printf("=== n=%d, r=%.2f, m=%d (20 seeds, timeout=%s) ===\n\n", N, r, m, timeout)

		fmt.Printf("%-6s | %-14s %-14s | %-14s %-14s | %s\n",
			"Seed",
			"WalkSAT", "",
			"ProbSAT", "",
			"Speedup")
		fmt.Printf("%-6s | %-14s %-14s | %-14s %-14s | %s\n",
			"", "Time", "Result", "Time", "Result", "")
		fmt.Println(strings.Repeat("-", 85))

		var wTimes, pTimes []float64
		var wOK, pOK int
		var speedups []float64

		for i := 0; i < 20; i++ {
			seed := makeSeed(N, r, i)
			f := sat.GenerateFormula(seed, N, r)

			// WalkSAT
			wStart := time.Now()
			wA, wErr := walkSATSolveOne(f, N, timeout)
			wElapsed := time.Since(wStart)
			wMs := float64(wElapsed.Microseconds()) / 1000.0
			wRes := "TIMEOUT"
			if wErr == nil && sat.Verify(f, wA) {
				wRes = "OK"
				wOK++
				wTimes = append(wTimes, wMs)
			} else {
				wTimes = append(wTimes, float64(timeout.Milliseconds()))
			}

			// ProbSAT
			pStart := time.Now()
			pA, pErr := sat.ProbSATSolve(f, N, timeout)
			pElapsed := time.Since(pStart)
			pMs := float64(pElapsed.Microseconds()) / 1000.0
			pRes := "TIMEOUT"
			if pErr == nil && sat.Verify(f, pA) {
				pRes = "OK"
				pOK++
				pTimes = append(pTimes, pMs)
			} else {
				pTimes = append(pTimes, float64(timeout.Milliseconds()))
			}

			// Speedup
			sp := ""
			if wRes == "OK" && pRes == "OK" && pMs > 0 {
				ratio := wMs / pMs
				speedups = append(speedups, ratio)
				sp = fmt.Sprintf("%.1fx", ratio)
			} else if wRes == "TIMEOUT" && pRes == "OK" {
				sp = "inf"
			} else if wRes == "OK" && pRes == "TIMEOUT" {
				sp = "0x"
			} else {
				sp = "-"
			}

			fmt.Printf("%-6d | %-14s %-14s | %-14s %-14s | %s\n",
				i, fmtDur(wElapsed), wRes, fmtDur(pElapsed), pRes, sp)
		}

		fmt.Println(strings.Repeat("-", 85))

		// Summary
		sort.Float64s(wTimes)
		sort.Float64s(pTimes)

		fmt.Printf("\n  Summary (r=%.2f):\n", r)
		fmt.Printf("  %-10s | %-8s | %-10s | %-10s | %-10s | %-10s | %-10s\n",
			"Solver", "Success", "P50", "P90", "P95", "Max", "Mean")
		fmt.Printf("  %s\n", strings.Repeat("-", 78))
		fmt.Printf("  %-10s | %4d/20  | %-10s | %-10s | %-10s | %-10s | %-10s\n",
			"WalkSAT", wOK,
			fmtMs(pct(wTimes, 50)), fmtMs(pct(wTimes, 90)),
			fmtMs(pct(wTimes, 95)), fmtMs(pct(wTimes, 100)),
			fmtMs(mean(wTimes)))
		fmt.Printf("  %-10s | %4d/20  | %-10s | %-10s | %-10s | %-10s | %-10s\n",
			"ProbSAT", pOK,
			fmtMs(pct(pTimes, 50)), fmtMs(pct(pTimes, 90)),
			fmtMs(pct(pTimes, 95)), fmtMs(pct(pTimes, 100)),
			fmtMs(mean(pTimes)))

		if len(speedups) > 0 {
			sort.Float64s(speedups)
			fmt.Printf("\n  Speedup (ProbSAT vs WalkSAT, both-solved only, n=%d):\n", len(speedups))
			fmt.Printf("    Median: %.1fx  Mean: %.1fx  Min: %.2fx  Max: %.1fx\n",
				pct(speedups, 50), mean(speedups), speedups[0], speedups[len(speedups)-1])
		}
	}
}

// walkSATSolveOne wraps the existing WalkSATEnumerate to get a single solution.
func walkSATSolveOne(f sat.Formula, n int, timeout time.Duration) (sat.Assignment, error) {
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

	for time.Now().Before(deadline) {
		a := make(sat.Assignment, n)
		for i := range a {
			a[i] = rng.Intn(2) == 1
		}

		for flip := 0; flip < maxFlips && time.Now().Before(deadline); flip++ {
			var unsat []int
			for ci, c := range f {
				if !clauseSat(c, a) {
					unsat = append(unsat, ci)
				}
			}
			if len(unsat) == 0 {
				return a, nil
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
	return nil, fmt.Errorf("timeout")
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

// --- stats helpers ---

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

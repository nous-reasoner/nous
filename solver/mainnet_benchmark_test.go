package solver

import (
	"fmt"
	"math"
	"runtime"
	"testing"
	"time"

	"github.com/nous-chain/nous/crypto"
	"github.com/nous-chain/nous/csp"
)

// ============================================================
// TestMainnetBenchmark: mainnet CSP difficulty benchmark
//
// Demonstrates that brute-force search cannot solve mainnet-scale
// CSP problems, validating the need for AI solvers.
//
// Domain analysis (Standard level):
//   Variables: 8-12, each with domain size ~20-100
//   Search space: ~80^8 = 1.68×10^15 (8 vars) to ~80^12 = 6.87×10^22 (12 vars)
//   At 10^9 checks/sec, brute-force needs ~19 days (8 vars) to ~2.18×10^6 years (12 vars)
// ============================================================

// epochConfig defines CSP parameters for a future difficulty epoch.
type epochConfig struct {
	Name            string
	BaseVariables   int
	ConstraintRatio float64
}

var epochs = []epochConfig{
	{"epoch0 (year 1)", 8, 1.2},
	{"epoch1 (year 2)", 10, 1.3},
	{"epoch2 (year 3)", 12, 1.4},
	{"epoch3 (year 4)", 14, 1.5},
	{"epoch4 (year 5)", 16, 1.6},
}

// epochResult holds aggregated results for one epoch.
type epochResult struct {
	Epoch       string
	Variables   int
	Constraints int
	Problems    int
	Solved      int
	AvgTime     time.Duration
	MaxTime     time.Duration
}

func TestMainnetBenchmark(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping benchmark in short mode")
	}
	t.Log("=== Part 1: Mainnet default parameters (10 problems) ===")
	t.Log("")
	testMainnetDefault(t)

	// Force GC to reclaim resources from timed-out goroutines' allocations.
	runtime.GC()

	t.Log("")
	t.Log("=== Part 2: Future difficulty growth (5 epochs × 3 problems) ===")
	t.Log("")
	testFutureEpochs(t)
}

// testMainnetDefault generates 10 problems with mainnet Standard level and solves them.
//
// Per-problem timeout is 10 seconds. Brute-force is expected to fail on all
// problems with 8+ variables (domain size ~80), confirming AI solvers are required.
func testMainnetDefault(t *testing.T) {
	t.Helper()

	bf := NewBruteForceSolver()
	const numProblems = 10
	const timeout = 10 * time.Second

	t.Logf("Solver: BruteForceSolver | Timeout per problem: %s", timeout)
	t.Log("")
	t.Logf("%-8s %-6s %-12s %-14s %-12s %-10s %s",
		"Problem", "Vars", "Constraints", "Search Space", "Time", "Result", "Solution")
	t.Logf("%-8s %-6s %-12s %-14s %-12s %-10s %s",
		"-------", "----", "-----------", "------------", "----", "------", "--------")

	var totalTime time.Duration
	solved := 0

	for i := 0; i < numProblems; i++ {
		seed := crypto.Sha256([]byte(fmt.Sprintf("mainnet-bench-seed-%d", i)))
		problem, knownSol := csp.GenerateProblem(seed, csp.Standard)

		numVars := len(problem.Variables)
		numConstraints := len(problem.Constraints)
		searchSpace := estimateSearchSpace(problem)

		start := time.Now()
		sol, err := bf.Solve(problem, timeout)
		elapsed := time.Since(start)
		totalTime += elapsed

		if err != nil {
			t.Logf("%-8d %-6d %-12d %-14s %-12s %-10s known=%v",
				i, numVars, numConstraints, searchSpace,
				elapsed.Round(time.Millisecond), "TIMEOUT", knownSol.Values)
			continue
		}

		if csp.VerifySolution(problem, sol) {
			solved++
			t.Logf("%-8d %-6d %-12d %-14s %-12s %-10s %v",
				i, numVars, numConstraints, searchSpace,
				elapsed.Round(time.Millisecond), "OK", sol.Values)
		} else {
			t.Logf("%-8d %-6d %-12d %-14s %-12s %-10s got=%v known=%v",
				i, numVars, numConstraints, searchSpace,
				elapsed.Round(time.Millisecond), "INVALID", sol.Values, knownSol.Values)
		}
	}

	avgTime := totalTime / time.Duration(numProblems)
	t.Log("")
	t.Logf("Summary: %d/%d solved, avg time %s, total time %s",
		solved, numProblems, avgTime.Round(time.Millisecond), totalTime.Round(time.Millisecond))

	if solved == 0 {
		t.Log("Result: Brute-force CANNOT solve mainnet Standard-level CSPs within timeout.")
		t.Log("        AI solvers (LLM, SAT/CSP engines) are required for mining.")
	}
}

// testFutureEpochs tests progressively harder CSP parameters across 5 epochs.
//
// Each epoch increases variables and constraint ratio, simulating future
// CSP difficulty upgrades. Per-problem timeout is 120 seconds.
func testFutureEpochs(t *testing.T) {
	t.Helper()

	bf := NewBruteForceSolver()
	const problemsPerEpoch = 3

	// Per-epoch timeouts: shorter for harder epochs since they'll all timeout anyway.
	epochTimeouts := []time.Duration{
		30 * time.Second, // epoch 0: might solve a few small ones
		15 * time.Second, // epoch 1
		10 * time.Second, // epoch 2
		5 * time.Second,  // epoch 3
		5 * time.Second,  // epoch 4
	}

	results := make([]epochResult, len(epochs))

	for ei, ep := range epochs {
		numVars := ep.BaseVariables
		numConstraints := int(math.Ceil(float64(numVars) * ep.ConstraintRatio))
		timeout := epochTimeouts[ei]

		res := epochResult{
			Epoch:       ep.Name,
			Variables:   numVars,
			Constraints: numConstraints,
			Problems:    problemsPerEpoch,
		}

		t.Logf("--- %s: %d vars, %d constraints, timeout %s ---",
			ep.Name, numVars, numConstraints, timeout)

		var totalElapsed time.Duration

		for pi := 0; pi < problemsPerEpoch; pi++ {
			seed := crypto.Sha256([]byte(fmt.Sprintf("epoch%d-problem%d", ei, pi)))
			problem, knownSol := csp.GenerateProblemWithParams(seed, numVars, numConstraints)

			searchSpace := estimateSearchSpace(problem)

			start := time.Now()
			sol, err := bf.Solve(problem, timeout)
			elapsed := time.Since(start)
			totalElapsed += elapsed

			if elapsed > res.MaxTime {
				res.MaxTime = elapsed
			}

			if err != nil {
				t.Logf("  problem %d: TIMEOUT after %s  space=%s  known=%v",
					pi, elapsed.Round(time.Millisecond), searchSpace, knownSol.Values)
				continue
			}

			if csp.VerifySolution(problem, sol) {
				res.Solved++
				t.Logf("  problem %d: OK in %s  space=%s  solution=%v",
					pi, elapsed.Round(time.Millisecond), searchSpace, sol.Values)
			} else {
				t.Logf("  problem %d: INVALID in %s  space=%s  known=%v",
					pi, elapsed.Round(time.Millisecond), searchSpace, knownSol.Values)
			}
		}

		if problemsPerEpoch > 0 {
			res.AvgTime = totalElapsed / time.Duration(problemsPerEpoch)
		}
		results[ei] = res

		// Allow timed-out goroutines a moment to be scheduled away.
		runtime.Gosched()
	}

	// Print summary table.
	t.Log("")
	t.Logf("%-20s | %-10s | %-12s | %-14s | %-12s | %-12s | %s",
		"epoch", "variables", "constraints", "search_space", "avg_time", "max_time", "success_rate")
	t.Logf("%-20s-+-%-10s-+-%-12s-+-%-14s-+-%-12s-+-%-12s-+-%s",
		"--------------------", "----------", "------------", "--------------",
		"------------", "------------", "------------")

	for _, r := range results {
		rate := fmt.Sprintf("%d/%d", r.Solved, r.Problems)
		space := fmt.Sprintf("~%d^%d", estimateAvgDomain(r.Variables), r.Variables)
		t.Logf("%-20s | %-10d | %-12d | %-14s | %-12s | %-12s | %s",
			r.Epoch, r.Variables, r.Constraints, space,
			r.AvgTime.Round(time.Millisecond),
			r.MaxTime.Round(time.Millisecond),
			rate)
	}

	t.Log("")
	t.Log("Conclusion: Brute-force complexity grows exponentially with variable count.")
	t.Log("            AI solvers must scale to handle epoch 2+ difficulty.")
}

// estimateSearchSpace returns a human-readable estimate of the brute-force search space.
func estimateSearchSpace(problem *csp.Problem) string {
	if len(problem.Variables) == 0 {
		return "0"
	}
	total := 1.0
	for _, v := range problem.Variables {
		domSize := float64(v.Upper - v.Lower + 1)
		total *= domSize
	}
	if total < 1e6 {
		return fmt.Sprintf("%.0f", total)
	}
	exp := math.Log10(total)
	return fmt.Sprintf("~10^%.1f", exp)
}

// estimateAvgDomain returns an approximate average domain size for a given variable count.
// Based on GenerateProblem: lo ∈ [0,50), hi = lo + 20 + [0,80) → domain ∈ [21, 100], avg ~60.
func estimateAvgDomain(numVars int) int {
	_ = numVars
	return 60
}

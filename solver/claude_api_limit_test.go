package solver

import (
	"fmt"
	"math"
	"os"
	"testing"
	"time"

	"github.com/nous-chain/nous/crypto"
	"github.com/nous-chain/nous/csp"
)

// ============================================================
// TestClaudeAPILimit: Claude CSP solving capability at increasing difficulty
//
// Tests Claude via the Anthropic API on progressively harder CSP problems
// to determine the practical upper bound of LLM-based CSP solving.
//
// Environment variables:
//   ANTHROPIC_AUTH_TOKEN or ANTHROPIC_API_KEY — API key
//   ANTHROPIC_BASE_URL — API base URL (default: https://api.anthropic.com)
//   CLAUDE_MODEL — model ID (default: claude-opus-4-6)
// ============================================================

type difficultyLevel struct {
	Label           string
	BaseVariables   int
	ConstraintRatio float64
}

var difficultyLevels = []difficultyLevel{
	{"year 1  (epoch 0)", 8, 1.2},
	{"year 3  (epoch 2)", 12, 1.4},
	{"year 5  (epoch 4)", 16, 1.6},
	{"year 7  (epoch 6)", 20, 1.8},
	{"year 9  (epoch 8)", 24, 2.0},
}

// levelResult holds aggregated results for one difficulty level.
type levelResult struct {
	Label       string
	Variables   int
	Constraints int
	Problems    int
	Solved      int
	TotalTime   time.Duration
	Times       []time.Duration
}

func TestClaudeAPILimit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping API test in short mode")
	}
	// Resolve API credentials from environment.
	apiKey := os.Getenv("ANTHROPIC_AUTH_TOKEN")
	if apiKey == "" {
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	if apiKey == "" {
		t.Skip("No API key: set ANTHROPIC_AUTH_TOKEN or ANTHROPIC_API_KEY")
	}

	baseURL := os.Getenv("ANTHROPIC_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	endpoint := baseURL + "/v1/messages"

	model := os.Getenv("CLAUDE_MODEL")
	if model == "" {
		model = "claude-opus-4-6"
	}

	const problemsPerLevel = 2

	// Per-level config: timeout and retries decrease for harder problems
	// to keep total test time within 600s.
	type levelConfig struct {
		timeout    time.Duration
		maxRetries int
	}
	levelConfigs := []levelConfig{
		{90 * time.Second, 2},  // 8 vars:  ~40s per call × 2 retries = ~80s max per problem
		{90 * time.Second, 2},  // 12 vars: ~40s per call × 2 retries
		{90 * time.Second, 2},  // 16 vars: ~40s per call × 2 retries
		{90 * time.Second, 1},  // 20 vars: ~50s per call × 1 retry (single shot)
		{90 * time.Second, 1},  // 24 vars: ~50s per call × 1 retry (single shot)
	}

	t.Logf("Endpoint: %s", endpoint)
	t.Logf("Model:    %s", model)
	t.Logf("Problems: %d per level, 5 levels", problemsPerLevel)
	t.Log("")

	results := make([]levelResult, len(difficultyLevels))
	testStart := time.Now()

	for li, lv := range difficultyLevels {
		numVars := lv.BaseVariables
		numConstraints := int(math.Ceil(float64(numVars) * lv.ConstraintRatio))
		cfg := levelConfigs[li]

		s := NewRemoteAPISolver(endpoint, apiKey, model, ProviderAnthropic)
		s.SetMaxRetries(cfg.maxRetries)

		res := levelResult{
			Label:       lv.Label,
			Variables:   numVars,
			Constraints: numConstraints,
			Problems:    problemsPerLevel,
			Times:       make([]time.Duration, 0, problemsPerLevel),
		}

		t.Logf("--- %s: %d vars, %d constraints (timeout=%s, retries=%d) ---",
			lv.Label, numVars, numConstraints, cfg.timeout, cfg.maxRetries)

		for pi := 0; pi < problemsPerLevel; pi++ {
			// Time guard: bail out if we've consumed most of the 600s budget.
			if time.Since(testStart) > 500*time.Second {
				t.Logf("  problem %d: SKIPPED (time budget exceeded)", pi)
				res.Times = append(res.Times, 0)
				continue
			}

			seed := crypto.Sha256([]byte(fmt.Sprintf("claude-limit-%dv-%.1fr-p%d", numVars, lv.ConstraintRatio, pi)))
			problem, knownSol := csp.GenerateProblemWithParams(seed, numVars, numConstraints)

			t.Logf("  problem %d: %d vars, %d constraints, known=%v",
				pi, len(problem.Variables), len(problem.Constraints), knownSol.Values)

			start := time.Now()
			sol, err := s.Solve(problem, cfg.timeout)
			elapsed := time.Since(start)

			res.TotalTime += elapsed
			res.Times = append(res.Times, elapsed)

			if err != nil {
				t.Logf("  problem %d: FAIL in %s — %v",
					pi, elapsed.Round(time.Millisecond), err)
				continue
			}

			if csp.VerifySolution(problem, sol) {
				res.Solved++
				t.Logf("  problem %d: PASS in %s — solution=%v",
					pi, elapsed.Round(time.Millisecond), sol.Values)
			} else {
				t.Logf("  problem %d: WRONG in %s — got=%v known=%v",
					pi, elapsed.Round(time.Millisecond), sol.Values, knownSol.Values)
			}
		}

		results[li] = res
		t.Log("")
	}

	// Print summary table.
	t.Log("=== Summary ===")
	t.Log("")
	t.Logf("%-22s | %-10s | %-12s | %-12s | %-14s | %s",
		"difficulty", "variables", "constraints", "avg_time", "success_rate", "times")
	t.Logf("%-22s-+-%-10s-+-%-12s-+-%-12s-+-%-14s-+-%s",
		"----------------------", "----------", "------------",
		"------------", "--------------", "-------------------")

	for _, r := range results {
		var avgTime time.Duration
		if r.Problems > 0 {
			avgTime = r.TotalTime / time.Duration(r.Problems)
		}
		rate := fmt.Sprintf("%d/%d", r.Solved, r.Problems)

		timesStr := ""
		for i, d := range r.Times {
			if i > 0 {
				timesStr += ", "
			}
			if d == 0 {
				timesStr += "skipped"
			} else {
				timesStr += d.Round(time.Millisecond).String()
			}
		}

		t.Logf("%-22s | %-10d | %-12d | %-12s | %-14s | %s",
			r.Label, r.Variables, r.Constraints,
			avgTime.Round(time.Millisecond), rate, timesStr)
	}

	// Overall stats.
	t.Log("")
	totalSolved := 0
	totalProblems := 0
	for _, r := range results {
		totalSolved += r.Solved
		totalProblems += r.Problems
	}
	t.Logf("Overall: %d/%d problems solved by %s", totalSolved, totalProblems, model)
	t.Logf("Total wall time: %s", time.Since(testStart).Round(time.Second))

	// Identify the difficulty ceiling.
	lastSolvedLevel := -1
	for i, r := range results {
		if r.Solved > 0 {
			lastSolvedLevel = i
		}
	}
	if lastSolvedLevel >= 0 {
		lv := difficultyLevels[lastSolvedLevel]
		t.Logf("Ceiling: %s can solve up to %d vars / %.1f ratio (%s)",
			model, lv.BaseVariables, lv.ConstraintRatio, lv.Label)
	} else {
		t.Logf("Ceiling: %s could not solve any difficulty level", model)
	}
}

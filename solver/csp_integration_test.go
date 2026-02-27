//go:build integration

package solver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/nous-chain/nous/crypto"
	"github.com/nous-chain/nous/csp"
)

const ollamaEndpoint = "http://localhost:11434"

// skipIfOllamaUnavailable sends a tiny generate request to Ollama.
// If no valid response arrives within 10 seconds, the test is skipped.
func skipIfOllamaUnavailable(t *testing.T, model string) {
	t.Helper()
	reqBody := ollamaRequest{Model: model, Prompt: "hi", Stream: false}
	body, _ := json.Marshal(reqBody)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(ollamaEndpoint+"/api/generate", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Skipf("Ollama unavailable: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		t.Skipf("Ollama model %s not ready: HTTP %d: %s", model, resp.StatusCode, string(respBody))
	}
	io.Copy(io.Discard, resp.Body)
}

// callOllamaRaw makes a direct Ollama API call and returns the raw response text.
func callOllamaRaw(model, prompt string, timeout time.Duration) (string, error) {
	reqBody := ollamaRequest{Model: model, Prompt: prompt, Stream: false}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	client := &http.Client{Timeout: timeout}
	resp, err := client.Post(ollamaEndpoint+"/api/generate", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result ollamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.Response, nil
}

// TestCSPDiagnosticClaude tests Claude API on progressively harder CSP problems.
// Requires CLAUDE_API_KEY env var. Optional: CLAUDE_API_ENDPOINT, CLAUDE_MODEL.
func TestCSPDiagnosticClaude(t *testing.T) {
	apiKey := os.Getenv("CLAUDE_API_KEY")
	if apiKey == "" {
		t.Skip("CLAUDE_API_KEY not set")
	}
	endpoint := os.Getenv("CLAUDE_API_ENDPOINT")
	if endpoint == "" {
		endpoint = "https://api.anthropic.com/v1/messages"
	}
	model := os.Getenv("CLAUDE_MODEL")
	if model == "" {
		model = "claude-sonnet-4-20250514"
	}

	levels := []struct{ vars, constraints int }{
		{3, 3}, {4, 4}, {5, 5}, {6, 6}, {8, 8}, {8, 10},
	}

	s := NewRemoteAPISolver(endpoint, apiKey, model, "anthropic")

	for _, lv := range levels {
		name := fmt.Sprintf("%dv%dc", lv.vars, lv.constraints)
		t.Run(name, func(t *testing.T) {
			seed := crypto.Sha256([]byte("claude-bench-" + name))
			problem, knownSol := csp.GenerateProblemWithParams(seed, lv.vars, lv.constraints)

			t.Logf("Problem: %d vars, %d constraints",
				len(problem.Variables), len(problem.Constraints))
			t.Logf("Known solution: %v", knownSol.Values)

			start := time.Now()
			sol, err := s.Solve(problem, 120*time.Second)
			elapsed := time.Since(start)

			if err != nil {
				t.Logf("FAIL %s in %v: %v", name, elapsed.Round(time.Millisecond), err)
				return
			}
			if csp.VerifySolution(problem, sol) {
				t.Logf("PASS %s in %v", name, elapsed.Round(time.Millisecond))
			} else {
				t.Logf("FAIL %s in %v (known: %v)",
					name, elapsed.Round(time.Millisecond), knownSol.Values)
			}
		})
	}
}

// TestCSPDiagnosticOllama tests deepseek-r1:7b on progressively harder CSPs.
func TestCSPDiagnosticOllama(t *testing.T) {
	const model = "deepseek-r1:7b"
	skipIfOllamaUnavailable(t, model)

	levels := []struct{ vars, constraints int }{
		{3, 3}, {4, 4}, {5, 5}, {6, 6}, {7, 7}, {8, 8},
	}

	for _, lv := range levels {
		name := fmt.Sprintf("%dv%dc", lv.vars, lv.constraints)
		t.Run(name, func(t *testing.T) {
			seed := crypto.Sha256([]byte("ollama-bench-" + name))
			problem, knownSol := csp.GenerateProblemWithParams(seed, lv.vars, lv.constraints)

			t.Logf("Problem: %d vars, %d constraints",
				len(problem.Variables), len(problem.Constraints))
			t.Logf("Known solution: %v", knownSol.Values)

			s := NewOllamaSolver(ollamaEndpoint, model)
			start := time.Now()
			sol, err := s.Solve(problem, 120*time.Second)
			elapsed := time.Since(start)

			if err != nil {
				t.Logf("FAIL %s in %v: %v",
					name, elapsed.Round(time.Millisecond), err)
				return
			}
			if csp.VerifySolution(problem, sol) {
				t.Logf("PASS %s in %v",
					name, elapsed.Round(time.Millisecond))
			} else {
				t.Logf("FAIL %s in %v (known: %v)",
					name, elapsed.Round(time.Millisecond), knownSol.Values)
			}
		})
	}
}

// TestCSPOllamaQwen tests qwen2.5:7b on CSP problems with raw response output.
func TestCSPOllamaQwen(t *testing.T) {
	const model = "qwen2.5:7b"
	skipIfOllamaUnavailable(t, model)

	levels := []struct{ vars, constraints int }{
		{3, 3}, {4, 4}, {5, 5},
	}

	for _, lv := range levels {
		name := fmt.Sprintf("%dv%dc", lv.vars, lv.constraints)
		t.Run(name, func(t *testing.T) {
			seed := crypto.Sha256([]byte("qwen-bench-" + name))
			problem, knownSol := csp.GenerateProblemWithParams(
				seed, lv.vars, lv.constraints)

			t.Logf("Known solution: %v", knownSol.Values)

			prompt := FormatCSPPrompt(problem)
			start := time.Now()
			raw, err := callOllamaRaw(model, prompt, 60*time.Second)
			elapsed := time.Since(start)

			if err != nil {
				t.Logf("FAIL %s in %v: %v",
					name, elapsed.Round(time.Millisecond), err)
				return
			}
			t.Logf("Raw response (%v):\n%s",
				elapsed.Round(time.Millisecond), raw)

			sol, err := ParseSolution(raw, problem)
			if err != nil {
				t.Logf("FAIL %s: parse error: %v", name, err)
				return
			}
			t.Logf("Parsed values: %v", sol.Values)

			if csp.VerifySolution(problem, sol) {
				t.Logf("PASS %s in %v",
					name, elapsed.Round(time.Millisecond))
			} else {
				t.Logf("FAIL %s (known: %v)",
					name, knownSol.Values)
			}
		})
	}
}

// TestCSPMultiAttempt tests a fast-retry strategy with qwen2.5:7b.
func TestCSPMultiAttempt(t *testing.T) {
	const model = "qwen2.5:7b"
	skipIfOllamaUnavailable(t, model)

	const maxAttempts = 3
	const perAttempt = 30 * time.Second

	levels := []struct{ vars, constraints int }{
		{3, 3}, {4, 4}, {5, 5},
	}

	var totalPassed, totalFailed int

	for _, lv := range levels {
		name := fmt.Sprintf("%dv%dc", lv.vars, lv.constraints)
		t.Run(name, func(t *testing.T) {
			seed := crypto.Sha256([]byte("multi-bench-" + name))
			problem, knownSol := csp.GenerateProblemWithParams(
				seed, lv.vars, lv.constraints)
			t.Logf("Known solution: %v", knownSol.Values)

			totalStart := time.Now()
			solved := false

			for attempt := 1; attempt <= maxAttempts; attempt++ {
				s := NewOllamaSolver(ollamaEndpoint, model)
				s.SetMaxRetries(1)

				start := time.Now()
				sol, err := s.Solve(problem, perAttempt)
				elapsed := time.Since(start)

				if err != nil {
					t.Logf("  Attempt %d: FAIL in %v: %v",
						attempt, elapsed.Round(time.Millisecond), err)
					continue
				}
				if csp.VerifySolution(problem, sol) {
					t.Logf("  Attempt %d: PASS in %v (total: %v)",
						attempt, elapsed.Round(time.Millisecond),
						time.Since(totalStart).Round(time.Millisecond))
					solved = true
					break
				}
				t.Logf("  Attempt %d: WRONG in %v: %v",
					attempt, elapsed.Round(time.Millisecond), sol.Values)
			}

			if solved {
				totalPassed++
			} else {
				totalFailed++
			}
		})
	}

	t.Logf("Passed: %d | Failed: %d", totalPassed, totalFailed)
}

// TestCSPAlternativePrompt tests qwen2.5:7b with different prompt styles.
func TestCSPAlternativePrompt(t *testing.T) {
	const model = "qwen2.5:7b"
	skipIfOllamaUnavailable(t, model)

	trivialProb := &csp.Problem{
		Variables: []csp.Variable{
			{Name: "x0", Lower: 1, Upper: 100},
			{Name: "x1", Lower: 1, Upper: 100},
		},
		Constraints: []csp.Constraint{
			{Type: csp.CtLinear, Vars: []int{0, 1}, Params: []int{1, 1, 50}},
		},
	}

	t.Run("trivial-english", func(t *testing.T) {
		prompt := "Find X0 in [1,100] and X1 in [1,100] where X0 + X1 = 50. Reply only: X0=number X1=number"
		raw, err := callOllamaRaw(model, prompt, 60*time.Second)
		if err != nil {
			t.Logf("FAIL: %v", err)
			return
		}
		t.Logf("Raw response:\n%s", raw)
		sol, err := ParseSolution(raw, trivialProb)
		if err != nil {
			t.Logf("FAIL parse: %v", err)
			return
		}
		if csp.VerifySolution(trivialProb, sol) {
			t.Logf("PASS: X0=%d, X1=%d", sol.Values[0], sol.Values[1])
		} else {
			t.Logf("FAIL: X0=%d, X1=%d", sol.Values[0], sol.Values[1])
		}
	})

	t.Run("chinese-prompt", func(t *testing.T) {
		prompt := "请解决以下约束满足问题。\n变量：X0 取值范围 [1, 100]，X1 取值范围 [1, 100]\n约束：X0 + X1 = 50\n只回答数值，格式：X0=数字 X1=数字"
		raw, err := callOllamaRaw(model, prompt, 60*time.Second)
		if err != nil {
			t.Logf("FAIL: %v", err)
			return
		}
		t.Logf("Raw response:\n%s", raw)
		sol, err := ParseSolution(raw, trivialProb)
		if err != nil {
			t.Logf("FAIL parse: %v", err)
			return
		}
		if csp.VerifySolution(trivialProb, sol) {
			t.Logf("PASS: X0=%d, X1=%d", sol.Values[0], sol.Values[1])
		} else {
			t.Logf("FAIL: X0=%d, X1=%d", sol.Values[0], sol.Values[1])
		}
	})

	t.Run("standard-prompt-trivial", func(t *testing.T) {
		prompt := FormatCSPPrompt(trivialProb)
		raw, err := callOllamaRaw(model, prompt, 60*time.Second)
		if err != nil {
			t.Logf("FAIL: %v", err)
			return
		}
		t.Logf("Raw response:\n%s", raw)
		sol, err := ParseSolution(raw, trivialProb)
		if err != nil {
			t.Logf("FAIL parse: %v", err)
			return
		}
		if csp.VerifySolution(trivialProb, sol) {
			t.Logf("PASS: X0=%d, X1=%d", sol.Values[0], sol.Values[1])
		} else {
			t.Logf("FAIL: X0=%d, X1=%d", sol.Values[0], sol.Values[1])
		}
	})
}

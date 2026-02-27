package solver

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nous-chain/nous/csp"
)

// smallProblem returns a tiny CSP suitable for brute-force and prompt tests.
// X0 in [1,3], X1 in [1,3], constraint: 1*X0 + 1*X1 = 4
func smallProblem() *csp.Problem {
	return &csp.Problem{
		Variables: []csp.Variable{
			{Name: "x0", Lower: 1, Upper: 3},
			{Name: "x1", Lower: 1, Upper: 3},
		},
		Constraints: []csp.Constraint{
			{Type: csp.CtLinear, Vars: []int{0, 1}, Params: []int{1, 1, 4}},
		},
		Level: csp.Standard,
	}
}

// failingSolver always returns an error.
type failingSolver struct{}

func (f *failingSolver) Solve(_ *csp.Problem, _ time.Duration) (*csp.Solution, error) {
	return nil, errors.New("always fails")
}

// ============================================================
// 1. TestFormatCSPPrompt
// ============================================================

func TestFormatCSPPrompt(t *testing.T) {
	prob := smallProblem()
	prompt := FormatCSPPrompt(prob)

	// Must contain variable descriptions.
	if !strings.Contains(prompt, "X0") || !strings.Contains(prompt, "X1") {
		t.Fatal("prompt should reference X0 and X1")
	}
	if !strings.Contains(prompt, "[1, 3]") {
		t.Fatal("prompt should contain variable range [1, 3]")
	}

	// Must contain the linear constraint.
	if !strings.Contains(prompt, "1 * X0 + 1 * X1 = 4") {
		t.Fatalf("prompt should contain constraint text, got:\n%s", prompt)
	}

	// Must contain response format instructions.
	if !strings.Contains(prompt, "X0=<value>") {
		t.Fatal("prompt should contain response format instructions")
	}

	// Test with all 14 constraint types to ensure no panics.
	bigProb := &csp.Problem{
		Variables: []csp.Variable{
			{Name: "x0", Lower: 1, Upper: 10},
			{Name: "x1", Lower: 1, Upper: 10},
			{Name: "x2", Lower: 1, Upper: 10},
		},
		Constraints: []csp.Constraint{
			{Type: csp.CtLinear, Vars: []int{0, 1}, Params: []int{1, 1, 5}},
			{Type: csp.CtMulMod, Vars: []int{0, 1}, Params: []int{7, 3}},
			{Type: csp.CtSumSquares, Vars: []int{0, 1}, Params: []int{10, 2}},
			{Type: csp.CtCompare, Vars: []int{0, 1}, Params: []int{1}},
			{Type: csp.CtModChain, Vars: []int{0, 1}, Params: []int{3, 4}},
			{Type: csp.CtConditional, Vars: []int{0, 1}, Params: []int{5, 8, 2}},
			{Type: csp.CtTrilinear, Vars: []int{0, 1, 2}, Params: []int{15}},
			{Type: csp.CtDivisible, Vars: []int{0, 1}, Params: nil},
			{Type: csp.CtPrimeNth, Vars: []int{0, 1}, Params: []int{5, 10}},
			{Type: csp.CtGCD, Vars: []int{0, 1}, Params: []int{2}},
			{Type: csp.CtFibMod, Vars: []int{0, 1}, Params: []int{8, 7, 3}},
			{Type: csp.CtNestedCond, Vars: []int{0, 1, 2}, Params: []int{3, 8, 2}},
			{Type: csp.CtXOR, Vars: []int{0, 1}, Params: []int{6}},
			{Type: csp.CtDigitRoot, Vars: []int{0, 1}, Params: []int{9}},
		},
		Level: csp.Standard,
	}
	bigPrompt := FormatCSPPrompt(bigProb)
	if !strings.Contains(bigPrompt, "Constraints:") {
		t.Fatal("big prompt should contain Constraints section")
	}
	// All 14 constraints should appear as numbered lines.
	if !strings.Contains(bigPrompt, "14.") {
		t.Fatal("big prompt should have 14 numbered constraints")
	}
}

// ============================================================
// 2. TestParseSolution
// ============================================================

func TestParseSolution(t *testing.T) {
	prob := smallProblem()

	tests := []struct {
		name     string
		input    string
		wantX0   int
		wantX1   int
	}{
		{"equals no space", "X0=1\nX1=3", 1, 3},
		{"equals with spaces", "X0 = 2\nX1 = 2", 2, 2},
		{"colon separator", "X0: 3\nX1: 1", 3, 1},
		{"lowercase", "x0=1\nx1=3", 1, 3},
		{"mixed with noise", "Here is the solution:\nX0=2\nX1=2\nDone!", 2, 2},
		{"bullet prefix", "- X0=1\n- X1=3", 1, 3},
		{"extra whitespace", "  X0 = 3  \n  X1 = 1  ", 3, 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sol, err := ParseSolution(tc.input, prob)
			if err != nil {
				t.Fatalf("ParseSolution(%q): %v", tc.input, err)
			}
			if sol.Values[0] != tc.wantX0 || sol.Values[1] != tc.wantX1 {
				t.Fatalf("got [%d, %d], want [%d, %d]",
					sol.Values[0], sol.Values[1], tc.wantX0, tc.wantX1)
			}
		})
	}
}

// ============================================================
// 3. TestParseSolutionInvalid
// ============================================================

func TestParseSolutionInvalid(t *testing.T) {
	prob := smallProblem()

	invalid := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"garbage", "hello world\nno numbers here"},
		{"missing variable", "X0=1"},
		{"no separator", "X0 1\nX1 3"},
		{"wrong variable names", "A=1\nB=3"},
	}

	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseSolution(tc.input, prob)
			if err == nil {
				t.Fatalf("expected error for input %q", tc.input)
			}
		})
	}
}

// ============================================================
// 4. TestBruteForceSolver
// ============================================================

func TestBruteForceSolver(t *testing.T) {
	prob := smallProblem()
	bf := NewBruteForceSolver()

	sol, err := bf.Solve(prob, 10*time.Second)
	if err != nil {
		t.Fatalf("BruteForceSolver.Solve: %v", err)
	}

	// Solution must satisfy the constraint: X0 + X1 = 4.
	if sol.Values[0]+sol.Values[1] != 4 {
		t.Fatalf("expected sum 4, got %d+%d=%d",
			sol.Values[0], sol.Values[1], sol.Values[0]+sol.Values[1])
	}

	// Must pass csp.VerifySolution.
	if !csp.VerifySolution(prob, sol) {
		t.Fatal("brute-force solution should verify")
	}
}

// ============================================================
// 5. TestSolverManager
// ============================================================

func TestSolverManager(t *testing.T) {
	prob := smallProblem()

	// Primary fails, fallback (brute-force) succeeds.
	mgr := NewSolverManager()
	mgr.SetPrimary(&failingSolver{})
	mgr.SetFallback(NewBruteForceSolver())

	sol, err := mgr.Solve(prob, 10*time.Second)
	if err != nil {
		t.Fatalf("SolverManager.Solve: %v", err)
	}
	if !csp.VerifySolution(prob, sol) {
		t.Fatal("manager solution should verify")
	}

	// Both fail → error.
	mgr2 := NewSolverManager()
	mgr2.SetPrimary(&failingSolver{})
	mgr2.SetFallback(&failingSolver{})
	_, err = mgr2.Solve(prob, 10*time.Second)
	if err == nil {
		t.Fatal("expected error when both solvers fail")
	}

	// No solvers configured → error.
	mgr3 := NewSolverManager()
	_, err = mgr3.Solve(prob, 10*time.Second)
	if err == nil {
		t.Fatal("expected error with no solvers configured")
	}
}

// ============================================================
// 6. TestOllamaSolverMock
// ============================================================

func TestOllamaSolverMock(t *testing.T) {
	prob := smallProblem()

	// Mock Ollama server that returns a valid solution.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request format.
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/generate" {
			t.Errorf("expected /api/generate, got %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected application/json content type")
		}

		// Decode and verify request body.
		var req ollamaRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if req.Model != "llama3" {
			t.Errorf("expected model llama3, got %s", req.Model)
		}
		if req.Stream != false {
			t.Errorf("expected stream=false")
		}
		if !strings.Contains(req.Prompt, "X0") {
			t.Errorf("prompt should contain variable X0")
		}

		// Return a valid solution.
		resp := ollamaResponse{
			Response: "X0=1\nX1=3",
			Done:     true,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	s := NewOllamaSolver(server.URL, "llama3")
	sol, err := s.Solve(prob, 10*time.Second)
	if err != nil {
		t.Fatalf("OllamaSolver.Solve: %v", err)
	}
	if !csp.VerifySolution(prob, sol) {
		t.Fatal("ollama mock solution should verify")
	}
}

// ============================================================
// 7. TestRemoteAPISolverMock
// ============================================================

func TestRemoteAPISolverMock(t *testing.T) {
	prob := smallProblem()

	// Mock OpenAI-compatible server.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}

		// Verify Authorization header.
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-key-123" {
			t.Errorf("expected Bearer test-key-123, got %s", auth)
		}

		// Decode request body.
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if req["model"] != "gpt-4" {
			t.Errorf("expected model gpt-4, got %v", req["model"])
		}
		msgs, ok := req["messages"].([]interface{})
		if !ok || len(msgs) == 0 {
			t.Errorf("expected messages array")
		}

		// Return OpenAI-format response with valid solution.
		resp := fmt.Sprintf(`{
			"choices": [{
				"message": {"content": "X0=2\nX1=2"}
			}]
		}`)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(resp))
	}))
	defer server.Close()

	s := NewRemoteAPISolver(server.URL, "test-key-123", "gpt-4", ProviderOpenAI)
	sol, err := s.Solve(prob, 10*time.Second)
	if err != nil {
		t.Fatalf("RemoteAPISolver.Solve (OpenAI): %v", err)
	}
	if !csp.VerifySolution(prob, sol) {
		t.Fatal("remote OpenAI mock solution should verify")
	}

	// Mock Anthropic server.
	anthropicServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify Anthropic-specific headers.
		if r.Header.Get("x-api-key") != "ant-key-456" {
			t.Errorf("expected x-api-key ant-key-456, got %s", r.Header.Get("x-api-key"))
		}
		if r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Errorf("expected anthropic-version 2023-06-01")
		}

		resp := `{"content": [{"text": "X0=3\nX1=1"}]}`
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(resp))
	}))
	defer anthropicServer.Close()

	s2 := NewRemoteAPISolver(anthropicServer.URL, "ant-key-456", "claude-3", ProviderAnthropic)
	sol2, err := s2.Solve(prob, 10*time.Second)
	if err != nil {
		t.Fatalf("RemoteAPISolver.Solve (Anthropic): %v", err)
	}
	if !csp.VerifySolution(prob, sol2) {
		t.Fatal("remote Anthropic mock solution should verify")
	}
}

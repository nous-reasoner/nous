package solver

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nous-chain/nous/csp"
)

// OllamaSolver solves CSPs by calling a local Ollama LLM instance.
type OllamaSolver struct {
	endpoint   string
	model      string
	maxRetries int
	client     *http.Client
}

// NewOllamaSolver creates a new Ollama-based CSP solver.
func NewOllamaSolver(endpoint, model string) *OllamaSolver {
	if endpoint == "" {
		endpoint = "http://localhost:11434"
	}
	return &OllamaSolver{
		endpoint:   endpoint,
		model:      model,
		maxRetries: 3,
		client:     &http.Client{},
	}
}

// SetMaxRetries overrides the default retry count.
func (s *OllamaSolver) SetMaxRetries(n int) {
	if n > 0 {
		s.maxRetries = n
	}
}

// Solve generates a prompt from the CSP, calls Ollama, parses and verifies the result.
// On verification failure it retries with error feedback up to maxRetries times.
func (s *OllamaSolver) Solve(problem *csp.Problem, timeout time.Duration) (*csp.Solution, error) {
	if problem == nil {
		return nil, errors.New("solver: nil problem")
	}

	deadline := time.Now().Add(timeout)
	prompt := FormatCSPPrompt(problem)
	var lastErr string

	for attempt := 0; attempt < s.maxRetries; attempt++ {
		if time.Now().After(deadline) {
			return nil, errors.New("solver: ollama timeout")
		}

		// Build prompt with optional error feedback from previous attempt.
		fullPrompt := prompt
		if lastErr != "" {
			fullPrompt += fmt.Sprintf("\n\nYour previous answer was incorrect: %s\nPlease try again with corrected values.", lastErr)
		}

		remaining := time.Until(deadline)
		resp, err := s.callOllama(fullPrompt, remaining)
		if err != nil {
			lastErr = err.Error()
			continue
		}

		sol, err := ParseSolution(resp, problem)
		if err != nil {
			lastErr = err.Error()
			continue
		}

		if csp.VerifySolution(problem, sol) {
			return sol, nil
		}
		lastErr = "solution does not satisfy all constraints"
	}

	return nil, fmt.Errorf("solver: ollama failed after %d attempts: %s", s.maxRetries, lastErr)
}

// ollamaRequest is the JSON body for POST /api/generate.
type ollamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

// ollamaResponse is the JSON response from Ollama /api/generate.
type ollamaResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

func (s *OllamaSolver) callOllama(prompt string, timeout time.Duration) (string, error) {
	reqBody := ollamaRequest{
		Model:  s.model,
		Prompt: prompt,
		Stream: false,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	ctx := s.client
	ctx.Timeout = timeout

	resp, err := ctx.Post(s.endpoint+"/api/generate", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("ollama request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("ollama HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result ollamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode ollama response: %w", err)
	}
	return result.Response, nil
}

// FormatCSPPrompt converts a CSP problem into a natural language prompt for an LLM.
func FormatCSPPrompt(problem *csp.Problem) string {
	var b strings.Builder
	b.WriteString("Solve this constraint satisfaction problem.\n\nVariables:\n")

	for i, v := range problem.Variables {
		fmt.Fprintf(&b, "  X%d: integer in range [%d, %d]\n", i, v.Lower, v.Upper)
	}

	b.WriteString("\nConstraints:\n")
	for i, c := range problem.Constraints {
		fmt.Fprintf(&b, "  %d. %s\n", i+1, formatConstraint(c))
	}

	b.WriteString("\nThink step by step but be concise. For each constraint, find values that satisfy it, then check consistency.\n")
	b.WriteString("Respond with ONLY the values, nothing else.\n\n")
	b.WriteString("Use this exact format:\n")
	for i := range problem.Variables {
		fmt.Fprintf(&b, "X%d=<value>\n", i)
	}

	return b.String()
}

func formatConstraint(c csp.Constraint) string {
	v := c.Vars
	p := c.Params

	switch c.Type {
	case csp.CtLinear:
		return fmt.Sprintf("%d * X%d + %d * X%d = %d", p[0], v[0], p[1], v[1], p[2])

	case csp.CtMulMod:
		return fmt.Sprintf("X%d * X%d mod %d = %d", v[0], v[1], p[0], p[1])

	case csp.CtSumSquares:
		return fmt.Sprintf("|X%d^2 + X%d^2 - %d| <= %d", v[0], v[1], p[0], p[1])

	case csp.CtCompare:
		return fmt.Sprintf("X%d > X%d + %d", v[0], v[1], p[0])

	case csp.CtModChain:
		return fmt.Sprintf("X%d mod %d = X%d mod %d", v[0], p[0], v[1], p[1])

	case csp.CtConditional:
		return fmt.Sprintf("if X%d > %d then X%d < %d else X%d > %d",
			v[0], p[0], v[1], p[1], v[1], p[2])

	case csp.CtTrilinear:
		return fmt.Sprintf("X%d * X%d + X%d = %d", v[0], v[1], v[2], p[0])

	case csp.CtDivisible:
		return fmt.Sprintf("X%d mod X%d = 0", v[0], v[1])

	case csp.CtPrimeNth:
		return fmt.Sprintf("NthPrime(X%d mod %d) + X%d = %d", v[0], p[0], v[1], p[1])

	case csp.CtGCD:
		return fmt.Sprintf("gcd(X%d, X%d) = %d", v[0], v[1], p[0])

	case csp.CtFibMod:
		return fmt.Sprintf("(Fib(X%d mod %d) + X%d) mod %d = %d",
			v[0], p[0], v[1], p[1], p[2])

	case csp.CtNestedCond:
		return fmt.Sprintf("if X%d > %d AND X%d < %d then X%d > %d",
			v[0], p[0], v[1], p[1], v[2], p[2])

	case csp.CtXOR:
		return fmt.Sprintf("X%d XOR X%d = %d", v[0], v[1], p[0])

	case csp.CtDigitRoot:
		return fmt.Sprintf("digitRoot(X%d * X%d) = %d", v[0], v[1], p[0])

	default:
		return fmt.Sprintf("unknown constraint type %d", c.Type)
	}
}

// ParseSolution extracts variable assignments from LLM response text.
// Supports formats: "X0=5", "X0: 5", "X0 = 5", "x0=5", and multiple on one line "X0=5 X1=3".
func ParseSolution(response string, problem *csp.Problem) (*csp.Solution, error) {
	n := len(problem.Variables)
	values := make([]int, n)
	found := make([]bool, n)

	lines := strings.Split(response, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Try the whole line first.
		idx, val, ok := parseAssignment(line)
		if ok && idx >= 0 && idx < n {
			values[idx] = val
			found[idx] = true
		}

		// Also try splitting by spaces/commas for multi-assignment lines like "X0=25 X1=25".
		for _, token := range strings.Fields(line) {
			token = strings.TrimRight(token, ",;")
			idx, val, ok := parseAssignment(token)
			if ok && idx >= 0 && idx < n {
				values[idx] = val
				found[idx] = true
			}
		}
	}

	// Check all variables were assigned.
	var missing []int
	for i, f := range found {
		if !f {
			missing = append(missing, i)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing variables: X%v", missing)
	}

	return &csp.Solution{Values: values}, nil
}

// parseAssignment parses a single "X<n>=<val>" or "X<n>: <val>" line.
func parseAssignment(line string) (idx int, val int, ok bool) {
	// Strip common prefixes like "- ", "* ", bullet numbers "1. "
	line = strings.TrimLeft(line, "-*• ")
	line = strings.TrimSpace(line)

	// Find the variable name (X or x followed by digits).
	if len(line) < 2 {
		return 0, 0, false
	}
	if line[0] != 'X' && line[0] != 'x' {
		return 0, 0, false
	}

	// Extract index digits.
	i := 1
	for i < len(line) && line[i] >= '0' && line[i] <= '9' {
		i++
	}
	if i == 1 {
		return 0, 0, false
	}
	idx, err := strconv.Atoi(line[1:i])
	if err != nil {
		return 0, 0, false
	}

	// Skip separator: "=", ":", " = ", " : "
	rest := strings.TrimSpace(line[i:])
	if len(rest) == 0 {
		return 0, 0, false
	}
	if rest[0] == '=' || rest[0] == ':' {
		rest = strings.TrimSpace(rest[1:])
	} else {
		return 0, 0, false
	}

	// Parse integer value (may have leading/trailing non-digits from LLM noise).
	rest = strings.TrimSpace(rest)
	// Extract leading integer (possibly negative).
	j := 0
	if j < len(rest) && rest[j] == '-' {
		j++
	}
	for j < len(rest) && rest[j] >= '0' && rest[j] <= '9' {
		j++
	}
	if j == 0 || (j == 1 && rest[0] == '-') {
		return 0, 0, false
	}

	val, err = strconv.Atoi(rest[:j])
	if err != nil {
		return 0, 0, false
	}
	return idx, val, true
}

package solver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/nous-reasoner/nous-miner-gui/backend/sat"
)

// AIGuidedSolver uses AI to suggest an initial assignment, then ProbSAT to refine.
// AI is called once to generate a "seed bias" — a probability distribution for variable
// assignments. ProbSAT then uses this bias as its starting point for every solve attempt,
// making it much faster than calling AI per seed.
type AIGuidedSolver struct {
	Provider    string
	APIKey      string
	Model       string
	BaseURL     string
	UseFallback bool
	Timeout     time.Duration
	cachedBias  sat.Assignment // reused across solves until formula changes
	biasReady   bool
}

func NewAIGuided() *AIGuidedSolver {
	return &AIGuidedSolver{
		Provider:    "anthropic",
		Model:       "claude-sonnet-4-6",
		UseFallback: true,
		Timeout:     200 * time.Millisecond,
	}
}

func (s *AIGuidedSolver) Name() string {
	return "AI-Guided ProbSAT"
}

func (s *AIGuidedSolver) Description() string {
	return "AI suggests initial assignment, ProbSAT refines (Experimental)"
}

func (s *AIGuidedSolver) Solve(formula sat.Formula, nVars int) (sat.Assignment, error) {
	// Get AI bias once per formula (cached).
	if !s.biasReady {
		initial, err := s.getAIGuess(formula, nVars)
		if err != nil {
			log.Printf("AI guess failed: %v, using random initial", err)
			// Fall back to pure ProbSAT with random start.
			return sat.Solve(formula, nVars, s.Timeout)
		}
		s.cachedBias = initial
		s.biasReady = true
		log.Printf("AI bias cached, ProbSAT will use it for all seeds")
	}

	// Always use ProbSAT with AI-suggested starting point.
	return sat.SolveWithInitial(formula, nVars, s.cachedBias, s.Timeout)
}

// ResetBias clears the cached bias so AI is called again on next formula.
func (s *AIGuidedSolver) ResetBias() {
	s.biasReady = false
	s.cachedBias = nil
}

func (s *AIGuidedSolver) getAIGuess(formula sat.Formula, nVars int) (sat.Assignment, error) {
	// Send a compact summary of the formula to the AI.
	dimacs := sat.ToDIMACS(formula, nVars)
	// Take first 20 clauses as sample.
	lines := splitLines(dimacs)
	sample := lines
	if len(sample) > 21 { // header + 20 clauses
		sample = sample[:21]
	}

	prompt := fmt.Sprintf(`Analyze this 3-SAT problem and suggest a variable assignment that satisfies as many clauses as possible.

%d variables, %d clauses. Sample:
%s

Return ONLY %d binary digits (0 or 1), no explanation.`,
		nVars, len(formula), joinLines(sample), nVars)

	text, err := s.callAI(prompt)
	if err != nil {
		return nil, err
	}

	return parseBinaryString(text, nVars), nil
}

func (s *AIGuidedSolver) callAI(prompt string) (string, error) {
	switch s.Provider {
	case "openai":
		return s.callOpenAI(prompt)
	case "anthropic":
		return s.callAnthropic(prompt)
	default:
		return "", fmt.Errorf("unsupported provider: %s", s.Provider)
	}
}

func (s *AIGuidedSolver) callOpenAI(prompt string) (string, error) {
	reqBody, _ := json.Marshal(map[string]interface{}{
		"model":       s.Model,
		"messages":    []map[string]string{{"role": "user", "content": prompt}},
		"temperature": 0.9,
		"max_tokens":  512,
	})

	baseURL := s.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	req, _ := http.NewRequest("POST", baseURL+"/v1/chat/completions", bytes.NewBuffer(reqBody))
	req.Header.Set("Authorization", "Bearer "+s.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("OpenAI error (%d): %s", resp.StatusCode, body)
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no response")
	}
	return result.Choices[0].Message.Content, nil
}

func (s *AIGuidedSolver) callAnthropic(prompt string) (string, error) {
	reqBody, _ := json.Marshal(map[string]interface{}{
		"model":      s.Model,
		"messages":   []map[string]string{{"role": "user", "content": prompt}},
		"max_tokens": 512,
	})

	baseURL := s.BaseURL
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	req, _ := http.NewRequest("POST", baseURL+"/v1/messages", bytes.NewBuffer(reqBody))
	req.Header.Set("x-api-key", s.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("Anthropic error (%d): %s", resp.StatusCode, body)
	}

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}
	if len(result.Content) == 0 {
		return "", fmt.Errorf("no response")
	}
	return result.Content[0].Text, nil
}

func parseBinaryString(s string, n int) sat.Assignment {
	a := make(sat.Assignment, n)
	i := 0
	for _, ch := range s {
		if i >= n {
			break
		}
		if ch == '1' {
			a[i] = true
			i++
		} else if ch == '0' {
			a[i] = false
			i++
		}
	}
	return a
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func joinLines(lines []string) string {
	result := ""
	for i, l := range lines {
		if i > 0 {
			result += "\n"
		}
		result += l
	}
	return result
}

func init() {
	Register("ai-guided", NewAIGuided())
}

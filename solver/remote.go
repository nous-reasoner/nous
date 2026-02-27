package solver

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/nous-chain/nous/csp"
)

// Supported API providers.
const (
	ProviderOpenAI    = "openai"
	ProviderAnthropic = "anthropic"
	ProviderDeepSeek  = "deepseek"
)

// RemoteAPISolver solves CSPs by calling a remote LLM API.
type RemoteAPISolver struct {
	endpoint   string
	apiKey     string
	model      string
	provider   string
	maxRetries int
	client     *http.Client
}

// NewRemoteAPISolver creates a new remote API solver.
func NewRemoteAPISolver(endpoint, apiKey, model, provider string) *RemoteAPISolver {
	return &RemoteAPISolver{
		endpoint:   endpoint,
		apiKey:     apiKey,
		model:      model,
		provider:   provider,
		maxRetries: 3,
		client:     &http.Client{},
	}
}

// SetMaxRetries overrides the default retry count.
func (s *RemoteAPISolver) SetMaxRetries(n int) {
	if n > 0 {
		s.maxRetries = n
	}
}

// Solve generates a prompt, calls the remote API, parses and verifies the result.
func (s *RemoteAPISolver) Solve(problem *csp.Problem, timeout time.Duration) (*csp.Solution, error) {
	if problem == nil {
		return nil, errors.New("solver: nil problem")
	}

	deadline := time.Now().Add(timeout)
	prompt := FormatCSPPrompt(problem)
	var lastErr string

	for attempt := 0; attempt < s.maxRetries; attempt++ {
		if time.Now().After(deadline) {
			return nil, errors.New("solver: remote timeout")
		}

		fullPrompt := prompt
		if lastErr != "" {
			fullPrompt += fmt.Sprintf("\n\nYour previous answer was incorrect: %s\nPlease try again with corrected values.", lastErr)
		}

		remaining := time.Until(deadline)
		var resp string
		var err error

		switch s.provider {
		case ProviderAnthropic:
			resp, err = s.callAnthropic(fullPrompt, remaining)
		default:
			// OpenAI-compatible format (works for openai, deepseek, and most providers).
			resp, err = s.callOpenAICompat(fullPrompt, remaining)
		}

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

	return nil, fmt.Errorf("solver: remote failed after %d attempts: %s", s.maxRetries, lastErr)
}

// --- OpenAI-compatible API ---

func (s *RemoteAPISolver) callOpenAICompat(prompt string, timeout time.Duration) (string, error) {
	reqBody := map[string]interface{}{
		"model": s.model,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"temperature": 0.2,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequest("POST", s.endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.apiKey)

	s.client.Timeout = timeout
	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", errors.New("empty choices in response")
	}
	return result.Choices[0].Message.Content, nil
}

// --- Anthropic API ---

func (s *RemoteAPISolver) callAnthropic(prompt string, timeout time.Duration) (string, error) {
	reqBody := map[string]interface{}{
		"model":      s.model,
		"max_tokens": 16384,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequest("POST", s.endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", s.apiKey)
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	s.client.Timeout = timeout
	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode: %w", err)
	}
	if len(result.Content) == 0 {
		return "", errors.New("empty content in response")
	}
	return result.Content[0].Text, nil
}

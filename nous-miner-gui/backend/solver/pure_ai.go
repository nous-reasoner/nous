package solver

import (
	"fmt"
	"log"

	"github.com/nous-reasoner/nous-miner-gui/backend/sat"
)

// PureAISolver lets AI attempt to solve directly (very low success rate).
type PureAISolver struct {
	Provider   string
	APIKey     string
	Model      string
	BaseURL    string
	MaxRetries int
}

func NewPureAI() *PureAISolver {
	return &PureAISolver{
		Provider:   "anthropic",
		Model:      "claude-sonnet-4-6",
		MaxRetries: 3,
	}
}

func (s *PureAISolver) Name() string {
	return "Pure AI"
}

func (s *PureAISolver) Description() string {
	return "AI attempts to solve directly (Very slow, educational only)"
}

func (s *PureAISolver) Solve(formula sat.Formula, nVars int) (sat.Assignment, error) {
	guided := &AIGuidedSolver{
		Provider: s.Provider,
		APIKey:   s.APIKey,
		Model:    s.Model,
		BaseURL:  s.BaseURL,
	}

	for i := 0; i < s.MaxRetries; i++ {
		guess, err := guided.getAIGuess(formula, nVars)
		if err != nil {
			log.Printf("Pure AI attempt %d failed: %v", i+1, err)
			continue
		}
		if sat.Verify(formula, guess) {
			return guess, nil
		}
		log.Printf("Pure AI attempt %d: solution invalid", i+1)
	}
	return nil, fmt.Errorf("AI failed after %d attempts", s.MaxRetries)
}

func init() {
	Register("pure-ai", NewPureAI())
}

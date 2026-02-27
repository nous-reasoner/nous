package solver

import (
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/nous-chain/nous/csp"
)

// SolverManager manages a primary solver with an optional fallback.
// If the primary solver fails or times out, it automatically tries the fallback.
type SolverManager struct {
	primary  CSPSolver
	fallback CSPSolver
}

// NewSolverManager creates a manager with no solvers configured.
func NewSolverManager() *SolverManager {
	return &SolverManager{}
}

// SetPrimary sets the primary solver.
func (m *SolverManager) SetPrimary(s CSPSolver) {
	m.primary = s
}

// SetFallback sets the fallback solver.
func (m *SolverManager) SetFallback(s CSPSolver) {
	m.fallback = s
}

// Solve tries the primary solver first, then falls back if it fails.
func (m *SolverManager) Solve(problem *csp.Problem, timeout time.Duration) (*csp.Solution, error) {
	if m.primary == nil && m.fallback == nil {
		return nil, errors.New("solver: no solvers configured")
	}

	// Try primary solver.
	if m.primary != nil {
		sol, err := m.primary.Solve(problem, timeout)
		if err == nil {
			return sol, nil
		}
		log.Printf("solver: primary failed: %v", err)
	}

	// Try fallback solver.
	if m.fallback != nil {
		sol, err := m.fallback.Solve(problem, timeout)
		if err == nil {
			return sol, nil
		}
		return nil, fmt.Errorf("solver: fallback also failed: %w", err)
	}

	return nil, errors.New("solver: primary failed and no fallback configured")
}

package solver

import (
	"time"

	"github.com/nous-reasoner/nous-miner-gui/backend/sat"
)

type ProbSATSolver struct {
	Timeout time.Duration
}

func NewProbSAT(timeout time.Duration) *ProbSATSolver {
	return &ProbSATSolver{Timeout: timeout}
}

func (s *ProbSATSolver) Name() string {
	return "ProbSAT"
}

func (s *ProbSATSolver) Description() string {
	return "Fast stochastic local search (CPU-only)"
}

func (s *ProbSATSolver) Solve(formula sat.Formula, nVars int) (sat.Assignment, error) {
	return sat.Solve(formula, nVars, s.Timeout)
}

func init() {
	Register("probsat", NewProbSAT(100*time.Millisecond))
}

package solver

import (
	"errors"
	"time"

	"github.com/nous-chain/nous/csp"
)

// BruteForceSolver solves CSPs by exhaustive search.
// Only viable for small problems. Used as a fallback when no AI solver is available.
type BruteForceSolver struct{}

// NewBruteForceSolver creates a new brute-force solver.
func NewBruteForceSolver() *BruteForceSolver {
	return &BruteForceSolver{}
}

// Solve attempts to solve the CSP by exhaustive enumeration within the timeout.
func (s *BruteForceSolver) Solve(problem *csp.Problem, timeout time.Duration) (*csp.Solution, error) {
	if problem == nil || len(problem.Variables) == 0 {
		return nil, errors.New("solver: empty problem")
	}

	done := make(chan *csp.Solution, 1)
	go func() {
		sol := &csp.Solution{Values: make([]int, len(problem.Variables))}
		if bruteForceHelper(problem, sol, 0) {
			done <- sol
		} else {
			done <- nil
		}
	}()

	select {
	case sol := <-done:
		if sol == nil {
			return nil, errors.New("solver: brute-force exhausted without solution")
		}
		return sol, nil
	case <-time.After(timeout):
		return nil, errors.New("solver: brute-force timeout")
	}
}

func bruteForceHelper(problem *csp.Problem, sol *csp.Solution, idx int) bool {
	if idx == len(problem.Variables) {
		return csp.VerifySolution(problem, sol)
	}
	v := problem.Variables[idx]
	for val := v.Lower; val <= v.Upper; val++ {
		sol.Values[idx] = val
		if bruteForceHelper(problem, sol, idx+1) {
			return true
		}
	}
	return false
}

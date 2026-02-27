// Package solver provides AI-based and fallback solvers for CSP problems
// in the NOUS consensus mechanism.
package solver

import (
	"time"

	"github.com/nous-chain/nous/csp"
)

// CSPSolver is the interface that all CSP solvers must implement.
type CSPSolver interface {
	Solve(problem *csp.Problem, timeout time.Duration) (*csp.Solution, error)
}

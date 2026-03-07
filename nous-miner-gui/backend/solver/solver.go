package solver

import (
	"fmt"

	"github.com/nous-reasoner/nous-miner-gui/backend/sat"
)

// Solver is the interface for all SAT solving strategies.
type Solver interface {
	Name() string
	Description() string
	Solve(formula sat.Formula, nVars int) (sat.Assignment, error)
}

var registry = map[string]Solver{}

func Register(name string, s Solver) {
	registry[name] = s
}

func Get(name string) (Solver, error) {
	s, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("solver not found: %s (available: probsat, ai-guided, pure-ai, custom)", name)
	}
	return s, nil
}

func List() map[string]Solver {
	return registry
}

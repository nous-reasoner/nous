package solver

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/nous-reasoner/nous-miner-gui/backend/sat"
)

// CustomSolver invokes a user-provided script that reads DIMACS from stdin
// and outputs a 256-character binary string to stdout.
type CustomSolver struct {
	ScriptPath string
	Timeout    time.Duration
}

func NewCustom() *CustomSolver {
	return &CustomSolver{Timeout: 30 * time.Second}
}

func (s *CustomSolver) Name() string {
	return "Custom Solver"
}

func (s *CustomSolver) Description() string {
	return "User-provided solver script (reads DIMACS stdin, outputs binary string)"
}

func (s *CustomSolver) Solve(formula sat.Formula, nVars int) (sat.Assignment, error) {
	if s.ScriptPath == "" {
		return nil, fmt.Errorf("no script path configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, s.ScriptPath)
	dimacs := sat.ToDIMACS(formula, nVars)
	cmd.Stdin = nopReader(dimacs)

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("script error: %w", err)
	}

	a := parseBinaryString(string(output), nVars)
	if !sat.Verify(formula, a) {
		return nil, fmt.Errorf("script returned invalid solution")
	}
	return a, nil
}

type stringReader struct {
	s string
	i int
}

func nopReader(s string) *stringReader {
	return &stringReader{s: s}
}

func (r *stringReader) Read(p []byte) (int, error) {
	if r.i >= len(r.s) {
		return 0, fmt.Errorf("EOF")
	}
	n := copy(p, r.s[r.i:])
	r.i += n
	return n, nil
}

func init() {
	Register("custom", NewCustom())
}

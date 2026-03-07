package sat

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
)

type Literal struct {
	Var int
	Neg bool
}

type Clause []Literal

type Formula []Clause

type Assignment []bool

func Verify(f Formula, a Assignment) bool {
	for _, c := range f {
		sat := false
		for _, lit := range c {
			if lit.Var >= len(a) {
				continue
			}
			val := a[lit.Var]
			if lit.Neg {
				val = !val
			}
			if val {
				sat = true
				break
			}
		}
		if !sat {
			return false
		}
	}
	return true
}

// ParseDIMACS parses a DIMACS CNF format string into a Formula.
// Variables are 1-indexed in DIMACS; converted to 0-indexed internally.
func ParseDIMACS(s string) (Formula, int, error) {
	var f Formula
	nVars := 0
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == 'c' {
			continue
		}
		if line[0] == 'p' {
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				n, _ := strconv.Atoi(parts[2])
				nVars = n
			}
			continue
		}
		// Clause line: "1 -5 200 0"
		fields := strings.Fields(line)
		var clause Clause
		for _, f := range fields {
			v, err := strconv.Atoi(f)
			if err != nil {
				continue
			}
			if v == 0 {
				break
			}
			neg := v < 0
			if neg {
				v = -v
			}
			clause = append(clause, Literal{Var: v - 1, Neg: neg}) // 1-indexed → 0-indexed
		}
		if len(clause) > 0 {
			f = append(f, clause)
		}
	}
	if len(f) == 0 {
		return nil, 0, fmt.Errorf("no clauses found")
	}
	return f, nVars, nil
}

// SerializeAssignment packs assignment into a compact bit array.
// Matches the node's sat.SerializeAssignment format exactly.
func SerializeAssignment(a Assignment) []byte {
	n := len(a)
	nbytes := (n + 7) / 8
	out := make([]byte, 4+nbytes)
	binary.BigEndian.PutUint32(out[:4], uint32(n))
	for i, v := range a {
		if v {
			out[4+i/8] |= 1 << (uint(i) % 8)
		}
	}
	return out
}

// ToDIMACS formats a formula in DIMACS CNF format.
func ToDIMACS(f Formula, nVars int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "p cnf %d %d\n", nVars, len(f))
	for _, clause := range f {
		for _, lit := range clause {
			v := lit.Var + 1 // 0-indexed to 1-indexed
			if lit.Neg {
				v = -v
			}
			fmt.Fprintf(&b, "%d ", v)
		}
		b.WriteString("0\n")
	}
	return b.String()
}

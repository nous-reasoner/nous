// Package csp handles deterministic generation and verification
// of Constraint Satisfaction Problems for the NOUS consensus mechanism.
//
// Mining flow:
//  1. VDF output → seed
//  2. seed + level → deterministic CSP (GenerateProblem)
//  3. Miner solves CSP using AI or any solver
//  4. Verifier checks solution via VerifySolution (no AI needed, microseconds)
package csp

import "fmt"

// Level identifies the CSP difficulty tier.
type Level int

const (
	Standard Level = iota // base_variables=12, constraint_ratio=1.4
)

func (l Level) String() string {
	switch l {
	case Standard:
		return "Standard"
	default:
		return fmt.Sprintf("Level(%d)", int(l))
	}
}

// ConstraintType enumerates the 14 constraint types per whitepaper §3.2.1.
type ConstraintType int

const (
	CtLinear      ConstraintType = iota // Type 1:  a*X + b*Y = c
	CtMulMod                            // Type 2:  X*Y mod Z = W
	CtSumSquares                        // Type 3:  |X² + Y² - Z| ≤ k
	CtCompare                           // Type 4:  X > Y + k
	CtModChain                          // Type 5:  X mod A = Y mod B
	CtConditional                       // Type 6:  if X > k then Y < m else Y > n
	CtTrilinear                         // Type 7:  X*Y + Z = W
	CtDivisible                         // Type 8:  X mod Y = 0  (Y ≠ 0)
	CtPrimeNth                          // Type 9:  NthPrime(X mod N) + Y = k
	CtGCD                               // Type 10: GCD(X, Y) = k
	CtFibMod                            // Type 11: (Fibonacci(X mod N) + Y) mod M = k
	CtNestedCond                        // Type 12: if X > a AND Y < b then Z > c
	CtXOR                               // Type 13: X XOR Y = k
	CtDigitRoot                         // Type 14: DigitRoot(X * Y) = k

	NumConstraintTypes = 14
)

func (t ConstraintType) String() string {
	names := [NumConstraintTypes]string{
		"Linear", "MulMod", "SumSquares", "Compare",
		"ModChain", "Conditional", "Trilinear", "Divisible",
		"PrimeNth", "GCD", "FibMod", "NestedCond", "XOR", "DigitRoot",
	}
	if int(t) >= 0 && int(t) < NumConstraintTypes {
		return names[t]
	}
	return fmt.Sprintf("Unknown(%d)", int(t))
}

// Variable represents a CSP variable with a bounded integer domain [Lower, Upper].
type Variable struct {
	Name  string
	Lower int // inclusive
	Upper int // inclusive
}

// Constraint represents a single typed constraint.
//
// Params layout per type:
//
//	Linear:      [a, b, c]        → a*X + b*Y = c
//	MulMod:      [Z, W]           → X*Y mod Z = W
//	SumSquares:  [Z, k]           → |X² + Y² - Z| ≤ k
//	Compare:     [k]              → X > Y + k
//	ModChain:    [A, B]           → X mod A = Y mod B
//	Conditional: [k, m, n]        → if X > k then Y < m else Y > n
//	Trilinear:   [W]              → X*Y + Z = W
//	Divisible:   []               → X mod Y = 0
//	PrimeNth:    [N, k]           → NthPrime(X mod N) + Y = k
//	GCD:         [k]              → GCD(X, Y) = k
//	FibMod:      [N, M, k]        → (Fibonacci(X mod N) + Y) mod M = k
//	NestedCond:  [a, b, c]        → if X > a AND Y < b then Z > c
//	XOR:         [k]              → X XOR Y = k
//	DigitRoot:   [k]              → DigitRoot(X * Y) = k
type Constraint struct {
	Type   ConstraintType
	Vars   []int // indices into Problem.Variables
	Params []int // type-specific constants (layout documented above)
}

// Problem is a complete CSP instance.
type Problem struct {
	Variables   []Variable
	Constraints []Constraint
	Level       Level
}

// Solution is an assignment of integer values to variables.
// Values[i] is the value assigned to Problem.Variables[i].
type Solution struct {
	Values []int
}

// VerifySolution checks whether the solution satisfies every constraint.
// This is the consensus-critical verification path — pure arithmetic, no AI needed.
func VerifySolution(problem *Problem, solution *Solution) bool {
	if problem == nil || solution == nil {
		return false
	}
	if len(solution.Values) != len(problem.Variables) {
		return false
	}

	// 1. Domain bounds check
	for i, v := range solution.Values {
		if v < problem.Variables[i].Lower || v > problem.Variables[i].Upper {
			return false
		}
	}

	// 2. Check every constraint
	for i := range problem.Constraints {
		if !checkConstraint(&problem.Constraints[i], solution.Values) {
			return false
		}
	}
	return true
}

// checkConstraint evaluates a single constraint against the given variable values.
func checkConstraint(c *Constraint, vals []int) bool {
	switch c.Type {

	case CtLinear: // a*X + b*Y = c
		if len(c.Vars) < 2 || len(c.Params) < 3 {
			return false
		}
		a, b, cv := c.Params[0], c.Params[1], c.Params[2]
		return a*vals[c.Vars[0]]+b*vals[c.Vars[1]] == cv

	case CtMulMod: // X*Y mod Z = W
		if len(c.Vars) < 2 || len(c.Params) < 2 {
			return false
		}
		z, w := c.Params[0], c.Params[1]
		if z <= 0 {
			return false
		}
		r := (vals[c.Vars[0]] * vals[c.Vars[1]]) % z
		if r < 0 {
			r += z
		}
		return r == w

	case CtSumSquares: // |X² + Y² - Z| ≤ k
		if len(c.Vars) < 2 || len(c.Params) < 2 {
			return false
		}
		zv, k := c.Params[0], c.Params[1]
		x, y := vals[c.Vars[0]], vals[c.Vars[1]]
		diff := x*x + y*y - zv
		if diff < 0 {
			diff = -diff
		}
		return diff <= k

	case CtCompare: // X > Y + k
		if len(c.Vars) < 2 || len(c.Params) < 1 {
			return false
		}
		return vals[c.Vars[0]] > vals[c.Vars[1]]+c.Params[0]

	case CtModChain: // X mod A = Y mod B
		if len(c.Vars) < 2 || len(c.Params) < 2 {
			return false
		}
		a, b := c.Params[0], c.Params[1]
		if a <= 0 || b <= 0 {
			return false
		}
		lhs := vals[c.Vars[0]] % a
		rhs := vals[c.Vars[1]] % b
		if lhs < 0 {
			lhs += a
		}
		if rhs < 0 {
			rhs += b
		}
		return lhs == rhs

	case CtConditional: // if X > k then Y < m else Y > n
		if len(c.Vars) < 2 || len(c.Params) < 3 {
			return false
		}
		k, m, n := c.Params[0], c.Params[1], c.Params[2]
		x, y := vals[c.Vars[0]], vals[c.Vars[1]]
		if x > k {
			return y < m
		}
		return y > n

	case CtTrilinear: // X*Y + Z = W
		if len(c.Vars) < 3 || len(c.Params) < 1 {
			return false
		}
		return vals[c.Vars[0]]*vals[c.Vars[1]]+vals[c.Vars[2]] == c.Params[0]

	case CtDivisible: // X mod Y = 0, Y ≠ 0
		if len(c.Vars) < 2 {
			return false
		}
		x, y := vals[c.Vars[0]], vals[c.Vars[1]]
		if y == 0 {
			return false
		}
		return x%y == 0

	case CtPrimeNth: // NthPrime(X mod N) + Y = k
		if len(c.Vars) < 2 || len(c.Params) < 2 {
			return false
		}
		N, k := c.Params[0], c.Params[1]
		if N <= 0 {
			return false
		}
		idx := vals[c.Vars[0]] % N
		if idx < 0 {
			idx += N
		}
		return NthPrime(idx)+vals[c.Vars[1]] == k

	case CtGCD: // GCD(X, Y) = k
		if len(c.Vars) < 2 || len(c.Params) < 1 {
			return false
		}
		return GCD(vals[c.Vars[0]], vals[c.Vars[1]]) == c.Params[0]

	case CtFibMod: // (Fibonacci(X mod N) + Y) mod M = k
		if len(c.Vars) < 2 || len(c.Params) < 3 {
			return false
		}
		N, M, k := c.Params[0], c.Params[1], c.Params[2]
		if N <= 0 || M <= 0 {
			return false
		}
		idx := vals[c.Vars[0]] % N
		if idx < 0 {
			idx += N
		}
		result := (Fibonacci(idx) + vals[c.Vars[1]]) % M
		if result < 0 {
			result += M
		}
		return result == k

	case CtNestedCond: // if X > a AND Y < b then Z > c
		if len(c.Vars) < 3 || len(c.Params) < 3 {
			return false
		}
		a, b, cv := c.Params[0], c.Params[1], c.Params[2]
		x, y, z := vals[c.Vars[0]], vals[c.Vars[1]], vals[c.Vars[2]]
		if x > a && y < b {
			return z > cv
		}
		return true // condition not met → vacuously true

	case CtXOR: // X XOR Y = k
		if len(c.Vars) < 2 || len(c.Params) < 1 {
			return false
		}
		return vals[c.Vars[0]]^vals[c.Vars[1]] == c.Params[0]

	case CtDigitRoot: // DigitRoot(X * Y) = k
		if len(c.Vars) < 2 || len(c.Params) < 1 {
			return false
		}
		product := vals[c.Vars[0]] * vals[c.Vars[1]]
		return DigitRoot(product) == c.Params[0]
	}

	return false
}

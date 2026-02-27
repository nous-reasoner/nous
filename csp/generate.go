package csp

import (
	"fmt"

	"github.com/nous-chain/nous/crypto"
)

// ceilConstraints computes ceil(numVars * ratio) using deterministic integer arithmetic.
// All CSP ratios are multiples of 0.1, so ratio*10 rounds to an exact integer.
// This avoids platform-dependent floating-point rounding in consensus-critical code.
func ceilConstraints(numVars int, ratio float64) int {
	ratioX10 := int(ratio*10 + 0.5) // deterministic round for multiples of 0.1
	return (numVars*ratioX10 + 9) / 10
}

// levelConfig holds the generation parameters for a difficulty tier.
type levelConfig struct {
	BaseVariables   int
	VariableRange   int     // numVars ∈ [Base, Base+Range)
	ConstraintRatio float64 // numConstraints = ceil(numVars * ratio)
}

var levelConfigs = map[Level]levelConfig{
	Standard: {BaseVariables: 12, VariableRange: 5, ConstraintRatio: 1.4},
}

// constraintWeights defines the weighted distribution for each constraint type.
// Indices correspond to ConstraintType values. Sum = 100.
var constraintWeights = [NumConstraintTypes]int{
	12, // CtLinear
	10, // CtMulMod
	8,  // CtSumSquares
	10, // CtCompare
	8,  // CtModChain
	7,  // CtConditional
	8,  // CtTrilinear
	7,  // CtDivisible
	5,  // CtPrimeNth
	6,  // CtGCD
	5,  // CtFibMod
	5,  // CtNestedCond
	5,  // CtXOR
	4,  // CtDigitRoot
}

// weightedConstraintType selects a constraint type using weighted distribution.
func weightedConstraintType(seed crypto.Hash, tag string) ConstraintType {
	val := int(crypto.DeriveInt(seed, tag) % 100)
	cumulative := 0
	for i, w := range constraintWeights {
		cumulative += w
		if val < cumulative {
			return ConstraintType(i)
		}
	}
	return CtLinear // fallback
}

// GenerateProblem deterministically creates a CSP and a candidate solution
// that is guaranteed to satisfy all constraints.
//
// Same (seed, level) always produces the identical problem and candidate.
func GenerateProblem(seed crypto.Hash, level Level) (*Problem, *Solution) {
	cfg := levelConfigs[level]

	// --- 1. Number of variables ---
	numVars := cfg.BaseVariables + int(crypto.DeriveInt(seed, "num_var")%uint64(cfg.VariableRange))

	// --- 2. Generate variables ---
	vars := make([]Variable, numVars)
	for i := range vars {
		lo := int(crypto.DeriveInt(seed, fmt.Sprintf("lo%d", i)) % 50)
		hi := lo + 20 + int(crypto.DeriveInt(seed, fmt.Sprintf("hi%d", i))%80)
		vars[i] = Variable{
			Name:  fmt.Sprintf("x%d", i),
			Lower: lo,
			Upper: hi,
		}
	}

	// --- 3. Derive candidate solution (within each domain) ---
	cand := make([]int, numVars)
	for i := range cand {
		domSize := uint64(vars[i].Upper - vars[i].Lower + 1)
		cand[i] = vars[i].Lower + int(crypto.DeriveInt(seed, fmt.Sprintf("cand%d", i))%domSize)
	}

	// --- 4. Generate constraints, guaranteed satisfiable by candidate ---
	numConstraints := ceilConstraints(numVars, cfg.ConstraintRatio)
	constraints := make([]Constraint, numConstraints)
	for c := range constraints {
		ctype := weightedConstraintType(seed, fmt.Sprintf("ctype%d", c))
		constraints[c] = buildConstraint(seed, c, ctype, numVars, cand)
	}

	return &Problem{Variables: vars, Constraints: constraints, Level: level},
		&Solution{Values: cand}
}

// GenerateProblemWithParams creates a CSP with custom variable and constraint counts.
func GenerateProblemWithParams(seed crypto.Hash, numVars int, numConstraints int) (*Problem, *Solution) {
	if numVars < 2 {
		numVars = 2
	}
	if numConstraints < 1 {
		numConstraints = 1
	}

	// Generate variables.
	vars := make([]Variable, numVars)
	for i := range vars {
		lo := int(crypto.DeriveInt(seed, fmt.Sprintf("lo%d", i)) % 50)
		hi := lo + 20 + int(crypto.DeriveInt(seed, fmt.Sprintf("hi%d", i))%80)
		vars[i] = Variable{
			Name:  fmt.Sprintf("x%d", i),
			Lower: lo,
			Upper: hi,
		}
	}

	// Derive candidate solution.
	cand := make([]int, numVars)
	for i := range cand {
		domSize := uint64(vars[i].Upper - vars[i].Lower + 1)
		cand[i] = vars[i].Lower + int(crypto.DeriveInt(seed, fmt.Sprintf("cand%d", i))%domSize)
	}

	// Generate constraints.
	constraints := make([]Constraint, numConstraints)
	for c := range constraints {
		ctype := weightedConstraintType(seed, fmt.Sprintf("ctype%d", c))
		constraints[c] = buildConstraint(seed, c, ctype, numVars, cand)
	}

	return &Problem{Variables: vars, Constraints: constraints, Level: Standard},
		&Solution{Values: cand}
}

// buildConstraint dispatches to the per-type builder, which computes
// constant parameters such that the candidate solution satisfies the constraint.
func buildConstraint(seed crypto.Hash, idx int, ct ConstraintType, n int, cand []int) Constraint {
	p := fmt.Sprintf("c%d_", idx)
	switch ct {
	case CtLinear:
		return buildLinear(seed, p, n, cand)
	case CtMulMod:
		return buildMulMod(seed, p, n, cand)
	case CtSumSquares:
		return buildSumSquares(seed, p, n, cand)
	case CtCompare:
		return buildCompare(seed, p, n, cand)
	case CtModChain:
		return buildModChain(seed, p, n, cand)
	case CtConditional:
		return buildConditional(seed, p, n, cand)
	case CtTrilinear:
		return buildTrilinear(seed, p, n, cand)
	case CtDivisible:
		return buildDivisible(seed, p, n, cand)
	case CtPrimeNth:
		return buildPrimeNth(seed, p, n, cand)
	case CtGCD:
		return buildGCD(seed, p, n, cand)
	case CtFibMod:
		return buildFibMod(seed, p, n, cand)
	case CtNestedCond:
		return buildNestedCond(seed, p, n, cand)
	case CtXOR:
		return buildXOR(seed, p, n, cand)
	case CtDigitRoot:
		return buildDigitRoot(seed, p, n, cand)
	default:
		return buildLinear(seed, p, n, cand)
	}
}

// --- per-type builders ---

// buildLinear: a*X + b*Y = c   where c is computed from the candidate.
func buildLinear(seed crypto.Hash, p string, n int, cand []int) Constraint {
	v := crypto.DeriveSubset(seed, p+"v", n, 2)
	a := int(crypto.DeriveInt(seed, p+"a")%10) + 1
	b := int(crypto.DeriveInt(seed, p+"b")%10) + 1
	if crypto.DeriveInt(seed, p+"sa")%2 == 1 {
		a = -a
	}
	if crypto.DeriveInt(seed, p+"sb")%2 == 1 {
		b = -b
	}
	c := a*cand[v[0]] + b*cand[v[1]]
	return Constraint{Type: CtLinear, Vars: v, Params: []int{a, b, c}}
}

// buildMulMod: X*Y mod Z = W   where W is computed from the candidate.
func buildMulMod(seed crypto.Hash, p string, n int, cand []int) Constraint {
	v := crypto.DeriveSubset(seed, p+"v", n, 2)
	z := int(crypto.DeriveInt(seed, p+"z")%50) + 2 // ∈ [2, 51]
	product := cand[v[0]] * cand[v[1]]
	w := product % z
	if w < 0 {
		w += z
	}
	return Constraint{Type: CtMulMod, Vars: v, Params: []int{z, w}}
}

// buildSumSquares: |X² + Y² - Z| ≤ k   where Z = X²+Y² from the candidate.
func buildSumSquares(seed crypto.Hash, p string, n int, cand []int) Constraint {
	v := crypto.DeriveSubset(seed, p+"v", n, 2)
	x, y := cand[v[0]], cand[v[1]]
	z := x*x + y*y                                // exact for candidate → diff=0 ≤ k
	k := int(crypto.DeriveInt(seed, p+"k") % 5)   // tolerance [0,4]
	return Constraint{Type: CtSumSquares, Vars: v, Params: []int{z, k}}
}

// buildCompare: X > Y + k   where k = X−Y−1 (tightest valid value).
func buildCompare(seed crypto.Hash, p string, n int, cand []int) Constraint {
	v := crypto.DeriveSubset(seed, p+"v", n, 2)
	k := cand[v[0]] - cand[v[1]] - 1 // guaranteed: X > Y + (X−Y−1) ⇔ X > X−1 ✓
	return Constraint{Type: CtCompare, Vars: v, Params: []int{k}}
}

// buildModChain: X mod A = Y mod B
func buildModChain(seed crypto.Hash, p string, n int, cand []int) Constraint {
	v := crypto.DeriveSubset(seed, p+"v", n, 2)
	x, y := cand[v[0]], cand[v[1]]

	// Fast path: x == y → any A=B works.
	if x == y {
		a := int(crypto.DeriveInt(seed, p+"a")%20) + 2
		return Constraint{Type: CtModChain, Vars: v, Params: []int{a, a}}
	}

	// Try a derived A, then search for matching B.
	a := int(crypto.DeriveInt(seed, p+"a")%20) + 2
	target := x % a

	for i := 0; i < 10; i++ {
		b := int(crypto.DeriveInt(seed, fmt.Sprintf("%sb%d", p, i))%30) + 1
		if y%b == target {
			return Constraint{Type: CtModChain, Vars: v, Params: []int{a, b}}
		}
	}

	// Fallback: A = B = |x−y|.  Since x ≡ y (mod |x−y|), this always holds.
	diff := x - y
	if diff < 0 {
		diff = -diff
	}
	if diff < 1 {
		diff = 1
	}
	return Constraint{Type: CtModChain, Vars: v, Params: []int{diff, diff}}
}

// buildConditional: if X > k then Y < m else Y > n
func buildConditional(seed crypto.Hash, p string, n int, cand []int) Constraint {
	v := crypto.DeriveSubset(seed, p+"v", n, 2)
	x, y := cand[v[0]], cand[v[1]]
	k := int(crypto.DeriveInt(seed, p+"k") % 100)

	var m, nv int
	if x > k {
		// Active: need Y < m  →  m > y
		m = y + 1 + int(crypto.DeriveInt(seed, p+"m")%20)
		nv = int(crypto.DeriveInt(seed, p+"n") % 50) // inactive
	} else {
		// Active: need Y > n  →  n < y
		nv = y - 1 - int(crypto.DeriveInt(seed, p+"nd")%20)
		m = int(crypto.DeriveInt(seed, p+"m")%200) + 1 // inactive
	}
	return Constraint{Type: CtConditional, Vars: v, Params: []int{k, m, nv}}
}

// buildTrilinear: X*Y + Z = W   where W is computed from the candidate.
func buildTrilinear(seed crypto.Hash, p string, n int, cand []int) Constraint {
	if n < 3 {
		return buildLinear(seed, p, n, cand) // need ≥3 vars
	}
	v := crypto.DeriveSubset(seed, p+"v", n, 3)
	w := cand[v[0]]*cand[v[1]] + cand[v[2]]
	return Constraint{Type: CtTrilinear, Vars: v, Params: []int{w}}
}

// buildDivisible: X mod Y = 0 (Y ≠ 0).
// Searches for a variable pair where divisibility holds in the candidate;
// falls back to Compare if no such pair exists.
func buildDivisible(seed crypto.Hash, p string, n int, cand []int) Constraint {
	for attempt := 0; attempt < 10; attempt++ {
		v := crypto.DeriveSubset(seed, fmt.Sprintf("%sv%d", p, attempt), n, 2)
		x, y := cand[v[0]], cand[v[1]]
		if y != 0 && x%y == 0 {
			return Constraint{Type: CtDivisible, Vars: v, Params: nil}
		}
		if x != 0 && y%x == 0 {
			return Constraint{Type: CtDivisible, Vars: []int{v[1], v[0]}, Params: nil}
		}
	}
	return buildCompare(seed, p, n, cand) // fallback
}

// --- new constraint type builders ---

// buildPrimeNth: NthPrime(X mod N) + Y = k
func buildPrimeNth(seed crypto.Hash, p string, n int, cand []int) Constraint {
	v := crypto.DeriveSubset(seed, p+"v", n, 2)
	N := int(crypto.DeriveInt(seed, p+"n")%8) + 2 // N ∈ [2, 9]
	idx := cand[v[0]] % N
	if idx < 0 {
		idx += N
	}
	k := NthPrime(idx) + cand[v[1]]
	return Constraint{Type: CtPrimeNth, Vars: v, Params: []int{N, k}}
}

// buildGCD: GCD(X, Y) = k
func buildGCD(seed crypto.Hash, p string, n int, cand []int) Constraint {
	v := crypto.DeriveSubset(seed, p+"v", n, 2)
	k := GCD(cand[v[0]], cand[v[1]])
	return Constraint{Type: CtGCD, Vars: v, Params: []int{k}}
}

// buildFibMod: (Fibonacci(X mod N) + Y) mod M = k
func buildFibMod(seed crypto.Hash, p string, n int, cand []int) Constraint {
	v := crypto.DeriveSubset(seed, p+"v", n, 2)
	N := int(crypto.DeriveInt(seed, p+"n")%15) + 2 // N ∈ [2, 16]
	M := int(crypto.DeriveInt(seed, p+"m")%20) + 2 // M ∈ [2, 21]
	idx := cand[v[0]] % N
	if idx < 0 {
		idx += N
	}
	fibVal := Fibonacci(idx)
	k := (fibVal + cand[v[1]]) % M
	if k < 0 {
		k += M
	}
	return Constraint{Type: CtFibMod, Vars: v, Params: []int{N, M, k}}
}

// buildNestedCond: if X > a AND Y < b then Z > c
func buildNestedCond(seed crypto.Hash, p string, n int, cand []int) Constraint {
	if n < 3 {
		return buildLinear(seed, p, n, cand)
	}
	v := crypto.DeriveSubset(seed, p+"v", n, 3)
	x, y, z := cand[v[0]], cand[v[1]], cand[v[2]]
	a := int(crypto.DeriveInt(seed, p+"a") % 100)
	b := int(crypto.DeriveInt(seed, p+"b") % 100)

	if x > a && y < b {
		// Condition met, need Z > c where c < z
		c := z - 1 - int(crypto.DeriveInt(seed, p+"c")%20)
		return Constraint{Type: CtNestedCond, Vars: v, Params: []int{a, b, c}}
	}
	// Condition not met → vacuously true. Any c is fine.
	c := int(crypto.DeriveInt(seed, p+"c") % 100)
	return Constraint{Type: CtNestedCond, Vars: v, Params: []int{a, b, c}}
}

// buildXOR: X XOR Y = k
func buildXOR(seed crypto.Hash, p string, n int, cand []int) Constraint {
	v := crypto.DeriveSubset(seed, p+"v", n, 2)
	k := cand[v[0]] ^ cand[v[1]]
	return Constraint{Type: CtXOR, Vars: v, Params: []int{k}}
}

// buildDigitRoot: DigitRoot(X * Y) = k
func buildDigitRoot(seed crypto.Hash, p string, n int, cand []int) Constraint {
	v := crypto.DeriveSubset(seed, p+"v", n, 2)
	product := cand[v[0]] * cand[v[1]]
	k := DigitRoot(product)
	return Constraint{Type: CtDigitRoot, Vars: v, Params: []int{k}}
}

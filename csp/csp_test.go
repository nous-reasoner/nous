package csp

import (
	"fmt"
	"testing"

	"github.com/nous-chain/nous/crypto"
)

// ============================================================
// Structural tests
// ============================================================

func TestGenerateStandardLevel(t *testing.T) {
	seed := crypto.Sha256([]byte("standard test"))
	prob, sol := GenerateProblem(seed, Standard)

	// base=12, range=5 → numVars ∈ [12, 16]
	if len(prob.Variables) < 12 || len(prob.Variables) > 16 {
		t.Fatalf("standard vars: want [12,16], got %d", len(prob.Variables))
	}
	expectedC := ceilConstraints(len(prob.Variables), 1.4)
	if len(prob.Constraints) != expectedC {
		t.Fatalf("standard constraints: want %d, got %d", expectedC, len(prob.Constraints))
	}
	if prob.Level != Standard {
		t.Fatalf("level: want Standard, got %v", prob.Level)
	}
	if len(sol.Values) != len(prob.Variables) {
		t.Fatalf("solution length mismatch: %d vs %d vars", len(sol.Values), len(prob.Variables))
	}
}

// ============================================================
// Determinism & uniqueness
// ============================================================

func TestDeterministic(t *testing.T) {
	seed := crypto.Sha256([]byte("deterministic"))
	p1, s1 := GenerateProblem(seed, Standard)
	p2, s2 := GenerateProblem(seed, Standard)

	if len(p1.Variables) != len(p2.Variables) {
		t.Fatal("same seed should produce same variable count")
	}
	for i := range p1.Variables {
		if p1.Variables[i] != p2.Variables[i] {
			t.Fatalf("variable %d differs", i)
		}
	}
	if len(p1.Constraints) != len(p2.Constraints) {
		t.Fatal("same seed should produce same constraint count")
	}
	for i := range p1.Constraints {
		if p1.Constraints[i].Type != p2.Constraints[i].Type {
			t.Fatalf("constraint %d type differs", i)
		}
	}
	for i := range s1.Values {
		if s1.Values[i] != s2.Values[i] {
			t.Fatalf("candidate value %d differs", i)
		}
	}
}

func TestDifferentSeeds(t *testing.T) {
	seed1 := crypto.Sha256([]byte("seed A"))
	seed2 := crypto.Sha256([]byte("seed B"))
	p1, _ := GenerateProblem(seed1, Standard)
	p2, _ := GenerateProblem(seed2, Standard)

	// At least one variable should differ (overwhelmingly likely)
	same := true
	for i := range p1.Variables {
		if i >= len(p2.Variables) {
			same = false
			break
		}
		if p1.Variables[i] != p2.Variables[i] {
			same = false
			break
		}
	}
	if len(p1.Variables) != len(p2.Variables) {
		same = false
	}
	if same {
		t.Fatal("different seeds should produce different problems")
	}
}

// ============================================================
// Candidate solution verification
// ============================================================

func TestCandidateSolutionVerifies(t *testing.T) {
	// Run over many seeds to build confidence
	for i := 0; i < 100; i++ {
		seed := crypto.Sha256([]byte("verify_" + string(rune(i))))
		prob, sol := GenerateProblem(seed, Standard)
		if !VerifySolution(prob, sol) {
			t.Fatalf("seed %d: candidate solution should verify (standard)", i)
		}
	}
}

// ============================================================
// Rejection tests
// ============================================================

func TestRejectsOutOfRange(t *testing.T) {
	seed := crypto.Sha256([]byte("out of range"))
	prob, sol := GenerateProblem(seed, Standard)

	bad := &Solution{Values: make([]int, len(sol.Values))}
	copy(bad.Values, sol.Values)
	bad.Values[0] = prob.Variables[0].Upper + 1 // too high

	if VerifySolution(prob, bad) {
		t.Fatal("value above upper bound should be rejected")
	}

	bad.Values[0] = prob.Variables[0].Lower - 1 // too low
	if VerifySolution(prob, bad) {
		t.Fatal("value below lower bound should be rejected")
	}
}

func TestRejectsWrongAssignment(t *testing.T) {
	seed := crypto.Sha256([]byte("wrong assignment"))
	prob, sol := GenerateProblem(seed, Standard)

	bad := &Solution{Values: make([]int, len(sol.Values))}
	copy(bad.Values, sol.Values)

	// Flip multiple values within domain — almost certainly breaks a constraint.
	for i := range bad.Values {
		v := &prob.Variables[i]
		bad.Values[i] = v.Lower + (bad.Values[i]-v.Lower+1)%(v.Upper-v.Lower+1)
	}

	if VerifySolution(prob, bad) {
		t.Fatal("randomized wrong assignment should (overwhelmingly) fail verification")
	}
}

func TestRejectsWrongLength(t *testing.T) {
	seed := crypto.Sha256([]byte("wrong length"))
	prob, _ := GenerateProblem(seed, Standard)

	short := &Solution{Values: []int{1, 2}}
	if VerifySolution(prob, short) {
		t.Fatal("too-short solution should be rejected")
	}
}

func TestRejectsNil(t *testing.T) {
	seed := crypto.Sha256([]byte("nil"))
	prob, _ := GenerateProblem(seed, Standard)

	if VerifySolution(prob, nil) {
		t.Fatal("nil solution should be rejected")
	}
	if VerifySolution(nil, &Solution{Values: []int{}}) {
		t.Fatal("nil problem should be rejected")
	}
}

// ============================================================
// Per-type constraint unit tests
// ============================================================

func makeTwoVarProblem(x, y int) (*Problem, []int) {
	return &Problem{
		Variables: []Variable{
			{Name: "X", Lower: 0, Upper: 200},
			{Name: "Y", Lower: 0, Upper: 200},
		},
	}, []int{x, y}
}

func makeThreeVarProblem(x, y, z int) (*Problem, []int) {
	return &Problem{
		Variables: []Variable{
			{Name: "X", Lower: 0, Upper: 200},
			{Name: "Y", Lower: 0, Upper: 200},
			{Name: "Z", Lower: 0, Upper: 200},
		},
	}, []int{x, y, z}
}

func TestCtLinear(t *testing.T) {
	prob, vals := makeTwoVarProblem(10, 20)
	// 3*10 + (-2)*20 = 30 - 40 = -10
	prob.Constraints = []Constraint{
		{Type: CtLinear, Vars: []int{0, 1}, Params: []int{3, -2, -10}},
	}
	if !VerifySolution(prob, &Solution{Values: vals}) {
		t.Fatal("Linear: should pass for 3*10 + (-2)*20 = -10")
	}

	prob.Constraints[0].Params[2] = 99 // wrong c
	if VerifySolution(prob, &Solution{Values: vals}) {
		t.Fatal("Linear: should fail for wrong c")
	}
}

func TestCtMulMod(t *testing.T) {
	prob, vals := makeTwoVarProblem(7, 8)
	// 7*8 = 56, 56 mod 10 = 6
	prob.Constraints = []Constraint{
		{Type: CtMulMod, Vars: []int{0, 1}, Params: []int{10, 6}},
	}
	if !VerifySolution(prob, &Solution{Values: vals}) {
		t.Fatal("MulMod: should pass for 7*8 mod 10 = 6")
	}

	prob.Constraints[0].Params[1] = 5 // wrong W
	if VerifySolution(prob, &Solution{Values: vals}) {
		t.Fatal("MulMod: should fail for wrong W")
	}
}

func TestCtSumSquares(t *testing.T) {
	prob, vals := makeTwoVarProblem(3, 4)
	// 9 + 16 = 25, |25 - 25| = 0 ≤ 2
	prob.Constraints = []Constraint{
		{Type: CtSumSquares, Vars: []int{0, 1}, Params: []int{25, 2}},
	}
	if !VerifySolution(prob, &Solution{Values: vals}) {
		t.Fatal("SumSquares: should pass for |9+16-25|=0 ≤ 2")
	}

	// |25 - 30| = 5, 5 ≤ 2? No.
	prob.Constraints[0].Params[0] = 30
	prob.Constraints[0].Params[1] = 2
	if VerifySolution(prob, &Solution{Values: vals}) {
		t.Fatal("SumSquares: should fail for |25-30|=5 > 2")
	}

	// Tolerance exactly at boundary: |25 - 30| = 5 ≤ 5
	prob.Constraints[0].Params[1] = 5
	if !VerifySolution(prob, &Solution{Values: vals}) {
		t.Fatal("SumSquares: should pass for |25-30|=5 ≤ 5")
	}
}

func TestCtCompare(t *testing.T) {
	prob, vals := makeTwoVarProblem(50, 30)
	// 50 > 30 + 10  →  50 > 40 ✓
	prob.Constraints = []Constraint{
		{Type: CtCompare, Vars: []int{0, 1}, Params: []int{10}},
	}
	if !VerifySolution(prob, &Solution{Values: vals}) {
		t.Fatal("Compare: should pass for 50 > 30+10")
	}

	// 50 > 30 + 20  →  50 > 50? No (strict).
	prob.Constraints[0].Params[0] = 20
	if VerifySolution(prob, &Solution{Values: vals}) {
		t.Fatal("Compare: should fail for 50 > 50 (not strict >)")
	}

	// Negative k: 50 > 30 + (-5)  →  50 > 25 ✓
	prob.Constraints[0].Params[0] = -5
	if !VerifySolution(prob, &Solution{Values: vals}) {
		t.Fatal("Compare: should pass for 50 > 25")
	}
}

func TestCtModChain(t *testing.T) {
	prob, vals := makeTwoVarProblem(17, 11)
	// 17 mod 5 = 2, 11 mod 3 = 2  →  equal
	prob.Constraints = []Constraint{
		{Type: CtModChain, Vars: []int{0, 1}, Params: []int{5, 3}},
	}
	if !VerifySolution(prob, &Solution{Values: vals}) {
		t.Fatal("ModChain: should pass for 17%%5==2 == 11%%3==2")
	}

	// 17 mod 5 = 2, 11 mod 4 = 3  →  not equal
	prob.Constraints[0].Params[1] = 4
	if VerifySolution(prob, &Solution{Values: vals}) {
		t.Fatal("ModChain: should fail for 2 != 3")
	}
}

func TestCtConditional(t *testing.T) {
	// Branch 1: X > k
	prob, vals := makeTwoVarProblem(60, 30)
	// if 60 > 50 then 30 < 40 else 30 > 10  →  branch 1: 30 < 40 ✓
	prob.Constraints = []Constraint{
		{Type: CtConditional, Vars: []int{0, 1}, Params: []int{50, 40, 10}},
	}
	if !VerifySolution(prob, &Solution{Values: vals}) {
		t.Fatal("Conditional: branch1 should pass")
	}

	// Branch 1 fail: if 60 > 50 then 30 < 20  →  30 < 20? No.
	prob.Constraints[0].Params[1] = 20
	if VerifySolution(prob, &Solution{Values: vals}) {
		t.Fatal("Conditional: branch1 should fail for Y=30 < 20")
	}

	// Branch 2: X <= k
	prob2, vals2 := makeTwoVarProblem(40, 30)
	// if 40 > 50 → false → else 30 > 10 ✓
	prob2.Constraints = []Constraint{
		{Type: CtConditional, Vars: []int{0, 1}, Params: []int{50, 100, 10}},
	}
	if !VerifySolution(prob2, &Solution{Values: vals2}) {
		t.Fatal("Conditional: branch2 should pass for Y=30 > 10")
	}

	// Branch 2 fail: else 30 > 40? No.
	prob2.Constraints[0].Params[2] = 40
	if VerifySolution(prob2, &Solution{Values: vals2}) {
		t.Fatal("Conditional: branch2 should fail for Y=30 > 40")
	}
}

func TestCtTrilinear(t *testing.T) {
	prob, vals := makeThreeVarProblem(5, 6, 7)
	// 5*6 + 7 = 37
	prob.Constraints = []Constraint{
		{Type: CtTrilinear, Vars: []int{0, 1, 2}, Params: []int{37}},
	}
	if !VerifySolution(prob, &Solution{Values: vals}) {
		t.Fatal("Trilinear: should pass for 5*6+7=37")
	}

	prob.Constraints[0].Params[0] = 38
	if VerifySolution(prob, &Solution{Values: vals}) {
		t.Fatal("Trilinear: should fail for W=38")
	}
}

func TestCtDivisible(t *testing.T) {
	prob, vals := makeTwoVarProblem(12, 4)
	// 12 mod 4 = 0 ✓
	prob.Constraints = []Constraint{
		{Type: CtDivisible, Vars: []int{0, 1}, Params: nil},
	}
	if !VerifySolution(prob, &Solution{Values: vals}) {
		t.Fatal("Divisible: should pass for 12 %% 4 = 0")
	}

	// 12 mod 5 = 2 ≠ 0
	prob2, vals2 := makeTwoVarProblem(12, 5)
	prob2.Constraints = []Constraint{
		{Type: CtDivisible, Vars: []int{0, 1}, Params: nil},
	}
	if VerifySolution(prob2, &Solution{Values: vals2}) {
		t.Fatal("Divisible: should fail for 12 %% 5 ≠ 0")
	}

	// Y=0 is always rejected
	prob3, vals3 := makeTwoVarProblem(0, 0)
	prob3.Constraints = []Constraint{
		{Type: CtDivisible, Vars: []int{0, 1}, Params: nil},
	}
	if VerifySolution(prob3, &Solution{Values: vals3}) {
		t.Fatal("Divisible: Y=0 should be rejected")
	}
}

// ============================================================
// Constraint type coverage
// ============================================================

func TestAllConstraintTypesPresent(t *testing.T) {
	// Over many seeds, all 14 types should appear at least once.
	seen := make(map[ConstraintType]bool)
	for i := 0; i < 200; i++ {
		seed := crypto.Sha256([]byte("coverage_" + string(rune(i))))
		prob, _ := GenerateProblem(seed, Standard)
		for _, c := range prob.Constraints {
			seen[c.Type] = true
		}
	}
	for ct := ConstraintType(0); ct < NumConstraintTypes; ct++ {
		if !seen[ct] {
			// Divisible may fall back to Compare, so it might not always appear
			if ct == CtDivisible {
				continue
			}
			t.Fatalf("constraint type %v never generated in 200 seeds", ct)
		}
	}
}

// ============================================================
// Math helper tests
// ============================================================

func TestMathIsPrimeAndNthPrime(t *testing.T) {
	// IsPrime spot checks.
	primes := []int{2, 3, 5, 7, 11, 13, 17, 19, 23, 29, 97}
	for _, p := range primes {
		if !IsPrime(p) {
			t.Fatalf("IsPrime(%d) should be true", p)
		}
	}
	nonPrimes := []int{-1, 0, 1, 4, 6, 8, 9, 15, 21, 100}
	for _, np := range nonPrimes {
		if IsPrime(np) {
			t.Fatalf("IsPrime(%d) should be false", np)
		}
	}

	// NthPrime: first few primes.
	expected := []int{2, 3, 5, 7, 11, 13, 17, 19, 23, 29}
	for i, want := range expected {
		got := NthPrime(i)
		if got != want {
			t.Fatalf("NthPrime(%d): want %d, got %d", i, want, got)
		}
	}
	// NthPrime(negative) = 2
	if NthPrime(-5) != 2 {
		t.Fatal("NthPrime(-5) should be 2")
	}
}

func TestMathGCD(t *testing.T) {
	cases := [][3]int{
		{12, 8, 4},
		{7, 13, 1},
		{0, 5, 5},
		{0, 0, 0},
		{100, 75, 25},
		{-12, 8, 4},
		{17, 17, 17},
	}
	for _, tc := range cases {
		got := GCD(tc[0], tc[1])
		if got != tc[2] {
			t.Fatalf("GCD(%d, %d): want %d, got %d", tc[0], tc[1], tc[2], got)
		}
	}
}

func TestMathFibonacci(t *testing.T) {
	expected := []int{0, 1, 1, 2, 3, 5, 8, 13, 21, 34, 55}
	for i, want := range expected {
		got := Fibonacci(i)
		if got != want {
			t.Fatalf("Fibonacci(%d): want %d, got %d", i, want, got)
		}
	}
	if Fibonacci(-1) != 0 {
		t.Fatal("Fibonacci(-1) should be 0")
	}
}

func TestMathDigitRoot(t *testing.T) {
	cases := [][2]int{
		{0, 0}, {1, 1}, {9, 9}, {10, 1}, {18, 9},
		{123, 6}, {999, 9}, {493, 7}, {-15, 6},
	}
	for _, tc := range cases {
		got := DigitRoot(tc[0])
		if got != tc[1] {
			t.Fatalf("DigitRoot(%d): want %d, got %d", tc[0], tc[1], got)
		}
	}
}

// ============================================================
// New constraint type unit tests
// ============================================================

func TestCtPrimeNth(t *testing.T) {
	prob, vals := makeTwoVarProblem(10, 20)
	// NthPrime(10 mod 4) + 20 = NthPrime(2) + 20 = 5 + 20 = 25
	prob.Constraints = []Constraint{
		{Type: CtPrimeNth, Vars: []int{0, 1}, Params: []int{4, 25}},
	}
	if !VerifySolution(prob, &Solution{Values: vals}) {
		t.Fatal("PrimeNth: should pass for NthPrime(10%%4)+20=25")
	}
	prob.Constraints[0].Params[1] = 99
	if VerifySolution(prob, &Solution{Values: vals}) {
		t.Fatal("PrimeNth: should fail for wrong k")
	}
}

func TestCtGCD(t *testing.T) {
	prob, vals := makeTwoVarProblem(12, 8)
	// GCD(12, 8) = 4
	prob.Constraints = []Constraint{
		{Type: CtGCD, Vars: []int{0, 1}, Params: []int{4}},
	}
	if !VerifySolution(prob, &Solution{Values: vals}) {
		t.Fatal("GCD: should pass for GCD(12,8)=4")
	}
	prob.Constraints[0].Params[0] = 3
	if VerifySolution(prob, &Solution{Values: vals}) {
		t.Fatal("GCD: should fail for wrong k")
	}
}

func TestCtFibMod(t *testing.T) {
	prob, vals := makeTwoVarProblem(7, 30)
	// Fibonacci(7 mod 5) = Fibonacci(2) = 1; (1 + 30) mod 10 = 1
	prob.Constraints = []Constraint{
		{Type: CtFibMod, Vars: []int{0, 1}, Params: []int{5, 10, 1}},
	}
	if !VerifySolution(prob, &Solution{Values: vals}) {
		t.Fatal("FibMod: should pass for (Fib(7%%5)+30)%%10=1")
	}
	prob.Constraints[0].Params[2] = 9
	if VerifySolution(prob, &Solution{Values: vals}) {
		t.Fatal("FibMod: should fail for wrong k")
	}
}

func TestCtNestedCond(t *testing.T) {
	prob, vals := makeThreeVarProblem(80, 30, 50)
	// X=80 > 50 AND Y=30 < 40 → true → Z=50 > 10 ✓
	prob.Constraints = []Constraint{
		{Type: CtNestedCond, Vars: []int{0, 1, 2}, Params: []int{50, 40, 10}},
	}
	if !VerifySolution(prob, &Solution{Values: vals}) {
		t.Fatal("NestedCond: should pass when condition met and Z > c")
	}

	// Z=50 > 60? No.
	prob.Constraints[0].Params[2] = 60
	if VerifySolution(prob, &Solution{Values: vals}) {
		t.Fatal("NestedCond: should fail when condition met but Z <= c")
	}

	// Condition not met: X=80 > 90? No → vacuously true
	prob.Constraints[0].Params[0] = 90
	prob.Constraints[0].Params[2] = 200
	if !VerifySolution(prob, &Solution{Values: vals}) {
		t.Fatal("NestedCond: should pass vacuously when condition not met")
	}
}

func TestCtXOR(t *testing.T) {
	prob, vals := makeTwoVarProblem(0b1010, 0b1100)
	// 10 XOR 12 = 0b0110 = 6
	prob.Constraints = []Constraint{
		{Type: CtXOR, Vars: []int{0, 1}, Params: []int{6}},
	}
	if !VerifySolution(prob, &Solution{Values: vals}) {
		t.Fatal("XOR: should pass for 10^12=6")
	}
	prob.Constraints[0].Params[0] = 7
	if VerifySolution(prob, &Solution{Values: vals}) {
		t.Fatal("XOR: should fail for wrong k")
	}
}

func TestCtDigitRoot(t *testing.T) {
	prob, vals := makeTwoVarProblem(7, 8)
	// DigitRoot(7 * 8) = DigitRoot(56) = 1 + (56-1)%9 = 1+1 = 2
	prob.Constraints = []Constraint{
		{Type: CtDigitRoot, Vars: []int{0, 1}, Params: []int{2}},
	}
	if !VerifySolution(prob, &Solution{Values: vals}) {
		t.Fatal("DigitRoot: should pass for DigitRoot(56)=2")
	}
	prob.Constraints[0].Params[0] = 5
	if VerifySolution(prob, &Solution{Values: vals}) {
		t.Fatal("DigitRoot: should fail for wrong k")
	}

	// Edge case: product = 0 → DigitRoot(0) = 0
	prob2, vals2 := makeTwoVarProblem(0, 42)
	prob2.Constraints = []Constraint{
		{Type: CtDigitRoot, Vars: []int{0, 1}, Params: []int{0}},
	}
	if !VerifySolution(prob2, &Solution{Values: vals2}) {
		t.Fatal("DigitRoot: should pass for DigitRoot(0)=0")
	}
}

func TestSatisfiabilityGuarantee(t *testing.T) {
	for i := 0; i < 10000; i++ {
		seed := crypto.Sha256([]byte(fmt.Sprintf("stress-%d", i)))
		problem, candidate := GenerateProblem(seed, Standard)

		if !VerifySolution(problem, candidate) {
			t.Fatalf("seed %d: candidate solution failed verification, vars=%d constraints=%d",
				i, len(problem.Variables), len(problem.Constraints))
		}
	}
	t.Logf("All 10000 problems have valid candidate solutions")
}

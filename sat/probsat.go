package sat

import (
	"crypto/sha256"
	"errors"
	"math"
	"math/rand"
	"time"
)

const (
	probSATcb    = 2.3
	probSATeps   = 0.9
	probSATMaxSt = 1000 // max stored solutions for enumerate
)

// probSATState holds incremental data structures for ProbSAT.
type probSATState struct {
	f        Formula
	n        int
	a        Assignment
	rng      *rand.Rand
	posIn    [][]int // posIn[v] = clause indices where v appears positive
	negIn    [][]int // negIn[v] = clause indices where v appears negative
	satCount []int   // satCount[ci] = number of currently satisfied literals in clause ci
	unsatSet []int   // list of unsatisfied clause indices
	unsatPos []int   // unsatPos[ci] = index in unsatSet, or -1
}

func newProbSATState(f Formula, n int, rng *rand.Rand) *probSATState {
	s := &probSATState{
		f:        f,
		n:        n,
		a:        make(Assignment, n),
		rng:      rng,
		posIn:    make([][]int, n),
		negIn:    make([][]int, n),
		satCount: make([]int, len(f)),
		unsatPos: make([]int, len(f)),
	}
	// Build var→clause index.
	for ci, c := range f {
		for _, lit := range c {
			if lit.Var < n {
				if lit.Neg {
					s.negIn[lit.Var] = append(s.negIn[lit.Var], ci)
				} else {
					s.posIn[lit.Var] = append(s.posIn[lit.Var], ci)
				}
			}
		}
	}
	return s
}

// randomInit sets a random assignment and computes satCount/unsatSet from scratch.
func (s *probSATState) randomInit() {
	for i := range s.a {
		s.a[i] = s.rng.Intn(2) == 1
	}
	s.unsatSet = s.unsatSet[:0]
	for ci, c := range s.f {
		cnt := 0
		for _, lit := range c {
			if lit.Var < s.n {
				val := s.a[lit.Var]
				if lit.Neg {
					val = !val
				}
				if val {
					cnt++
				}
			}
		}
		s.satCount[ci] = cnt
		if cnt == 0 {
			s.unsatPos[ci] = len(s.unsatSet)
			s.unsatSet = append(s.unsatSet, ci)
		} else {
			s.unsatPos[ci] = -1
		}
	}
}

// flip toggles variable v and incrementally updates satCount and unsatSet.
func (s *probSATState) flip(v int) {
	wasTrue := s.a[v]
	s.a[v] = !wasTrue

	// Clauses where v appears positive:
	// If v was true → literal was satisfied, now unsatisfied → satCount--
	// If v was false → literal was unsatisfied, now satisfied → satCount++
	if wasTrue {
		// positive literals lose satisfaction
		for _, ci := range s.posIn[v] {
			s.satCount[ci]--
			if s.satCount[ci] == 0 {
				s.addUnsat(ci)
			}
		}
		// negative literals gain satisfaction
		for _, ci := range s.negIn[v] {
			if s.satCount[ci] == 0 {
				s.removeUnsat(ci)
			}
			s.satCount[ci]++
		}
	} else {
		// positive literals gain satisfaction
		for _, ci := range s.posIn[v] {
			if s.satCount[ci] == 0 {
				s.removeUnsat(ci)
			}
			s.satCount[ci]++
		}
		// negative literals lose satisfaction
		for _, ci := range s.negIn[v] {
			s.satCount[ci]--
			if s.satCount[ci] == 0 {
				s.addUnsat(ci)
			}
		}
	}
}

func (s *probSATState) addUnsat(ci int) {
	s.unsatPos[ci] = len(s.unsatSet)
	s.unsatSet = append(s.unsatSet, ci)
}

func (s *probSATState) removeUnsat(ci int) {
	pos := s.unsatPos[ci]
	last := len(s.unsatSet) - 1
	if pos != last {
		other := s.unsatSet[last]
		s.unsatSet[pos] = other
		s.unsatPos[other] = pos
	}
	s.unsatSet = s.unsatSet[:last]
	s.unsatPos[ci] = -1
}

// breakValue returns how many currently satisfied clauses would become
// unsatisfied if variable v is flipped (i.e. clauses with satCount == 1
// where v is the sole satisfying literal).
func (s *probSATState) breakValue(v int) int {
	breaks := 0
	if s.a[v] {
		// v is true; flipping makes positive literals false
		for _, ci := range s.posIn[v] {
			if s.satCount[ci] == 1 {
				breaks++
			}
		}
	} else {
		// v is false; flipping makes negative literals false
		for _, ci := range s.negIn[v] {
			if s.satCount[ci] == 1 {
				breaks++
			}
		}
	}
	return breaks
}

// ProbSATSolve finds one satisfying assignment using ProbSAT.
func ProbSATSolve(f Formula, n int, timeout time.Duration) (Assignment, error) {
	if len(f) == 0 || n == 0 {
		return nil, errors.New("empty formula")
	}
	deadline := time.Now().Add(timeout)
	maxFlips := n * 10
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	s := newProbSATState(f, n, rng)

	var weights [3]float64

	for time.Now().Before(deadline) {
		s.randomInit()

		for flip := 0; flip < maxFlips; flip++ {
			if len(s.unsatSet) == 0 {
				out := make(Assignment, n)
				copy(out, s.a)
				return out, nil
			}

			// Pick random unsatisfied clause.
			ci := s.unsatSet[rng.Intn(len(s.unsatSet))]
			c := f[ci]

			// Compute weights for each literal's variable.
			totalW := 0.0
			for j, lit := range c {
				if lit.Var >= n {
					weights[j] = 0
					continue
				}
				brk := s.breakValue(lit.Var)
				w := math.Pow(probSATeps+float64(brk), -probSATcb)
				weights[j] = w
				totalW += w
			}

			// Weighted random selection.
			r := rng.Float64() * totalW
			chosen := 0
			cum := 0.0
			for j := range c {
				cum += weights[j]
				if r < cum {
					chosen = j
					break
				}
			}

			s.flip(c[chosen].Var)
		}
	}

	return nil, errors.New("timeout")
}

// ProbSATEnumerate enumerates distinct solutions using ProbSAT within timeout.
func ProbSATEnumerate(f Formula, n int, timeout time.Duration) (count int, solutions []Assignment) {
	if len(f) == 0 || n == 0 {
		return 0, nil
	}
	deadline := time.Now().Add(timeout)
	maxFlips := n * 10
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	s := newProbSATState(f, n, rng)

	seen := make(map[[32]byte]struct{})
	var weights [3]float64

	for time.Now().Before(deadline) {
		s.randomInit()

		for flip := 0; flip < maxFlips && time.Now().Before(deadline); flip++ {
			if len(s.unsatSet) == 0 {
				h := hashAssign(s.a)
				if _, dup := seen[h]; !dup {
					seen[h] = struct{}{}
					count++
					if len(solutions) < probSATMaxSt {
						cp := make(Assignment, n)
						copy(cp, s.a)
						solutions = append(solutions, cp)
					}
				}
				// Perturb to continue searching.
				flips := 1 + rng.Intn(3)
				for k := 0; k < flips; k++ {
					s.flip(rng.Intn(n))
				}
				continue
			}

			ci := s.unsatSet[rng.Intn(len(s.unsatSet))]
			c := f[ci]

			totalW := 0.0
			for j, lit := range c {
				if lit.Var >= n {
					weights[j] = 0
					continue
				}
				brk := s.breakValue(lit.Var)
				w := math.Pow(probSATeps+float64(brk), -probSATcb)
				weights[j] = w
				totalW += w
			}

			r := rng.Float64() * totalW
			chosen := 0
			cum := 0.0
			for j := range c {
				cum += weights[j]
				if r < cum {
					chosen = j
					break
				}
			}

			s.flip(c[chosen].Var)
		}
	}
	return count, solutions
}

func hashAssign(a Assignment) [32]byte {
	b := make([]byte, (len(a)+7)/8)
	for i, v := range a {
		if v {
			b[i/8] |= 1 << (uint(i) % 8)
		}
	}
	return sha256.Sum256(b)
}

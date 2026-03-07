package sat

import (
	"errors"
	"math"
	"math/rand"
	"time"
)

const (
	probSATcb  = 2.3
	probSATeps = 0.9
)

type probSATState struct {
	f        Formula
	n        int
	a        Assignment
	rng      *rand.Rand
	posIn    [][]int
	negIn    [][]int
	satCount []int
	unsatSet []int
	unsatPos []int
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

func (s *probSATState) flip(v int) {
	wasTrue := s.a[v]
	s.a[v] = !wasTrue
	if wasTrue {
		for _, ci := range s.posIn[v] {
			s.satCount[ci]--
			if s.satCount[ci] == 0 {
				s.addUnsat(ci)
			}
		}
		for _, ci := range s.negIn[v] {
			if s.satCount[ci] == 0 {
				s.removeUnsat(ci)
			}
			s.satCount[ci]++
		}
	} else {
		for _, ci := range s.posIn[v] {
			if s.satCount[ci] == 0 {
				s.removeUnsat(ci)
			}
			s.satCount[ci]++
		}
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

func (s *probSATState) breakValue(v int) int {
	breaks := 0
	if s.a[v] {
		for _, ci := range s.posIn[v] {
			if s.satCount[ci] == 1 {
				breaks++
			}
		}
	} else {
		for _, ci := range s.negIn[v] {
			if s.satCount[ci] == 1 {
				breaks++
			}
		}
	}
	return breaks
}

// Solve finds a satisfying assignment using ProbSAT.
func Solve(f Formula, n int, timeout time.Duration) (Assignment, error) {
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
	return nil, errors.New("timeout")
}

// SolveWithInitial finds a satisfying assignment starting from the given initial assignment.
func SolveWithInitial(f Formula, n int, initial Assignment, timeout time.Duration) (Assignment, error) {
	if len(f) == 0 || n == 0 {
		return nil, errors.New("empty formula")
	}
	deadline := time.Now().Add(timeout)
	maxFlips := n * 10
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	s := newProbSATState(f, n, rng)

	var weights [3]float64

	// First attempt: use provided initial assignment.
	copy(s.a, initial)
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

	firstRound := true
	for time.Now().Before(deadline) {
		if !firstRound {
			s.randomInit()
		}
		firstRound = false

		for flip := 0; flip < maxFlips; flip++ {
			if len(s.unsatSet) == 0 {
				out := make(Assignment, n)
				copy(out, s.a)
				return out, nil
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
	return nil, errors.New("timeout")
}

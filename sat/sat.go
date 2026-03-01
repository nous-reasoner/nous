package sat

import (
	"encoding/binary"
	"math"

	"crypto/sha3"
)

// Literal represents a Boolean literal: variable Var, optionally negated.
type Literal struct {
	Var int
	Neg bool
}

// Clause is a disjunction of literals.
type Clause []Literal

// Formula is a conjunction of clauses (CNF).
type Formula []Clause

// Assignment maps variable index → true/false.
type Assignment []bool

// GenerateFormula deterministically generates a random 3-SAT formula with n
// variables and m = ceil(n*r) clauses. All randomness is derived from seed
// using SHAKE256.
func GenerateFormula(seed [32]byte, n int, r float64) Formula {
	if n < 1 {
		n = 1
	}
	m := int(math.Ceil(float64(n) * r))
	if m < 1 {
		m = 1
	}

	// Use SHAKE256 as a deterministic PRNG seeded by seed.
	// We need 3 variable indices (each 4 bytes) + 3 sign bits (1 byte) per clause = 13 bytes per clause.
	// Over-allocate to keep it simple: 16 bytes per clause.
	buf := make([]byte, m*16)
	shake := sha3.NewSHAKE256()
	shake.Write(seed[:])
	shake.Read(buf)

	f := make(Formula, m)
	for i := 0; i < m; i++ {
		off := i * 16
		c := make(Clause, 3)
		for j := 0; j < 3; j++ {
			v := int(binary.BigEndian.Uint32(buf[off+j*4:off+j*4+4]) % uint32(n))
			neg := buf[off+12+j]&1 == 1
			c[j] = Literal{Var: v, Neg: neg}
		}
		f[i] = c
	}
	return f
}

// Verify checks whether assignment a satisfies every clause of f.
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

// SerializeAssignment packs a into a compact bit array.
// Bit i of byte i/8 (LSB first within each byte) is set iff a[i] == true.
// Length prefix: first 4 bytes are big-endian uint32 of len(a).
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

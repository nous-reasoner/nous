package sat

import (
	"crypto/sha256"
	"encoding/binary"
	"math"

	"golang.org/x/crypto/sha3"
)

const (
	SATVariables   = 256
	SATClausesRatio = 3.85
)

// MakeSATSeed derives a deterministic 32-byte seed for SAT formula generation.
// Matches consensus.MakeSATSeed exactly: SHA256(prevHash || seed_le_bytes).
func MakeSATSeed(prevHash [32]byte, seed uint64) [32]byte {
	var buf [40]byte
	copy(buf[:32], prevHash[:])
	binary.LittleEndian.PutUint64(buf[32:], seed)
	return sha256.Sum256(buf[:])
}

// GenerateFormula deterministically generates a random 3-SAT formula.
// Matches sat.GenerateFormula exactly: SHAKE256 PRNG, BigEndian variable indices.
func GenerateFormula(seed [32]byte, n int, r float64) Formula {
	if n < 1 {
		n = 1
	}
	m := int(math.Ceil(float64(n) * r))
	if m < 1 {
		m = 1
	}

	buf := make([]byte, m*16)
	shake := sha3.NewShake256()
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

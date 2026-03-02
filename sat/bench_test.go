package sat

import (
	"crypto/sha256"
	"encoding/binary"
	"sort"
	"testing"
	"time"
)

func BenchmarkProbSATSolve_256(b *testing.B) {
	for i := 0; i < b.N; i++ {
		var seed [32]byte
		binary.BigEndian.PutUint64(seed[:8], uint64(i))
		seed = sha256.Sum256(seed[:])
		f := GenerateFormula(seed, 256, 3.85)
		_, err := ProbSATSolve(f, 256, 10*time.Second)
		if err != nil {
			// Some seeds produce hard instances; skip rather than fail.
			continue
		}
	}
}

func BenchmarkGenerateFormula_256(b *testing.B) {
	var seed [32]byte
	for i := 0; i < b.N; i++ {
		binary.BigEndian.PutUint64(seed[:8], uint64(i))
		GenerateFormula(seed, 256, 3.85)
	}
}

func TestProbSATSuccessRate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping long test")
	}

	total := 10000
	success := 0
	timeout := 0
	var durations []time.Duration

	for i := 0; i < total; i++ {
		var seed [32]byte
		binary.BigEndian.PutUint64(seed[:8], uint64(i))
		seed = sha256.Sum256(seed[:])
		f := GenerateFormula(seed, 256, 3.85)

		start := time.Now()
		sol, err := ProbSATSolve(f, 256, 10*time.Second)
		elapsed := time.Since(start)

		if err != nil {
			timeout++
			t.Logf("seed %d: TIMEOUT (10s)", i)
			continue
		}

		if !Verify(f, sol) {
			t.Fatalf("seed %d: solution does not verify!", i)
		}

		success++
		durations = append(durations, elapsed)
	}

	// Sort durations for percentiles
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })

	t.Logf("=== ProbSAT n=256 r=3.85 Success Rate ===")
	t.Logf("Total: %d", total)
	t.Logf("Success: %d (%.2f%%)", success, float64(success)/float64(total)*100)
	t.Logf("Timeout: %d (%.2f%%)", timeout, float64(timeout)/float64(total)*100)

	if len(durations) > 0 {
		t.Logf("P50: %v", durations[len(durations)*50/100])
		t.Logf("P90: %v", durations[len(durations)*90/100])
		t.Logf("P95: %v", durations[len(durations)*95/100])
		t.Logf("P99: %v", durations[len(durations)*99/100])
		t.Logf("Max: %v", durations[len(durations)-1])

		var sum time.Duration
		for _, d := range durations {
			sum += d
		}
		t.Logf("Mean: %v", sum/time.Duration(len(durations)))
	}
}

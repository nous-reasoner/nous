package crypto

import (
	"testing"
)

func TestDeriveIntDeterministic(t *testing.T) {
	seed := Sha256([]byte("test seed"))

	a := DeriveInt(seed, "label_a")
	b := DeriveInt(seed, "label_a")
	if a != b {
		t.Fatal("same seed + label should produce the same result")
	}

	c := DeriveInt(seed, "label_b")
	if a == c {
		t.Fatal("different labels should produce different results")
	}
}

func TestDeriveIntDifferentSeeds(t *testing.T) {
	seed1 := Sha256([]byte("seed one"))
	seed2 := Sha256([]byte("seed two"))

	a := DeriveInt(seed1, "same_label")
	b := DeriveInt(seed2, "same_label")
	if a == b {
		t.Fatal("different seeds should produce different results")
	}
}

func TestDeriveInts(t *testing.T) {
	seed := Sha256([]byte("test seed"))
	vals := DeriveInts(seed, "coeff", 5)

	if len(vals) != 5 {
		t.Fatalf("expected 5 values, got %d", len(vals))
	}

	// All values should be distinct (probabilistically guaranteed for 5 values).
	seen := make(map[uint64]bool)
	for _, v := range vals {
		if seen[v] {
			t.Fatalf("duplicate value in DeriveInts output: %d", v)
		}
		seen[v] = true
	}

	// Deterministic.
	vals2 := DeriveInts(seed, "coeff", 5)
	for i := range vals {
		if vals[i] != vals2[i] {
			t.Fatalf("DeriveInts not deterministic at index %d", i)
		}
	}
}

func TestDeriveSubsetBounds(t *testing.T) {
	seed := Sha256([]byte("subset test"))

	indices := DeriveSubset(seed, "vars", 10, 3)
	if len(indices) != 3 {
		t.Fatalf("expected 3 indices, got %d", len(indices))
	}
	for _, idx := range indices {
		if idx < 0 || idx >= 10 {
			t.Fatalf("index %d out of range [0, 10)", idx)
		}
	}
}

func TestDeriveSubsetDistinct(t *testing.T) {
	seed := Sha256([]byte("distinct test"))

	indices := DeriveSubset(seed, "pick", 20, 10)
	if len(indices) != 10 {
		t.Fatalf("expected 10 indices, got %d", len(indices))
	}

	seen := make(map[int]bool)
	for _, idx := range indices {
		if seen[idx] {
			t.Fatalf("duplicate index %d in DeriveSubset output", idx)
		}
		seen[idx] = true
	}
}

func TestDeriveSubsetCountExceedsN(t *testing.T) {
	seed := Sha256([]byte("overflow test"))

	// Requesting more than available should clamp to n.
	indices := DeriveSubset(seed, "all", 5, 100)
	if len(indices) != 5 {
		t.Fatalf("expected 5 indices (clamped), got %d", len(indices))
	}

	seen := make(map[int]bool)
	for _, idx := range indices {
		seen[idx] = true
	}
	if len(seen) != 5 {
		t.Fatal("all 5 indices should be distinct and cover [0,5)")
	}
}

func TestDeriveSubsetDeterministic(t *testing.T) {
	seed := Sha256([]byte("deterministic"))

	a := DeriveSubset(seed, "test", 50, 5)
	b := DeriveSubset(seed, "test", 50, 5)

	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("DeriveSubset not deterministic at index %d: %d vs %d", i, a[i], b[i])
		}
	}
}

func TestDeriveHashDeterministic(t *testing.T) {
	seed := Sha256([]byte("hash derive"))

	h1 := DeriveHash(seed, "challenge")
	h2 := DeriveHash(seed, "challenge")
	if h1 != h2 {
		t.Fatal("same seed + label should produce the same hash")
	}

	h3 := DeriveHash(seed, "standard")
	if h1 == h3 {
		t.Fatal("different labels should produce different hashes")
	}

	if h1.IsZero() {
		t.Fatal("derived hash should not be zero")
	}
}

func TestDeriveHashNotSameAsSha256(t *testing.T) {
	seed := Sha256([]byte("test"))
	label := "label"

	derived := DeriveHash(seed, label)
	plain := Sha256(append(seed[:], []byte(label)...))

	if derived == plain {
		t.Fatal("HMAC-based derivation should differ from plain SHA-256 concatenation")
	}
}

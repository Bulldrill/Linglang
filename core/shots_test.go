package core

import (
	"math"
	"math/rand"
	"testing"
)

func TestShotsConvergesToBornRule(t *testing.T) {
	q := NewHilbertSpace("Q", 2)
	// |ψ⟩ with P(0)=0.09, P(1)=0.91
	psi := NewQuantumState(q, []complex128{0.3, complex(math.Sqrt(0.91), 0)})

	rng := rand.New(rand.NewSource(42))
	counts := psi.Shots(100000, rng)

	total := counts[0] + counts[1]
	if total != 100000 {
		t.Fatalf("expected 100000 total samples, got %d", total)
	}
	p0 := float64(counts[0]) / 100000
	if math.Abs(p0-0.09) > 0.01 {
		t.Fatalf("empirical P(0)=%.4f too far from Born-rule 0.09 (100k shots)", p0)
	}
}

func TestShotsOnlyHitsNonZeroAmplitudes(t *testing.T) {
	q := NewHilbertSpace("Q", 2)
	ket1 := NewQuantumState(q, []complex128{0, 1})

	rng := rand.New(rand.NewSource(1))
	counts := ket1.Shots(1000, rng)

	if counts[0] != 0 {
		t.Fatalf("|1> should never sample outcome 0, got count=%d", counts[0])
	}
	if counts[1] != 1000 {
		t.Fatalf("expected all 1000 shots to land on outcome 1, got %d", counts[1])
	}
}

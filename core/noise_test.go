package core

import (
	"math"
	"math/rand"
	"testing"
)

func TestDecoherenceGammaZeroT1IsNoiseless(t *testing.T) {
	if g := DecoherenceGamma(0, 1e-6); g != 0 {
		t.Fatalf("expected gamma=0 for T=0 (noiseless), got %v", g)
	}
}

func TestDecoherenceGammaIncreasesWithTime(t *testing.T) {
	t1 := 100e-6
	g1 := DecoherenceGamma(t1, 10e-6)
	g2 := DecoherenceGamma(t1, 200e-6)
	if !(0 < g1 && g1 < g2 && g2 < 1) {
		t.Fatalf("expected 0 < gamma(short) < gamma(long) < 1, got %v, %v", g1, g2)
	}
}

func TestAmplitudeDampingDecaysExcitedPopulation(t *testing.T) {
	q := NewHilbertSpace("Q", 2)
	ket1 := NewQuantumState(q, []complex128{0, 1}) // |1>
	rho := ket1.ToDensityMatrix()

	damped := AmplitudeDamping(rho, 0.3)
	// P(1) should shrink from 1.0 toward 0 by exactly (1-gamma).
	if got, want := real(damped.Matrix[1][1]), 0.7; math.Abs(got-want) > 1e-9 {
		t.Fatalf("P(1) after damping = %v, want %v", got, want)
	}
	if got, want := real(damped.Matrix[0][0]), 0.3; math.Abs(got-want) > 1e-9 {
		t.Fatalf("P(0) after damping = %v, want %v (population moved from |1> to |0>)", got, want)
	}
}

func TestAmplitudeDampingFullyDecaysToGroundState(t *testing.T) {
	q := NewHilbertSpace("Q", 2)
	ket1 := NewQuantumState(q, []complex128{0, 1})
	rho := ket1.ToDensityMatrix()

	damped := AmplitudeDamping(rho, 1.0) // gamma=1: full relaxation
	if math.Abs(real(damped.Matrix[0][0])-1.0) > 1e-9 {
		t.Fatalf("expected full decay to |0>, got rho[0][0]=%v", damped.Matrix[0][0])
	}
}

func TestPhaseDampingPreservesPopulationsDecaysCoherence(t *testing.T) {
	q := NewHilbertSpace("Q", 2)
	inv := complex(1/math.Sqrt2, 0)
	plus := NewQuantumState(q, []complex128{inv, inv}) // |+>
	rho := plus.ToDensityMatrix()

	damped := PhaseDamping(rho, 0.5)
	// Populations (diagonal) must be untouched by pure dephasing.
	if math.Abs(real(damped.Matrix[0][0])-0.5) > 1e-9 || math.Abs(real(damped.Matrix[1][1])-0.5) > 1e-9 {
		t.Fatalf("phase damping should not change populations, got diag=[%v, %v]",
			damped.Matrix[0][0], damped.Matrix[1][1])
	}
	// Off-diagonal (coherence) must shrink.
	if cmplxAbs(damped.Matrix[0][1]) >= cmplxAbs(rho.Matrix[0][1]) {
		t.Fatalf("expected coherence to shrink: before=%v after=%v", rho.Matrix[0][1], damped.Matrix[0][1])
	}
}

func cmplxAbs(c complex128) float64 {
	r, i := real(c), imag(c)
	return math.Sqrt(r*r + i*i)
}

func TestApplyReadoutErrorFlipsStatistically(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	flips := 0
	const n = 10000
	for i := 0; i < n; i++ {
		if ApplyReadoutError(0, 1, 0.2, rng) == 1 {
			flips++
		}
	}
	rate := float64(flips) / n
	if math.Abs(rate-0.2) > 0.02 {
		t.Fatalf("empirical flip rate %.4f too far from configured 0.2 (%d samples)", rate, n)
	}
}

func TestApplyReadoutErrorZeroProbNeverFlips(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 1000; i++ {
		if ApplyReadoutError(1, 1, 0, rng) != 1 {
			t.Fatal("expected prob=0 to never flip the outcome")
		}
	}
}

func TestNoisyBackendMeasureAppliesDecoherenceAndReadoutError(t *testing.T) {
	q := NewHilbertSpace("Q", 2)
	ket1 := NewQuantumState(q, []complex128{0, 1})

	// Large GateTime relative to T1/T2 so decoherence is pronounced;
	// large ReadoutError so the flip is observable statistically.
	b := NewNoisyBackend(NoiseModel{T1: 1e-6, T2: 1e-6, ReadoutError: 0.3}, 1e-6)

	zeros := 0
	const n = 5000
	for i := 0; i < n; i++ {
		outcome, probs, err := b.Measure(ket1)
		if err != nil {
			t.Fatalf("Measure error: %v", err)
		}
		if len(probs) != 2 {
			t.Fatalf("expected 2 probabilities, got %d", len(probs))
		}
		if outcome == 0 {
			zeros++
		}
	}
	// Starting from a pure |1>, decoherence + 30% readout error should
	// produce a non-trivial fraction of 0-outcomes, but not none and not
	// (close to) all — i.e. noise is actually having an effect.
	rate := float64(zeros) / n
	if rate < 0.05 || rate > 0.95 {
		t.Fatalf("expected a non-degenerate mix of outcomes from noise, got P(0)=%.3f", rate)
	}
}

func TestNoisyBackendRejectsMultiQubitState(t *testing.T) {
	reg2 := NewHilbertSpace("Reg2", 4)
	state := NewQuantumState(reg2, []complex128{1, 0, 0, 0})
	b := NewNoisyBackend(NoNoise(), 0)
	if _, _, err := b.Measure(state); err == nil {
		t.Fatal("expected an error measuring a multi-qubit state on NoisyBackend")
	}
}

func TestOpenQuantumBackendNoisy(t *testing.T) {
	backend, err := OpenQuantumBackend("noisy")
	if err != nil {
		t.Fatalf("OpenQuantumBackend(noisy) error: %v", err)
	}
	if backend.Name() != "noisy-simulator" {
		t.Fatalf("expected backend name 'noisy-simulator', got %q", backend.Name())
	}
	if backend.NoiseModel() == NoNoise() {
		t.Fatal("expected the noisy backend to report a non-trivial NoiseModel")
	}
}

package core

import (
	"math"
	"math/rand"
)

// DecoherenceGamma converts a relaxation/dephasing time constant T
// (seconds) and an elapsed time t (seconds) into the Kraus-channel damping
// probability for that interval: γ = 1 - e^{-t/T}. T == 0 is the
// NoiseModel convention for "noiseless" (NoNoise()), so it maps to γ = 0
// rather than dividing by zero.
func DecoherenceGamma(t, elapsed float64) float64 {
	if t <= 0 {
		return 0
	}
	return 1 - math.Exp(-elapsed/t)
}

// AmplitudeDamping applies the single-qubit amplitude-damping channel
// (T1 relaxation, issue #11) to rho, with damping probability gamma —
// physically, the probability the qubit decayed from |1⟩ to |0⟩ over the
// elapsed time. Kraus operators:
//
//	K0 = [[1, 0], [0, √(1-γ)]],  K1 = [[0, √γ], [0, 0]]
//	ρ' = K0 ρ K0† + K1 ρ K1†
//
// Scoped to a single qubit (dim=2), matching how T1/T2 are defined as
// per-qubit physical constants.
func AmplitudeDamping(rho *DensityMatrix, gamma float64) *DensityMatrix {
	if rho.Space.Dim != 2 {
		panic("AmplitudeDamping: solo definido para un qubit (dim=2)")
	}
	a, b := rho.Matrix[0][0], rho.Matrix[0][1]
	c, d := rho.Matrix[1][0], rho.Matrix[1][1]
	sq := complex(math.Sqrt(1-gamma), 0)

	out := [][]complex128{
		{a + complex(gamma, 0)*d, b * sq},
		{c * sq, d * complex(1-gamma, 0)},
	}
	return &DensityMatrix{Space: rho.Space, Matrix: out}
}

// PhaseDamping applies the single-qubit phase-damping channel (T2
// dephasing, issue #11) to rho, with dephasing probability lambda. Unlike
// amplitude damping, populations (diagonal) are unchanged — only
// coherence (off-diagonal terms) decays:
//
//	K0 = [[1, 0], [0, √(1-λ)]],  K1 = [[0, 0], [0, √λ]]
//	ρ' = K0 ρ K0† + K1 ρ K1†  ⟹  off-diagonal *= √(1-λ)
func PhaseDamping(rho *DensityMatrix, lambda float64) *DensityMatrix {
	if rho.Space.Dim != 2 {
		panic("PhaseDamping: solo definido para un qubit (dim=2)")
	}
	factor := complex(math.Sqrt(1-lambda), 0)
	out := [][]complex128{
		{rho.Matrix[0][0], rho.Matrix[0][1] * factor},
		{rho.Matrix[1][0] * factor, rho.Matrix[1][1]},
	}
	return &DensityMatrix{Space: rho.Space, Matrix: out}
}

// ApplyReadoutError flips each of a numQubits-wide classical outcome's
// bits independently with probability prob — the standard readout-error
// model (NoiseModel.ReadoutError, issue #12): each qubit's classical
// readout is wrong with the same independent probability, regardless of
// what value the qubit was actually in.
func ApplyReadoutError(outcome, numQubits int, prob float64, rng *rand.Rand) int {
	result := outcome
	for i := 0; i < numQubits; i++ {
		if rng.Float64() < prob {
			result ^= 1 << i
		}
	}
	return result
}

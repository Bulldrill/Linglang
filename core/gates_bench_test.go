package core

import (
	"math/rand"
	"testing"
)

// benchRegister builds a 2^n-dimensional HilbertSpace, an n-qubit H⊗H⊗...
// gate via repeated KronGate, and the |0...0⟩ ket to apply it to —
// exercising the O(4^n) matrix-vector product Gate.Apply performs.
func benchRegister(nQubits int) (*Gate, *QuantumState) {
	qubit := NewHilbertSpace("Q", 2)
	h := BuiltinH(qubit)

	gate := h
	space := qubit
	for i := 1; i < nQubits; i++ {
		newSpace := NewHilbertSpace("Reg", space.Dim*2)
		gate = KronGate(gate, h, newSpace)
		space = newSpace
	}

	amps := make([]complex128, space.Dim)
	amps[0] = 1
	return gate, NewQuantumState(space, amps)
}

func BenchmarkGateApply(b *testing.B) {
	for _, n := range []int{2, 4, 6, 8} {
		gate, state := benchRegister(n)
		b.Run(itoa(n)+"qubits", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				gate.Apply(state)
			}
		})
	}
}

func BenchmarkQuantumStateMeasure(b *testing.B) {
	for _, n := range []int{2, 4, 6, 8} {
		_, state := benchRegister(n)
		result := BuiltinI(state.Space).Apply(state) // force a realized post-Hadamard state
		b.Run(itoa(n)+"qubits", func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				result.Measure()
			}
		})
	}
}

func BenchmarkQuantumStateShots(b *testing.B) {
	_, state := benchRegister(6)
	rng := rand.New(rand.NewSource(1))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		state.Shots(1000, rng)
	}
}

func BenchmarkOptimizeCircuit(b *testing.B) {
	q := NewHilbertSpace("Q", 2)
	h := BuiltinH(q)
	x := BuiltinX(q)

	circuit := NewQIRCircuit(1)
	for i := 0; i < 200; i++ {
		if i%2 == 0 {
			circuit.AddGate(h, 0)
		} else {
			circuit.AddGate(x, 0)
		}
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		OptimizeCircuit(circuit)
	}
}

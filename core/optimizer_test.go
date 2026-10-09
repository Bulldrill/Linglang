package core

import (
	"math/cmplx"
	"testing"
)

func TestOptimizeCircuitCancelsHH(t *testing.T) {
	q := NewHilbertSpace("Q", 2)
	h := BuiltinH(q)

	c := NewQIRCircuit(1)
	c.AddGate(h, 0)
	c.AddGate(h, 0)

	opt := OptimizeCircuit(c)
	if len(opt.Nodes) != 0 {
		t.Fatalf("H·H should cancel to nothing, got %d nodes", len(opt.Nodes))
	}
}

func TestOptimizeCircuitDropsIdentity(t *testing.T) {
	q := NewHilbertSpace("Q", 2)
	c := NewQIRCircuit(1)
	c.AddGate(BuiltinI(q), 0)

	opt := OptimizeCircuit(c)
	if len(opt.Nodes) != 0 {
		t.Fatalf("I should be dropped, got %d nodes", len(opt.Nodes))
	}
}

func TestOptimizeCircuitFusesNonCancellingGates(t *testing.T) {
	q := NewHilbertSpace("Q", 2)
	c := NewQIRCircuit(1)
	c.AddGate(BuiltinH(q), 0)
	c.AddGate(BuiltinX(q), 0)

	opt := OptimizeCircuit(c)
	if len(opt.Nodes) != 1 {
		t.Fatalf("H then X on the same qubit should fuse into 1 node, got %d", len(opt.Nodes))
	}

	// Equivalence check: run both circuits from |0> and compare amplitudes.
	backend := NewSimulatorBackend()
	ket0 := NewQuantumState(q, []complex128{1, 0})

	original := NewQIRCircuit(1)
	original.AddGate(BuiltinH(q), 0)
	original.AddGate(BuiltinX(q), 0)

	wantState, err := original.Execute(backend, ket0)
	if err != nil {
		t.Fatalf("original.Execute error: %v", err)
	}
	gotState, err := opt.Execute(backend, ket0)
	if err != nil {
		t.Fatalf("opt.Execute error: %v", err)
	}
	for i := range wantState.Amplitudes {
		if cmplx.Abs(wantState.Amplitudes[i]-gotState.Amplitudes[i]) > 1e-9 {
			t.Fatalf("optimized circuit diverges: want %v got %v", wantState.Amplitudes, gotState.Amplitudes)
		}
	}
}

func TestOptimizeCircuitPreservesIndependentQubits(t *testing.T) {
	q := NewHilbertSpace("Q", 2)
	c := NewQIRCircuit(2)
	c.AddGate(BuiltinH(q), 0)
	c.AddGate(BuiltinX(q), 1) // different qubit: must not fuse with qubit 0's H

	opt := OptimizeCircuit(c)
	if len(opt.Nodes) != 2 {
		t.Fatalf("gates on disjoint qubits must not fuse, got %d nodes", len(opt.Nodes))
	}
}

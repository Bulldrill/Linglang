package core

import "testing"

func TestQIRDependencyWiring(t *testing.T) {
	q := NewHilbertSpace("Q", 2)
	h := BuiltinH(q)
	x := BuiltinX(q)

	c := NewQIRCircuit(2)
	n0 := c.AddGate(h, 0) // acts on qubit 0
	n1 := c.AddGate(x, 1) // acts on qubit 1, independent of n0
	n2 := c.AddGate(h, 0) // depends on n0 (same qubit)

	if len(n0.Deps) != 0 {
		t.Fatalf("n0 should have no deps, got %v", n0.Deps)
	}
	if len(n1.Deps) != 0 {
		t.Fatalf("n1 should have no deps (disjoint qubit), got %v", n1.Deps)
	}
	if len(n2.Deps) != 1 || n2.Deps[0] != n0 {
		t.Fatalf("n2 should depend on n0, got %v", n2.Deps)
	}

	if d := c.Depth(); d != 2 {
		t.Fatalf("expected depth 2 (n0->n2 chain, n1 parallel), got %d", d)
	}
}

func TestQIRExecute(t *testing.T) {
	q := NewHilbertSpace("Q", 2)
	x := BuiltinX(q)

	c := NewQIRCircuit(1)
	c.AddGate(x, 0)

	backend := NewSimulatorBackend()
	ket0 := NewQuantumState(q, []complex128{1, 0})
	result, err := c.Execute(backend, ket0)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	probs := result.Probabilities()
	if probs[0] > 1e-9 || probs[1] < 0.999999 {
		t.Fatalf("X|0> should give |1>, got probs=%v", probs)
	}
}

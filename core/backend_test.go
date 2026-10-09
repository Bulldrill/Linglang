package core

import "testing"

func TestSimulatorBackendAppliesGates(t *testing.T) {
	q := NewHilbertSpace("Q", 2)
	backend := NewSimulatorBackend()
	ket0 := NewQuantumState(q, []complex128{1, 0})

	result, err := backend.Apply(BuiltinX(q), ket0)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}
	probs := result.Probabilities()
	if probs[1] < 0.999999 {
		t.Fatalf("X|0> should give |1>, got probs=%v", probs)
	}

	outcome, probs, err := backend.Measure(result)
	if err != nil {
		t.Fatalf("Measure error: %v", err)
	}
	if outcome != 1 {
		t.Fatalf("expected outcome 1, got %d (probs=%v)", outcome, probs)
	}
}

func TestSimulatorBackendIsFullyConnectedAndNoiseless(t *testing.T) {
	backend := NewSimulatorBackend()
	topo := backend.Topology()
	if !topo.Connected(0, 5) {
		t.Fatal("simulator topology should report any two qubits as connected")
	}
	noise := backend.NoiseModel()
	if noise != NoNoise() {
		t.Fatalf("simulator should be noiseless, got %+v", noise)
	}
}

func TestSimulatorBackendRejectsDimensionMismatch(t *testing.T) {
	q2 := NewHilbertSpace("Q2", 4)
	qreg := NewHilbertSpace("QReg", 2)
	backend := NewSimulatorBackend()
	mismatched := NewQuantumState(qreg, []complex128{1, 0})
	if _, err := backend.Apply(BuiltinI(q2), mismatched); err == nil {
		t.Fatal("expected error applying a dim-4 gate to a dim-2 state")
	}
}

package core

import (
	"math"
	"math/cmplx"
	"testing"
)

func TestTeleportRecoversState(t *testing.T) {
	qubit := NewHilbertSpace("Q", 2)
	inv := complex(1/math.Sqrt2, 0)
	cases := map[string]*QuantumState{
		"|0>":        NewQuantumState(qubit, []complex128{1, 0}),
		"|1>":        NewQuantumState(qubit, []complex128{0, 1}),
		"|+>":        NewQuantumState(qubit, []complex128{inv, inv}),
		"asymmetric": NewQuantumState(qubit, []complex128{0.6, 0.8}),
	}
	for label, psi := range cases {
		ch := NewLocalChannel("alice", "bob", qubit)
		result, err := Teleport(psi, ch)
		if err != nil {
			t.Fatalf("%s: Teleport error: %v", label, err)
		}
		for i := range psi.Amplitudes {
			if cmplx.Abs(result.State.Amplitudes[i]-psi.Amplitudes[i]) > 1e-9 {
				t.Fatalf("%s: teleport mismatch at amplitude %d: got %v want %v (m0=%d m1=%d)",
					label, i, result.State.Amplitudes, psi.Amplitudes, result.M0, result.M1)
			}
		}
	}
}

func TestTeleportRejectsNonQubit(t *testing.T) {
	qreg := NewHilbertSpace("QReg2", 4)
	psi := NewQuantumState(qreg, []complex128{1, 0, 0, 0})
	ch := NewLocalChannel("alice", "bob", NewHilbertSpace("Q", 2))
	if _, err := Teleport(psi, ch); err == nil {
		t.Fatal("expected error teleporting a non-qubit state, got nil")
	}
}

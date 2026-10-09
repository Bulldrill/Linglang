package core

import "testing"

func BenchmarkTeleport(b *testing.B) {
	qubit := NewHilbertSpace("Q", 2)
	psi := NewQuantumState(qubit, []complex128{0.6, 0.8})

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ch := NewLocalChannel("alice", "bob", qubit)
		if _, err := Teleport(psi, ch); err != nil {
			b.Fatalf("Teleport error: %v", err)
		}
	}
}

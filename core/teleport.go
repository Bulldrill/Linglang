package core

import "fmt"

// TeleportResult is the outcome of a Teleport call: Bob's recovered state
// together with Alice's classical measurement bits.
type TeleportResult struct {
	State *QuantumState // Bob's qubit after the X/Z correction — equal to the teleported ψ
	M0    int           // Alice's phase-correction bit
	M1    int           // Alice's bit-flip-correction bit
}

// Teleport migrates the single-qubit state psi from Alice to Bob over ch,
// without ever transmitting psi's physical qubit. This is the quantum
// teleportation protocol first demonstrated manually, gate by gate, in
// examples/teleportacion.lin — elevated here into a runtime primitive for
// the distributed-qubits runtime, formally documented in
// docs/teleportation.md.
//
// Protocol:
//  1. ch.ShareBellPair() entangles a fresh qubit pair, split between Alice
//     (ch.NodeA()) and Bob (ch.NodeB()).
//  2. Alice entangles psi with her half via CNOT, then applies a Hadamard
//     to psi.
//  3. Alice measures both her qubits, producing two classical bits
//     (m0, m1), which she sends to Bob over ch's classical channel.
//  4. Bob applies X^m1·Z^m0 to his half, recovering psi exactly — he never
//     received psi's physical qubit, only two classical bits.
//
// Measurement uses QuantumState.Measure's deterministic argmax, matching
// the rest of the runtime's current (non-stochastic) semantics; a
// shot-based Measure (tracked separately) would make outcomes probabilistic
// without changing this protocol.
func Teleport(psi *QuantumState, ch QuantumChannel) (*TeleportResult, error) {
	if psi.Space.Dim != 2 {
		return nil, fmt.Errorf("teleport: solo se pueden teleportar qubits (dim=2), dim=%d", psi.Space.Dim)
	}

	pair, err := ch.ShareBellPair()
	if err != nil {
		return nil, fmt.Errorf("teleport: %w", err)
	}
	if pair.Space.Dim != 4 {
		return nil, fmt.Errorf("teleport: par de Bell con dimensión inesperada %d", pair.Space.Dim)
	}

	qubit := NewHilbertSpace("Qubit", 2)
	reg2 := NewHilbertSpace("Reg2", 4)
	reg3 := NewHilbertSpace("Reg3", 8)

	h := BuiltinH(qubit)
	i2 := BuiltinI(qubit)
	x := BuiltinX(qubit)
	z := BuiltinZ(qubit)
	cnot := BuiltinCNOT(reg2)

	// |ψ, Φ+⟩ — psi tensored with the already-entangled Alice/Bob pair.
	state := psi.Tensor(pair.State, reg3)

	// Alice: CNOT(q0 controls q1), then H on q0.
	cnot01I := KronGate(cnot, i2, reg3)
	state = cnot01I.Apply(state)
	hII := KronGate(h, KronGate(i2, i2, reg2), reg3)
	state = hII.Apply(state)

	// Alice measures q0,q1. Basis order is |q0 q1 q2⟩, so bit 2 is q0 and
	// bit 1 is q2's sibling... concretely: index = m0*4 + m1*2 + q2.
	outcome, _ := state.Measure()
	m0 := (outcome >> 2) & 1
	m1 := (outcome >> 1) & 1

	if err := ch.SendClassical(ch.NodeA(), []int{m0, m1}); err != nil {
		return nil, fmt.Errorf("teleport: %w", err)
	}
	bits, err := ch.RecvClassical(ch.NodeB())
	if err != nil {
		return nil, fmt.Errorf("teleport: %w", err)
	}
	m0, m1 = bits[0], bits[1]

	// Collapse to Bob's (q2) amplitudes consistent with Alice's outcome,
	// then renormalise — this is the projective measurement's effect on
	// the remaining qubit.
	base := m0*4 + m1*2
	bobState := NewQuantumState(qubit, []complex128{
		state.Amplitudes[base],
		state.Amplitudes[base+1],
	}).Normalize()

	// Bob applies X^m1 · Z^m0 to recover ψ exactly.
	if m0 == 1 {
		bobState = z.Apply(bobState)
	}
	if m1 == 1 {
		bobState = x.Apply(bobState)
	}

	return &TeleportResult{State: bobState, M0: m0, M1: m1}, nil
}

package core

import (
	"math"
	"math/cmplx"
	"strings"
)

// pendingFusionGroup is an open run of adjacent, same-qubit gates that
// OptimizeCircuit is accumulating via matrix composition before deciding
// whether to fuse, cancel, or flush them.
type pendingFusionGroup struct {
	gate   *Gate
	qubits []int
}

// OptimizeCircuit returns a new QIRCircuit equivalent to circuit but with
// hardware-independent algebraic simplifications applied — exactly the use
// case the QIR DAG (core/qir.go, issue #7) exists to enable:
//
//  1. Identity removal: any "I" gate is dropped (I|ψ⟩ = |ψ⟩).
//  2. Self-cancelling pairs: if two adjacent gates on the same qubits
//     compose to a global-phase multiple of the identity — U₂U₁ = e^{iθ}I —
//     both are dropped. This subsumes H²=I, X²=I, Y²=I, Z²=I without
//     special-casing any gate by name: it falls out of unitarity (U U† = I)
//     and self-inverse gates (U² = I) alike.
//  3. Fusion: adjacent, non-cancelling gates on the same qubits are merged
//     into a single gate via matrix multiplication, reducing gate count —
//     fewer operations to schedule, route, and (on real hardware) suffer
//     decoherence during.
func OptimizeCircuit(circuit *QIRCircuit) *QIRCircuit {
	out := NewQIRCircuit(circuit.NumQubits)
	byQubit := map[int]*pendingFusionGroup{}

	flush := func(qubits []int) {
		flushed := map[*pendingFusionGroup]bool{}
		for _, q := range qubits {
			if p := byQubit[q]; p != nil && !flushed[p] {
				flushed[p] = true
				out.AddGate(p.gate, p.qubits...)
			}
		}
		for _, q := range qubits {
			delete(byQubit, q)
		}
	}

	for _, n := range circuit.TopoOrder() {
		if strings.EqualFold(n.Gate.Name, "I") {
			continue
		}

		if p := matchingGroup(byQubit, n.Qubits); p != nil {
			fused := composeGates(p.gate, n.Gate)
			for _, q := range n.Qubits {
				delete(byQubit, q)
			}
			if isGlobalPhaseIdentity(fused.Matrix) {
				continue // self-cancelling pair: drop both, emit nothing
			}
			np := &pendingFusionGroup{gate: fused, qubits: n.Qubits}
			for _, q := range n.Qubits {
				byQubit[q] = np
			}
			continue
		}

		flush(n.Qubits)
		p := &pendingFusionGroup{gate: n.Gate, qubits: n.Qubits}
		for _, q := range n.Qubits {
			byQubit[q] = p
		}
	}

	seen := map[*pendingFusionGroup]bool{}
	for _, p := range byQubit {
		if !seen[p] {
			seen[p] = true
			out.AddGate(p.gate, p.qubits...)
		}
	}
	return out
}

// matchingGroup returns the single pending group that already covers
// exactly qubits (same set, same size), or nil if there is no such group —
// e.g. because some of qubits are untouched, or because they are split
// across different/partial groups, which fusion cannot soundly handle.
func matchingGroup(byQubit map[int]*pendingFusionGroup, qubits []int) *pendingFusionGroup {
	p := byQubit[qubits[0]]
	if p == nil || len(p.qubits) != len(qubits) {
		return nil
	}
	for _, q := range qubits {
		if byQubit[q] != p {
			return nil
		}
	}
	return p
}

// composeGates returns the gate representing "apply g1, then g2" on the
// same qubits: matrix = g2 · g1 (operators compose right-to-left against
// a column vector, matching Gate.Apply's convention).
func composeGates(g1, g2 *Gate) *Gate {
	n := len(g1.Matrix)
	m := make([][]complex128, n)
	for i := 0; i < n; i++ {
		m[i] = make([]complex128, n)
		for j := 0; j < n; j++ {
			var sum complex128
			for k := 0; k < n; k++ {
				sum += g2.Matrix[i][k] * g1.Matrix[k][j]
			}
			m[i][j] = sum
		}
	}
	return NewGate(g1.Name+"·"+g2.Name, g1.Domain, g2.Codomain, m)
}

// isGlobalPhaseIdentity reports whether m equals e^{iθ}·I within tolerance:
// an operator with no observable effect beyond an unmeasurable global
// phase, so a gate pair composing to this is safe to drop entirely.
func isGlobalPhaseIdentity(m [][]complex128) bool {
	const tol = 1e-9
	n := len(m)

	var phase complex128
	found := false
	for i := 0; i < n; i++ {
		if cmplx.Abs(m[i][i]) > tol {
			phase = m[i][i]
			found = true
			break
		}
	}
	if !found || math.Abs(cmplx.Abs(phase)-1) > tol {
		return false
	}
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			expected := complex(0, 0)
			if i == j {
				expected = phase
			}
			if cmplx.Abs(m[i][j]-expected) > tol {
				return false
			}
		}
	}
	return true
}

package core

import (
	"fmt"
	"math"
	"math/cmplx"
)

// ── Gate ─────────────────────────────────────────────────────────────────────

// Gate represents a quantum gate as a unitary matrix U acting on a HilbertSpace.
// Unitarity condition: U†U = I  (verified by IsUnitary within tolerance 1e-9).
type Gate struct {
	Name     string
	Domain   *HilbertSpace
	Codomain *HilbertSpace
	Matrix   [][]complex128 // row-major: Matrix[row][col]
}

func NewGate(name string, domain, codomain *HilbertSpace, matrix [][]complex128) *Gate {
	return &Gate{
		Name:     name,
		Domain:   domain,
		Codomain: codomain,
		Matrix:   matrix,
	}
}

// Apply computes U|ψ⟩ via matrix-vector product over ℂ.
func (g *Gate) Apply(state *QuantumState) *QuantumState {
	n := len(g.Matrix)
	result := make([]complex128, n)
	for i := 0; i < n; i++ {
		for j := 0; j < len(state.Amplitudes); j++ {
			result[i] += g.Matrix[i][j] * state.Amplitudes[j]
		}
	}
	return NewQuantumState(g.Codomain, result)
}

// IsUnitary verifies U†U = I within absolute tolerance tol = 1e-9.
// (U†)ᵢⱼ = conj(Uⱼᵢ), so (U†U)ᵢⱼ = Σₖ conj(Uₖᵢ)·Uₖⱼ
func (g *Gate) IsUnitary() bool {
	n := len(g.Matrix)
	const tol = 1e-9
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			sum := complex(0.0, 0.0)
			for k := 0; k < n; k++ {
				sum += cmplx.Conj(g.Matrix[k][i]) * g.Matrix[k][j]
			}
			expected := complex(0.0, 0.0)
			if i == j {
				expected = complex(1.0, 0.0)
			}
			if cmplx.Abs(sum-expected) > tol {
				return false
			}
		}
	}
	return true
}

func (g *Gate) String() string {
	return fmt.Sprintf("Gate(%s, %d×%d, unitary=%v)",
		g.Name, len(g.Matrix), len(g.Matrix[0]), g.IsUnitary())
}

// ── Built-in quantum gates ────────────────────────────────────────────────────

var _invsqrt2 = complex(1.0/math.Sqrt(2), 0) // 1/√2

// BuiltinH returns the Hadamard gate for the given HilbertSpace (dim=2).
//
//	H = 1/√2 · [[1, 1], [1, -1]]
//	H|0⟩ = |+⟩,  H|1⟩ = |−⟩
func BuiltinH(h *HilbertSpace) *Gate {
	c := _invsqrt2
	return NewGate("H", h, h, [][]complex128{
		{c, c},
		{c, -c},
	})
}

// BuiltinX returns the Pauli-X (bit-flip / NOT) gate.
//
//	X = [[0, 1], [1, 0]]
//	X|0⟩ = |1⟩,  X|1⟩ = |0⟩
func BuiltinX(h *HilbertSpace) *Gate {
	return NewGate("X", h, h, [][]complex128{
		{0, 1},
		{1, 0},
	})
}

// BuiltinY returns the Pauli-Y gate.
//
//	Y = [[0, -i], [i, 0]]
func BuiltinY(h *HilbertSpace) *Gate {
	return NewGate("Y", h, h, [][]complex128{
		{0, complex(0, -1)},
		{complex(0, 1), 0},
	})
}

// BuiltinZ returns the Pauli-Z (phase-flip) gate.
//
//	Z = [[1, 0], [0, -1]]
//	Z|0⟩ = |0⟩,  Z|1⟩ = -|1⟩
func BuiltinZ(h *HilbertSpace) *Gate {
	return NewGate("Z", h, h, [][]complex128{
		{1, 0},
		{0, -1},
	})
}

// BuiltinCNOT returns the controlled-NOT gate on a 2-qubit (dim=4) system.
// Basis order: |00⟩, |01⟩, |10⟩, |11⟩
//
//	CNOT = [[1,0,0,0],[0,1,0,0],[0,0,0,1],[0,0,1,0]]
func BuiltinCNOT(h *HilbertSpace) *Gate {
	return NewGate("CNOT", h, h, [][]complex128{
		{1, 0, 0, 0},
		{0, 1, 0, 0},
		{0, 0, 0, 1},
		{0, 0, 1, 0},
	})
}

// BuiltinSWAP returns the SWAP gate on a 2-qubit (dim=4) system: exchanges
// the states of the two qubits. Basis order: |00⟩, |01⟩, |10⟩, |11⟩.
//
//	SWAP = [[1,0,0,0],[0,0,1,0],[0,1,0,0],[0,0,0,1]]
func BuiltinSWAP(h *HilbertSpace) *Gate {
	return NewGate("SWAP", h, h, [][]complex128{
		{1, 0, 0, 0},
		{0, 0, 1, 0},
		{0, 1, 0, 0},
		{0, 0, 0, 1},
	})
}

// BuiltinI returns the identity gate for any dimension.
func BuiltinI(h *HilbertSpace) *Gate {
	n := h.Dim
	matrix := make([][]complex128, n)
	for i := range matrix {
		matrix[i] = make([]complex128, n)
		matrix[i][i] = 1
	}
	return NewGate("I", h, h, matrix)
}

// KronGate returns the Kronecker (tensor) product G1 ⊗ G2 acting on the
// given product HilbertSpace.  The matrix satisfies:
//
//	(G1⊗G2)_{i·n2+k, j·n2+l} = G1_{ij} · G2_{kl}
func KronGate(g1, g2 *Gate, space *HilbertSpace) *Gate {
	n1 := len(g1.Matrix)
	n2 := len(g2.Matrix)
	n := n1 * n2
	matrix := make([][]complex128, n)
	for i := range matrix {
		matrix[i] = make([]complex128, n)
		for j := range matrix[i] {
			i1, k := i/n2, i%n2
			j1, l := j/n2, j%n2
			matrix[i][j] = g1.Matrix[i1][j1] * g2.Matrix[k][l]
		}
	}
	return NewGate(g1.Name+"⊗"+g2.Name, space, space, matrix)
}

package core

import (
	"fmt"
	"math"
	"math/cmplx"
)

// ── Hilbert Space ─────────────────────────────────────────────────────────────

// HilbertSpace represents an n-dimensional complex Hilbert space H^n.
// In the LinLang paradigm, a HilbertSpace is a specialised Space over ℂ
// rather than ℝ, enabling quantum-mechanical modelling.
type HilbertSpace struct {
	Name string
	Dim  int
}

func NewHilbertSpace(name string, dim int) *HilbertSpace {
	return &HilbertSpace{Name: name, Dim: dim}
}

func (h *HilbertSpace) String() string {
	return fmt.Sprintf("Hilbert(%s, dim=%d)", h.Name, h.Dim)
}

// ── Quantum State |ψ⟩ ─────────────────────────────────────────────────────────

// QuantumState represents a pure quantum state |ψ⟩ as a complex amplitude
// vector in a HilbertSpace.  For an n-dimensional space the state is:
//
//	|ψ⟩ = α₀|0⟩ + α₁|1⟩ + … + αₙ₋₁|n-1⟩
type QuantumState struct {
	Space      *HilbertSpace
	Amplitudes []complex128
}

func NewQuantumState(space *HilbertSpace, amps []complex128) *QuantumState {
	if len(amps) != space.Dim {
		panic(fmt.Sprintf("ket: espacio '%s' tiene dim=%d, pero se pasaron %d amplitudes",
			space.Name, space.Dim, len(amps)))
	}
	return &QuantumState{Space: space, Amplitudes: amps}
}

// Norm returns ‖ψ‖ = √(Σ|αᵢ|²)
func (q *QuantumState) Norm() float64 {
	sum := 0.0
	for _, a := range q.Amplitudes {
		r, im := real(a), imag(a)
		sum += r*r + im*im
	}
	return math.Sqrt(sum)
}

// Normalize returns |ψ⟩/‖ψ‖ (unit vector)
func (q *QuantumState) Normalize() *QuantumState {
	n := q.Norm()
	if n == 0 {
		return q
	}
	amps := make([]complex128, len(q.Amplitudes))
	for i, a := range q.Amplitudes {
		amps[i] = a / complex(n, 0)
	}
	return NewQuantumState(q.Space, amps)
}

// Braket computes the inner product ⟨φ|ψ⟩ = Σ conj(φᵢ)·ψᵢ
// (q is the bra ⟨q|, other is the ket |other⟩)
func (q *QuantumState) Braket(other *QuantumState) complex128 {
	result := complex(0.0, 0.0)
	for i := range q.Amplitudes {
		result += cmplx.Conj(q.Amplitudes[i]) * other.Amplitudes[i]
	}
	return result
}

// Tensor returns |q⟩ ⊗ |other⟩ via Kronecker product of amplitudes.
// The result lives in the supplied product HilbertSpace.
func (q *QuantumState) Tensor(other *QuantumState, space *HilbertSpace) *QuantumState {
	amps := make([]complex128, q.Space.Dim*other.Space.Dim)
	for i, a := range q.Amplitudes {
		for j, b := range other.Amplitudes {
			amps[i*other.Space.Dim+j] = a * b
		}
	}
	return NewQuantumState(space, amps)
}

// Probabilities returns |αᵢ|² for each computational basis state (Born rule).
func (q *QuantumState) Probabilities() []float64 {
	probs := make([]float64, len(q.Amplitudes))
	for i, a := range q.Amplitudes {
		r, im := real(a), imag(a)
		probs[i] = r*r + im*im
	}
	return probs
}

// Measure collapses |ψ⟩ and returns the most probable basis index and all
// probabilities.  A full stochastic implementation would sample; here we use
// the deterministic argmax for reproducibility in tests.
func (q *QuantumState) Measure() (int, []float64) {
	probs := q.Probabilities()
	maxIdx, maxP := 0, probs[0]
	for i, p := range probs {
		if p > maxP {
			maxIdx, maxP = i, p
		}
	}
	return maxIdx, probs
}

// ToDensityMatrix returns ρ = |ψ⟩⟨ψ|  (pure-state density matrix)
func (q *QuantumState) ToDensityMatrix() *DensityMatrix {
	n := q.Space.Dim
	m := make([][]complex128, n)
	for i := range m {
		m[i] = make([]complex128, n)
		for j := range m[i] {
			m[i][j] = q.Amplitudes[i] * cmplx.Conj(q.Amplitudes[j])
		}
	}
	return &DensityMatrix{Space: q.Space, Matrix: m}
}

func (q *QuantumState) String() string {
	return fmt.Sprintf("|ψ⟩@%s  amps=%v  probs=%v",
		q.Space.Name, q.Amplitudes, q.Probabilities())
}

// ── Density Matrix ρ ──────────────────────────────────────────────────────────

// DensityMatrix represents a (possibly mixed) quantum state as
//
//	ρ = Σᵢ pᵢ |ψᵢ⟩⟨ψᵢ|
//
// For a pure state, ρ = |ψ⟩⟨ψ| and Purity = 1.
type DensityMatrix struct {
	Space  *HilbertSpace
	Matrix [][]complex128
}

// Trace returns Tr(ρ) = Σᵢ ρᵢᵢ  (should equal 1 for a valid density matrix)
func (d *DensityMatrix) Trace() float64 {
	sum := 0.0
	for i := range d.Matrix {
		sum += real(d.Matrix[i][i])
	}
	return sum
}

// Purity returns Tr(ρ²) = Σᵢⱼ |ρᵢⱼ|²
// Range: 1/n ≤ Purity ≤ 1; equals 1 iff the state is pure.
func (d *DensityMatrix) Purity() float64 {
	sum := 0.0
	for _, row := range d.Matrix {
		for _, v := range row {
			r, im := real(v), imag(v)
			sum += r*r + im*im
		}
	}
	return sum
}

// PartialTrace traces out subsystem B of dimension subDim from a bipartite
// system ρ_AB ∈ H_A ⊗ H_B, returning the reduced density matrix ρ_A.
//
//	(ρ_A)ᵢⱼ = Σₖ ρ_(i·subDim+k, j·subDim+k)
func (d *DensityMatrix) PartialTrace(subDim int) *DensityMatrix {
	totalDim := d.Space.Dim
	remainDim := totalDim / subDim
	rho := make([][]complex128, remainDim)
	for i := range rho {
		rho[i] = make([]complex128, remainDim)
		for j := range rho[i] {
			for k := 0; k < subDim; k++ {
				rho[i][j] += d.Matrix[i*subDim+k][j*subDim+k]
			}
		}
	}
	reducedSpace := NewHilbertSpace(d.Space.Name+"_A", remainDim)
	return &DensityMatrix{Space: reducedSpace, Matrix: rho}
}

// PartialTraceA traces out the FIRST subsystem (A, of dimension totalDim/keepDim)
// and returns the reduced density matrix of the SECOND subsystem (B, of dimension keepDim).
//
//	(ρ_B)_{bb'} = Σ_a ρ_{a·keepDim+b, a·keepDim+b'}
//
// Use this when the state is ordered as |A⟩⊗|B⟩ (A first, B last) and you want
// to trace over A to recover B.  In quantum teleportation, this recovers Bob's qubit
// after tracing over Alice's measurement qubits.
func (d *DensityMatrix) PartialTraceA(keepDim int) *DensityMatrix {
	totalDim := d.Space.Dim
	traceDim := totalDim / keepDim
	rho := make([][]complex128, keepDim)
	for b := range rho {
		rho[b] = make([]complex128, keepDim)
		for b2 := range rho[b] {
			for a := 0; a < traceDim; a++ {
				rho[b][b2] += d.Matrix[a*keepDim+b][a*keepDim+b2]
			}
		}
	}
	reducedSpace := NewHilbertSpace(d.Space.Name+"_B", keepDim)
	return &DensityMatrix{Space: reducedSpace, Matrix: rho}
}

func (d *DensityMatrix) String() string {
	return fmt.Sprintf("ρ@%s  [tr=%.4f, purity=%.4f]",
		d.Space.Name, d.Trace(), d.Purity())
}

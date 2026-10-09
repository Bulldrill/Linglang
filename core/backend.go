package core

import (
	"fmt"
	"os"
)

// QuantumBackend is the abstraction every quantum backend — simulator or
// real QPU — must implement. It is the quantum analogue of store.Backend:
// it decouples the LinLang/Q runtime (gates, states, measurement) from the
// concrete hardware or simulation engine that actually executes them, so a
// .lin program written against gates and kets runs unmodified against any
// backend that satisfies this interface.
type QuantumBackend interface {
	// Name identifies the backend, e.g. "simulator", "ibm-superconductor".
	Name() string

	// Apply executes gate on state and returns the resulting state.
	// A hardware backend returns an error if gate is not in SupportedGates()
	// or if gate acts on qubits that are not Topology()-connected.
	Apply(gate *Gate, state *QuantumState) (*QuantumState, error)

	// Measure performs a projective measurement on state, returning the
	// collapsed basis index and the probability distribution it was drawn
	// from.
	Measure(state *QuantumState) (outcome int, probs []float64, err error)

	// SupportedGates lists the native gate set this backend can execute
	// directly, without compilation/decomposition by the QIR layer.
	SupportedGates() []string

	// Topology describes physical qubit connectivity.
	Topology() Topology

	// NoiseModel describes the backend's error characteristics. A noiseless
	// backend (e.g. the reference simulator) returns NoNoise().
	NoiseModel() NoiseModel
}

// Topology describes physical qubit connectivity as an undirected graph.
// Edges == nil means "fully connected" — any qubit can interact with any
// other — which is the case for the ideal simulator.
type Topology struct {
	Qubits int
	Edges  [][2]int
}

// Connected reports whether qubits a and b can interact directly.
func (t Topology) Connected(a, b int) bool {
	if t.Edges == nil {
		return true
	}
	for _, e := range t.Edges {
		if (e[0] == a && e[1] == b) || (e[0] == b && e[1] == a) {
			return true
		}
	}
	return false
}

// NoiseModel describes a backend's error characteristics. The zero value is
// noiseless and is what NoNoise() returns for the reference simulator.
type NoiseModel struct {
	T1           float64 // relaxation time in seconds; 0 = no relaxation
	T2           float64 // dephasing time in seconds; 0 = no dephasing
	ReadoutError float64 // probability of a bit-flip on measurement readout
}

// NoNoise returns the noiseless NoiseModel used by SimulatorBackend.
func NoNoise() NoiseModel {
	return NoiseModel{}
}

// SimulatorBackend is the reference, noiseless QuantumBackend. It executes
// gates exactly via Gate.Apply and performs deterministic measurement via
// QuantumState.Measure — the behaviour the LinLang/Q runtime has used since
// its introduction. Every other backend (#3-#6) implements the same
// interface against real or noisy hardware.
type SimulatorBackend struct{}

func NewSimulatorBackend() *SimulatorBackend { return &SimulatorBackend{} }

func (s *SimulatorBackend) Name() string { return "simulator" }

func (s *SimulatorBackend) Apply(gate *Gate, state *QuantumState) (*QuantumState, error) {
	if gate == nil || state == nil {
		return nil, fmt.Errorf("simulator: gate y estado no pueden ser nil")
	}
	if len(gate.Matrix) != len(state.Amplitudes) {
		return nil, fmt.Errorf("simulator: dimensión de la puerta (%d) no coincide con el estado (%d)",
			len(gate.Matrix), len(state.Amplitudes))
	}
	return gate.Apply(state), nil
}

func (s *SimulatorBackend) Measure(state *QuantumState) (int, []float64, error) {
	if state == nil {
		return 0, nil, fmt.Errorf("simulator: estado no puede ser nil")
	}
	outcome, probs := state.Measure()
	return outcome, probs, nil
}

func (s *SimulatorBackend) SupportedGates() []string {
	return []string{"H", "X", "Y", "Z", "I", "CNOT"}
}

func (s *SimulatorBackend) Topology() Topology {
	return Topology{} // fully connected
}

func (s *SimulatorBackend) NoiseModel() NoiseModel {
	return NoNoise()
}

// ── Real hardware: the Apply/state-vector mismatch ──────────────────────────
//
// QuantumBackend.Apply takes an arbitrary *QuantumState — a full amplitude
// vector — as "input". That is only physically meaningful for a software
// simulator (SimulatorBackend, and the GPU-accelerated one in
// cuda_backend.go): the state lives as data in RAM, so applying a matrix to
// it is literally a matrix-vector product. A real QPU has no such
// operation: you cannot "load" an arbitrary pre-computed amplitude vector
// onto physical qubits. A real backend only ever executes a full circuit
// starting from the ground state |0...0⟩ and returns classical measurement
// outcomes (shots).
//
// ErrStatePreparationUnsupported is what SuperconductorBackend and
// TrappedIonBackend return from Apply/Measure for exactly this reason: they
// satisfy the QuantumBackend contract (so they type-check and report their
// real topology/noise/gate-set), but the *meaningful* way to run a circuit
// on them is CircuitRunner.Run, which submits the whole circuit as one job.
var ErrStatePreparationUnsupported = fmt.Errorf(
	"este backend no admite preparar un estado arbitrario vía Apply/Measure: " +
		"el hardware real solo ejecuta circuitos completos desde |0...0⟩ — use Run(circuit, shots)")

// CircuitRunner is implemented by backends that execute a whole QIRCircuit
// as a single job and return a measurement-outcome histogram, rather than
// applying one gate at a time to an in-memory state vector. This is the
// only physically meaningful execution mode for real QPUs (see above).
type CircuitRunner interface {
	QuantumBackend

	// Run submits circuit for execution with the given number of shots,
	// starting from |0...0⟩, and returns how many times each basis index
	// was observed. Σ counts == shots.
	Run(circuit *QIRCircuit, shots int) (counts map[int]int, err error)
}

// ── Backend registry ─────────────────────────────────────────────────────────

// cudaBackendFactory constructs the GPU-accelerated simulator. It is nil in
// a default build (core/cuda_backend.go is excluded by the "cuda" build
// tag) and is set by that file's init() when compiled with -tags cuda on a
// machine with the NVIDIA cuQuantum toolkit installed.
var cudaBackendFactory func() (QuantumBackend, error)

// OpenQuantumBackend constructs the QuantumBackend named by technology, the
// quantum analogue of store.Open. If technology is empty it reads
// LINLANG_QUANTUM_BACKEND; if that is also unset it defaults to the ideal
// simulator.
//
// Recognised names:
//
//	simulator               → SimulatorBackend (default, noiseless)
//	superconductor | ibm    → SuperconductorBackend (IBM Quantum, real API)
//	trapped-ion | ionq      → TrappedIonBackend (IonQ, real API)
//	gpu | cuda              → GPU-accelerated simulator (requires -tags cuda)
func OpenQuantumBackend(technology string) (QuantumBackend, error) {
	if technology == "" {
		technology = os.Getenv("LINLANG_QUANTUM_BACKEND")
	}
	switch technology {
	case "", "simulator":
		return NewSimulatorBackend(), nil
	case "superconductor", "ibm":
		return NewSuperconductorBackend()
	case "trapped-ion", "ionq":
		return NewTrappedIonBackend()
	case "gpu", "cuda":
		if cudaBackendFactory == nil {
			return nil, fmt.Errorf("technology '%s': compilado sin soporte CUDA "+
				"(requiere 'go build -tags cuda' y el toolkit NVIDIA cuQuantum instalado)", technology)
		}
		return cudaBackendFactory()
	default:
		return nil, fmt.Errorf("technology '%s' no reconocida (simulator | superconductor | trapped-ion | gpu)", technology)
	}
}

package core

import "fmt"

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

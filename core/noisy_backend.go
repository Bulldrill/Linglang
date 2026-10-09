package core

import (
	"fmt"
	"math/rand"
	"time"
)

// NoisyBackend wraps the ideal simulator with a NoiseModel (issues #11,
// #12): gates apply exactly as SimulatorBackend.Apply — this scoped
// implementation does not model gate-error channels — but Measure first
// applies T1 relaxation and T2 dephasing (core/noise.go) to the state's
// density matrix for GateTime seconds, then samples via the Born rule,
// then applies independent readout bit-flip error.
//
// Honesty note: modelling decoherence *during* circuit execution would
// require tracking per-gate timing and composing a Kraus channel after
// every gate. This implementation applies decoherence once, for a single
// user-supplied "elapsed time since preparation" (GateTime), right before
// measurement — a standard simplification for illustrating T1/T2 effects,
// not a continuous-time master-equation simulator. It is also scoped to a
// single qubit (dim=2), matching AmplitudeDamping/PhaseDamping.
type NoisyBackend struct {
	inner    *SimulatorBackend
	noise    NoiseModel
	GateTime float64 // seconds elapsed since state preparation
	rng      *rand.Rand
}

// NewNoisyBackend constructs a NoisyBackend with the given NoiseModel and
// elapsed time (seconds) to simulate decoherence over.
func NewNoisyBackend(noise NoiseModel, gateTime float64) *NoisyBackend {
	return &NoisyBackend{
		inner:    NewSimulatorBackend(),
		noise:    noise,
		GateTime: gateTime,
		rng:      rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (b *NoisyBackend) Name() string { return "noisy-simulator" }

func (b *NoisyBackend) Apply(gate *Gate, state *QuantumState) (*QuantumState, error) {
	return b.inner.Apply(gate, state)
}

func (b *NoisyBackend) Measure(state *QuantumState) (int, []float64, error) {
	if state == nil {
		return 0, nil, fmt.Errorf("noisy-simulator: estado no puede ser nil")
	}
	if state.Space.Dim != 2 {
		return 0, nil, fmt.Errorf(
			"noisy-simulator: el modelo T1/T2/readout de esta implementación solo cubre 1 qubit (dim=2), recibido dim=%d",
			state.Space.Dim)
	}

	rho := state.ToDensityMatrix()
	rho = AmplitudeDamping(rho, DecoherenceGamma(b.noise.T1, b.GateTime))
	rho = PhaseDamping(rho, DecoherenceGamma(b.noise.T2, b.GateTime))

	probs := []float64{real(rho.Matrix[0][0]), real(rho.Matrix[1][1])}
	outcome := 0
	if b.rng.Float64() < probs[1] {
		outcome = 1
	}
	outcome = ApplyReadoutError(outcome, 1, b.noise.ReadoutError, b.rng)
	return outcome, probs, nil
}

func (b *NoisyBackend) SupportedGates() []string { return b.inner.SupportedGates() }
func (b *NoisyBackend) Topology() Topology       { return b.inner.Topology() }
func (b *NoisyBackend) NoiseModel() NoiseModel   { return b.noise }

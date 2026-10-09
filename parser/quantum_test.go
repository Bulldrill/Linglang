package parser

import (
	"math"
	"testing"
)

func TestHilbertAndBuiltinGatesAutoRegister(t *testing.T) {
	rt := NewRuntime()
	rt.Parse(`hilbert Qubit: dim 2`)

	if h := rt.HilbertSpaces["Qubit"]; h == nil || h.Dim != 2 {
		t.Fatalf("expected Qubit hilbert space dim=2, got %v", h)
	}
	for _, name := range []string{"H_Qubit", "X_Qubit", "Y_Qubit", "Z_Qubit", "I_Qubit"} {
		if rt.Gates[name] == nil {
			t.Fatalf("expected builtin gate '%s' to be auto-registered", name)
		}
	}
}

func TestKetApplyTensorMeasurePipeline(t *testing.T) {
	rt := NewRuntime()
	rt.Parse(`
hilbert Qubit: dim 2
let ket0 = ket(Qubit, 1, 0)
let plus = apply(H_Qubit, ket0)
let m = measure(plus)
`)
	plus := rt.QuantumStates["plus"]
	if plus == nil {
		t.Fatal("expected 'plus' quantum state to exist")
	}
	probs := plus.Probabilities()
	if math.Abs(probs[0]-0.5) > 1e-9 || math.Abs(probs[1]-0.5) > 1e-9 {
		t.Fatalf("H|0> should give 50/50 probs, got %v", probs)
	}
	if _, ok := rt.Measurements["m"]; !ok {
		t.Fatal("expected measurement outcome 'm' to be recorded")
	}
}

func TestTensorDensityPurity(t *testing.T) {
	rt := NewRuntime()
	rt.Parse(`
hilbert Qubit: dim 2
let ket0 = ket(Qubit, 1, 0)
let rho = density(ket0)
let pur = purity(rho)
`)
	if got := rt.Scalars["pur"]; math.Abs(got-1.0) > 1e-9 {
		t.Fatalf("purity of a pure state should be 1, got %v", got)
	}
}

func TestGateDeclarationChecksUnitarity(t *testing.T) {
	rt := NewRuntime()
	rt.Parse(`
hilbert Q2: dim 2
gate MyH: Q2 -> Q2 {
    row [0.7071067811865476, 0.7071067811865476]
    row [0.7071067811865476, -0.7071067811865476]
}
`)
	g := rt.Gates["MyH"]
	if g == nil {
		t.Fatal("expected gate 'MyH' to be registered")
	}
	if !g.IsUnitary() {
		t.Fatal("expected the declared Hadamard matrix to be unitary")
	}
}

func TestTeleportBuiltinRecoversOriginalState(t *testing.T) {
	rt := NewRuntime()
	rt.Parse(`
hilbert Qubit: dim 2
let psi = ket(Qubit, 0.6, 0.8)
let bob = teleport(psi)
`)
	psi := rt.QuantumStates["psi"]
	bob := rt.QuantumStates["bob"]
	if psi == nil || bob == nil {
		t.Fatal("expected both 'psi' and 'bob' quantum states to exist")
	}
	for i := range psi.Amplitudes {
		if psi.Amplitudes[i] != bob.Amplitudes[i] {
			t.Fatalf("teleport should recover psi exactly: psi=%v bob=%v", psi.Amplitudes, bob.Amplitudes)
		}
	}
}

func TestShotsBuiltinProducesHistogramSummingToN(t *testing.T) {
	rt := NewRuntime()
	rt.Parse(`
hilbert Qubit: dim 2
let ket0 = ket(Qubit, 1, 0)
let plus = apply(H_Qubit, ket0)
let hist = shots(plus, 500)
`)
	hist := rt.Histograms["hist"]
	if hist == nil {
		t.Fatal("expected histogram 'hist' to exist")
	}
	total := 0
	for _, c := range hist {
		total += c
	}
	if total != 500 {
		t.Fatalf("expected histogram counts to sum to 500, got %d", total)
	}
}

func TestTechnologyDirectiveSwitchesBackend(t *testing.T) {
	rt := NewRuntime()
	rt.Parse(`technology: simulator`)
	if rt.Backend == nil {
		t.Fatal("expected a backend to be set after 'technology: simulator'")
	}
	if rt.Backend.Name() != "simulator" {
		t.Fatalf("expected backend name 'simulator', got %q", rt.Backend.Name())
	}
}

func TestTechnologyDirectiveReportsClearErrorWithoutCredentials(t *testing.T) {
	t.Setenv("IONQ_API_KEY", "") // force the missing-credentials path regardless of the host environment
	rt := NewRuntime()
	rt.Parse(`technology: ionq`)
	if rt.Backend != nil {
		t.Fatal("expected rt.Backend to remain nil when the technology switch fails")
	}
}

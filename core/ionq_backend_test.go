package core

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTrappedIonBackendStatePreparationUnsupported(t *testing.T) {
	b := &TrappedIonBackend{}
	if _, err := b.Apply(nil, nil); err != ErrStatePreparationUnsupported {
		t.Fatalf("Apply: want ErrStatePreparationUnsupported, got %v", err)
	}
	if _, _, err := b.Measure(nil); err != ErrStatePreparationUnsupported {
		t.Fatalf("Measure: want ErrStatePreparationUnsupported, got %v", err)
	}
}

func TestTrappedIonBackendBuildCircuitTranslatesGates(t *testing.T) {
	q := NewHilbertSpace("Q", 2)
	reg2 := NewHilbertSpace("Reg2", 4)
	c := NewQIRCircuit(2)
	c.AddGate(BuiltinH(q), 0)
	c.AddGate(BuiltinCNOT(reg2), 0, 1)

	b := &TrappedIonBackend{}
	ops, err := b.buildCircuit(c)
	if err != nil {
		t.Fatalf("buildCircuit error: %v", err)
	}
	if len(ops) != 2 || ops[0].Gate != "h" || ops[0].Target != 0 {
		t.Fatalf("unexpected H op: %+v", ops)
	}
	if ops[1].Gate != "cnot" || ops[1].Control != 0 || ops[1].Target != 1 {
		t.Fatalf("unexpected CNOT op: %+v", ops)
	}
}

func TestTrappedIonBackendBuildCircuitRejectsUnsupportedGate(t *testing.T) {
	q := NewHilbertSpace("Q", 2)
	c := NewQIRCircuit(1)
	c.AddGate(KronGate(BuiltinH(q), BuiltinI(q), NewHilbertSpace("Reg2", 4)), 0, 1)

	b := &TrappedIonBackend{}
	if _, err := b.buildCircuit(c); err == nil {
		t.Fatal("expected error translating a composite (kron) gate")
	}
}

// TestTrappedIonBackendRunEndToEnd mocks the three real IonQ endpoints
// (create job, get job, get results) and verifies Run wires auth headers,
// request body and response parsing correctly.
func TestTrappedIonBackendRunEndToEnd(t *testing.T) {
	var gotAuth string
	var gotCreateBody map[string]any

	mux := http.NewServeMux()
	mux.HandleFunc("/jobs", func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotCreateBody)
		json.NewEncoder(w).Encode(map[string]string{"id": "job-1", "status": "ready"})
	})
	mux.HandleFunc("/jobs/job-1", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"id": "job-1", "status": "completed"})
	})
	mux.HandleFunc("/jobs/job-1/results", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]float64{"0": 0.5, "3": 0.5})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	b := &TrappedIonBackend{
		apiKey:  "test-key",
		target:  "simulator",
		baseURL: srv.URL,
		client:  srv.Client(),
		poll:    0,
	}

	q := NewHilbertSpace("Q", 2)
	reg2 := NewHilbertSpace("Reg2", 4)
	c := NewQIRCircuit(2)
	c.AddGate(BuiltinH(q), 0)
	c.AddGate(BuiltinCNOT(reg2), 0, 1)

	counts, err := b.Run(c, 1000)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if gotAuth != "apiKey test-key" {
		t.Fatalf("expected Authorization 'apiKey test-key', got %q", gotAuth)
	}
	if gotCreateBody["target"] != "simulator" {
		t.Fatalf("expected target=simulator in request body, got %v", gotCreateBody)
	}
	if counts[0] != 500 || counts[3] != 500 {
		t.Fatalf("expected counts {0:500, 3:500}, got %v", counts)
	}
}

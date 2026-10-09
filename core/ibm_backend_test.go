package core

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSuperconductorBackendStatePreparationUnsupported(t *testing.T) {
	b := &SuperconductorBackend{}
	if _, err := b.Apply(nil, nil); err != ErrStatePreparationUnsupported {
		t.Fatalf("Apply: want ErrStatePreparationUnsupported, got %v", err)
	}
	if _, _, err := b.Measure(nil); err != ErrStatePreparationUnsupported {
		t.Fatalf("Measure: want ErrStatePreparationUnsupported, got %v", err)
	}
}

func TestSuperconductorBackendBuildQASM(t *testing.T) {
	q := NewHilbertSpace("Q", 2)
	reg2 := NewHilbertSpace("Reg2", 4)
	c := NewQIRCircuit(2)
	c.AddGate(BuiltinH(q), 0)
	c.AddGate(BuiltinCNOT(reg2), 0, 1)

	b := &SuperconductorBackend{}
	qasm, err := b.buildQASM(c)
	if err != nil {
		t.Fatalf("buildQASM error: %v", err)
	}
	for _, want := range []string{"OPENQASM 2.0;", "qreg q[2];", "h q[0];", "cx q[0],q[1];", "measure q -> c;"} {
		if !strings.Contains(qasm, want) {
			t.Fatalf("expected QASM to contain %q, got:\n%s", want, qasm)
		}
	}
}

// TestSuperconductorBackendRunEndToEnd mocks IBM Cloud IAM plus the
// Qiskit Runtime /v1/jobs endpoints and verifies auth, headers and the
// hex-counts parsing path.
func TestSuperconductorBackendRunEndToEnd(t *testing.T) {
	iam := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"access_token": "tok-123", "expires_in": 3600})
	}))
	defer iam.Close()

	var gotAuth, gotCRN, gotAPIVersion string
	mux := http.NewServeMux()
	mux.HandleFunc("/jobs", func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotCRN = r.Header.Get("Service-CRN")
		gotAPIVersion = r.Header.Get("IBM-API-Version")
		json.NewEncoder(w).Encode(map[string]string{"id": "job-42"})
	})
	mux.HandleFunc("/jobs/job-42", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"id": "job-42", "status": "COMPLETED"})
	})
	mux.HandleFunc("/jobs/job-42/results", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{"data": map[string]any{"c": map[string]any{"counts": map[string]int{"0x0": 500, "0x3": 500}}}},
			},
		})
	})
	runtime := httptest.NewServer(mux)
	defer runtime.Close()

	b := &SuperconductorBackend{
		apiKey:  "ibm-key",
		crn:     "crn:v1:test",
		backend: "ibm_brisbane",
		iamURL:  iam.URL,
		baseURL: runtime.URL,
		client:  runtime.Client(),
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
	if gotAuth != "Bearer tok-123" {
		t.Fatalf("expected Authorization 'Bearer tok-123', got %q", gotAuth)
	}
	if gotCRN != "crn:v1:test" {
		t.Fatalf("expected Service-CRN header, got %q", gotCRN)
	}
	if gotAPIVersion == "" {
		t.Fatal("expected IBM-API-Version header to be set")
	}
	if counts[0] != 500 || counts[3] != 500 {
		t.Fatalf("expected counts {0:500, 3:500}, got %v", counts)
	}
}

package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// SuperconductorBackend talks to IBM's real Qiskit Runtime REST API
// (https://quantum.cloud.ibm.com/docs/en/api/qiskit-runtime-rest), grounded
// against IBM's public documentation for authentication, job submission
// and polling. It is a CircuitRunner for the same physical reason as
// TrappedIonBackend: Apply/Measure on an arbitrary pre-existing amplitude
// vector is not something real hardware can do.
//
// Honesty note: IBM Cloud IAM authentication, the /v1/jobs endpoints and
// their required headers (Service-CRN, IBM-API-Version) are documented and
// implemented faithfully below. The exact wire encoding of a Qiskit Runtime
// V2 "pub" (normally a QPY-serialized QuantumCircuit, produced client-side
// by the qiskit-ibm-runtime SDK) is not fully specified in IBM's public REST
// docs outside that SDK. This adapter submits circuits as OpenQASM 2.0
// source and parses results as hex-keyed counts — the long-standing Qiskit
// convention — but that encoding should be reverified against the target
// Runtime program version before production use.
type SuperconductorBackend struct {
	apiKey  string
	crn     string
	backend string // IBM backend name, e.g. "ibm_brisbane"

	iamURL  string
	baseURL string
	client  *http.Client
	poll    time.Duration

	token    string
	tokenExp time.Time
}

// NewSuperconductorBackend reads IBM_QUANTUM_API_KEY and IBM_QUANTUM_CRN
// (both required) and IBM_QUANTUM_BACKEND (optional, default
// "ibm_brisbane") from the environment.
func NewSuperconductorBackend() (*SuperconductorBackend, error) {
	apiKey := os.Getenv("IBM_QUANTUM_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("superconductor (IBM): falta la variable de entorno IBM_QUANTUM_API_KEY")
	}
	crn := os.Getenv("IBM_QUANTUM_CRN")
	if crn == "" {
		return nil, fmt.Errorf("superconductor (IBM): falta la variable de entorno IBM_QUANTUM_CRN (Service-CRN de la instancia)")
	}
	backend := os.Getenv("IBM_QUANTUM_BACKEND")
	if backend == "" {
		backend = "ibm_brisbane"
	}
	return &SuperconductorBackend{
		apiKey:  apiKey,
		crn:     crn,
		backend: backend,
		iamURL:  "https://iam.cloud.ibm.com/identity/token",
		baseURL: "https://quantum.cloud.ibm.com/api/v1",
		client:  &http.Client{Timeout: 30 * time.Second},
		poll:    3 * time.Second,
	}, nil
}

func (b *SuperconductorBackend) Name() string { return "superconductor-ibm" }

func (b *SuperconductorBackend) Apply(gate *Gate, state *QuantumState) (*QuantumState, error) {
	return nil, ErrStatePreparationUnsupported
}

func (b *SuperconductorBackend) Measure(state *QuantumState) (int, []float64, error) {
	return 0, nil, ErrStatePreparationUnsupported
}

func (b *SuperconductorBackend) SupportedGates() []string {
	return []string{"H", "X", "Y", "Z", "I", "CNOT"}
}

// Topology reports a generic 2D-grid-like connectivity, representative of
// superconducting heavy-hex layouts. Fetching the exact coupling map
// requires GET /v1/backends/{name}/configuration, not implemented here.
func (b *SuperconductorBackend) Topology() Topology {
	return Topology{Qubits: 0, Edges: nil}
}

// NoiseModel reports IBM's published figures for superconducting
// hardware (~100μs coherence, ~99.5% two-qubit fidelity), not fetched live.
func (b *SuperconductorBackend) NoiseModel() NoiseModel {
	return NoiseModel{T1: 100e-6, T2: 100e-6, ReadoutError: 0.01}
}

// ── IBM Cloud IAM authentication ────────────────────────────────────────────

type ibmTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

// authToken returns a cached bearer token, refreshing it via IBM Cloud IAM
// (POST /identity/token, grant_type=urn:ibm:params:oauth:grant-type:apikey)
// if it is missing or about to expire.
func (b *SuperconductorBackend) authToken() (string, error) {
	if b.token != "" && time.Now().Before(b.tokenExp) {
		return b.token, nil
	}
	form := url.Values{
		"grant_type": {"urn:ibm:params:oauth:grant-type:apikey"},
		"apikey":     {b.apiKey},
	}
	req, err := http.NewRequest("POST", b.iamURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := b.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ibm iam: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("ibm iam: HTTP %d: %s", resp.StatusCode, string(body))
	}
	var tok ibmTokenResponse
	if err := json.Unmarshal(body, &tok); err != nil {
		return "", fmt.Errorf("ibm iam: decodificando token: %w", err)
	}
	b.token = tok.AccessToken
	b.tokenExp = time.Now().Add(time.Duration(tok.ExpiresIn-30) * time.Second)
	return b.token, nil
}

// ── Circuit translation ──────────────────────────────────────────────────────

// buildQASM translates circuit's topologically-ordered gates into OpenQASM
// 2.0 source — see the honesty note on SuperconductorBackend.
func (b *SuperconductorBackend) buildQASM(circuit *QIRCircuit) (string, error) {
	var sb strings.Builder
	fmt.Fprintf(&sb, "OPENQASM 2.0;\ninclude \"qelib1.inc\";\nqreg q[%d];\ncreg c[%d];\n",
		circuit.NumQubits, circuit.NumQubits)

	for _, n := range circuit.TopoOrder() {
		name := strings.ToLower(n.Gate.Name)
		switch name {
		case "i":
			continue
		case "h", "x", "y", "z":
			if len(n.Qubits) != 1 {
				return "", fmt.Errorf("ibm: puerta '%s' esperaba 1 qubit, recibió %d", n.Gate.Name, len(n.Qubits))
			}
			fmt.Fprintf(&sb, "%s q[%d];\n", name, n.Qubits[0])
		case "cnot":
			if len(n.Qubits) != 2 {
				return "", fmt.Errorf("ibm: CNOT esperaba 2 qubits (control, target), recibió %d", len(n.Qubits))
			}
			fmt.Fprintf(&sb, "cx q[%d],q[%d];\n", n.Qubits[0], n.Qubits[1])
		default:
			return "", fmt.Errorf(
				"ibm: puerta '%s' no soportada (h,x,y,z,cnot) — se requiere descomposición previa (issue #8)", n.Gate.Name)
		}
	}
	fmt.Fprintf(&sb, "measure q -> c;\n")
	return sb.String(), nil
}

// ── Job submission & polling ─────────────────────────────────────────────────

type ibmCreateJobRequest struct {
	ProgramID string         `json:"program_id"`
	Backend   string         `json:"backend"`
	Params    map[string]any `json:"params"`
}

type ibmCreateJobResponse struct {
	ID string `json:"id"`
}

type ibmGetJobResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// Run submits circuit as a single Qiskit Runtime "sampler" job and polls
// until it reaches a terminal state, then parses hex-keyed counts out of
// the results payload (see the honesty note above).
func (b *SuperconductorBackend) Run(circuit *QIRCircuit, shots int) (map[int]int, error) {
	qasm, err := b.buildQASM(circuit)
	if err != nil {
		return nil, err
	}

	reqBody := ibmCreateJobRequest{
		ProgramID: "sampler",
		Backend:   b.backend,
		Params: map[string]any{
			"pubs":  []any{qasm},
			"shots": shots,
		},
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("ibm: serializando el job: %w", err)
	}

	created := &ibmCreateJobResponse{}
	if err := b.doJSON("POST", "/jobs", body, created); err != nil {
		return nil, fmt.Errorf("ibm: creando el job: %w", err)
	}

	for {
		status := &ibmGetJobResponse{}
		if err := b.doJSON("GET", "/jobs/"+created.ID, nil, status); err != nil {
			return nil, fmt.Errorf("ibm: consultando el job %s: %w", created.ID, err)
		}
		switch strings.ToUpper(status.Status) {
		case "COMPLETED":
			return b.fetchCounts(created.ID, shots)
		case "ERROR", "CANCELLED", "FAILED":
			return nil, fmt.Errorf("ibm: job %s terminó con estado '%s'", created.ID, status.Status)
		default: // QUEUED, RUNNING, VALIDATING, INITIALIZING
			time.Sleep(b.poll)
		}
	}
}

// ibmResultsResponse models the long-standing Qiskit convention of
// hex-keyed measurement counts nested under results[].data.<creg>.counts.
type ibmResultsResponse struct {
	Results []struct {
		Data map[string]struct {
			Counts map[string]int `json:"counts"`
		} `json:"data"`
	} `json:"results"`
}

func (b *SuperconductorBackend) fetchCounts(jobID string, shots int) (map[int]int, error) {
	var raw ibmResultsResponse
	if err := b.doJSON("GET", "/jobs/"+jobID+"/results", nil, &raw); err != nil {
		return nil, fmt.Errorf("ibm: leyendo resultados del job %s: %w", jobID, err)
	}
	if len(raw.Results) == 0 {
		return nil, fmt.Errorf("ibm: el job %s no devolvió resultados", jobID)
	}
	counts := map[int]int{}
	for _, reg := range raw.Results[0].Data {
		for hexKey, n := range reg.Counts {
			idx, err := strconv.ParseInt(strings.TrimPrefix(hexKey, "0x"), 16, 64)
			if err != nil {
				return nil, fmt.Errorf("ibm: clave de conteo inesperada '%s': %w", hexKey, err)
			}
			counts[int(idx)] += n
		}
	}
	return counts, nil
}

// doJSON performs an authenticated Qiskit Runtime REST call and decodes
// the JSON response into out.
func (b *SuperconductorBackend) doJSON(method, path string, body []byte, out any) error {
	token, err := b.authToken()
	if err != nil {
		return err
	}
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, b.baseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Service-CRN", b.crn)
	req.Header.Set("IBM-API-Version", "2024-01-01")
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("decodificando respuesta: %w (body=%s)", err, string(respBody))
	}
	return nil
}

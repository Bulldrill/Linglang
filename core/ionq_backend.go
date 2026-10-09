package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// TrappedIonBackend talks to IonQ's real Quantum Cloud REST API
// (https://docs.ionq.com/api-reference/v0.3), grounded against IonQ's
// public v0.3 documentation. It is a CircuitRunner: real trapped-ion
// hardware cannot accept an arbitrary pre-existing amplitude vector, so
// Apply/Measure return ErrStatePreparationUnsupported — only Run (submit a
// full circuit, poll, read back the histogram) is physically meaningful.
type TrappedIonBackend struct {
	apiKey  string
	target  string // IonQ hardware target, e.g. "simulator", "qpu.aria-1"
	baseURL string
	client  *http.Client
	poll    time.Duration
}

// NewTrappedIonBackend reads IONQ_API_KEY (required) and IONQ_TARGET
// (optional, default "simulator" — IonQ's own cloud-hosted simulator,
// distinct from LinLang's local SimulatorBackend) from the environment.
func NewTrappedIonBackend() (*TrappedIonBackend, error) {
	key := os.Getenv("IONQ_API_KEY")
	if key == "" {
		return nil, fmt.Errorf("trapped-ion (IonQ): falta la variable de entorno IONQ_API_KEY")
	}
	target := os.Getenv("IONQ_TARGET")
	if target == "" {
		target = "simulator"
	}
	return &TrappedIonBackend{
		apiKey:  key,
		target:  target,
		baseURL: "https://api.ionq.co/v0.3",
		client:  &http.Client{Timeout: 30 * time.Second},
		poll:    2 * time.Second,
	}, nil
}

func (b *TrappedIonBackend) Name() string { return "trapped-ion-ionq" }

func (b *TrappedIonBackend) Apply(gate *Gate, state *QuantumState) (*QuantumState, error) {
	return nil, ErrStatePreparationUnsupported
}

func (b *TrappedIonBackend) Measure(state *QuantumState) (int, []float64, error) {
	return 0, nil, ErrStatePreparationUnsupported
}

// SupportedGates lists the QIS gateset names this backend translates
// (docs.ionq.com/api-reference/v0.3/jobs/create-a-job): h, x, y, z, cnot.
// Composite gates (kron products, user `gate{}` declarations) are not
// translated — Run returns an error naming the unsupported gate.
func (b *TrappedIonBackend) SupportedGates() []string {
	return []string{"H", "X", "Y", "Z", "I", "CNOT"}
}

// Topology reports trapped-ion connectivity as all-to-all (Edges == nil),
// matching IonQ's physical architecture — any ion can be entangled with any
// other via the shared motional mode, unlike grid-connected superconductors.
func (b *TrappedIonBackend) Topology() Topology {
	return Topology{}
}

// NoiseModel reports IonQ's published Aria-class figures (not fetched live
// from the backend-characterization endpoint): ~1 minute coherence, ~99.9%
// two-qubit gate fidelity.
func (b *TrappedIonBackend) NoiseModel() NoiseModel {
	return NoiseModel{T1: 60, T2: 60, ReadoutError: 0.001}
}

// ── IonQ wire format ──────────────────────────────────────────────────────────

type ionqCircuitOp struct {
	Gate    string `json:"gate"`
	Target  int    `json:"target,omitempty"`
	Targets []int  `json:"targets,omitempty"`
	Control int    `json:"control,omitempty"`
}

type ionqCreateJobRequest struct {
	Target string `json:"target"`
	Shots  int    `json:"shots"`
	Input  struct {
		Format  string          `json:"format"`
		Gateset string          `json:"gateset"`
		Qubits  int             `json:"qubits"`
		Circuit []ionqCircuitOp `json:"circuit"`
	} `json:"input"`
}

type ionqCreateJobResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type ionqGetJobResponse struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Failure *struct {
		Error string `json:"error"`
	} `json:"failure,omitempty"`
}

// buildCircuit translates circuit's topologically-ordered gates into IonQ's
// ionq.circuit.v0 QIS format.
func (b *TrappedIonBackend) buildCircuit(circuit *QIRCircuit) ([]ionqCircuitOp, error) {
	ops := make([]ionqCircuitOp, 0, len(circuit.Nodes))
	for _, n := range circuit.TopoOrder() {
		name := strings.ToLower(n.Gate.Name)
		switch name {
		case "i":
			continue // identity: no-op on real hardware
		case "h", "x", "y", "z":
			if len(n.Qubits) != 1 {
				return nil, fmt.Errorf("ionq: puerta '%s' esperaba 1 qubit, recibió %d", n.Gate.Name, len(n.Qubits))
			}
			ops = append(ops, ionqCircuitOp{Gate: name, Target: n.Qubits[0]})
		case "cnot":
			if len(n.Qubits) != 2 {
				return nil, fmt.Errorf("ionq: CNOT esperaba 2 qubits (control, target), recibió %d", len(n.Qubits))
			}
			ops = append(ops, ionqCircuitOp{Gate: "cnot", Control: n.Qubits[0], Target: n.Qubits[1]})
		default:
			return nil, fmt.Errorf(
				"ionq: puerta '%s' no soportada por el gateset QIS (h,x,y,z,cnot) — "+
					"se requiere descomposición previa (ver issue #8, optimizador algebraico)", n.Gate.Name)
		}
	}
	return ops, nil
}

// Run submits circuit as a single IonQ job, polls until it leaves
// submitted/ready/running, and returns the measurement histogram.
func (b *TrappedIonBackend) Run(circuit *QIRCircuit, shots int) (map[int]int, error) {
	ops, err := b.buildCircuit(circuit)
	if err != nil {
		return nil, err
	}

	var req ionqCreateJobRequest
	req.Target = b.target
	req.Shots = shots
	req.Input.Format = "ionq.circuit.v0"
	req.Input.Gateset = "qis"
	req.Input.Qubits = circuit.NumQubits
	req.Input.Circuit = ops

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("ionq: serializando el job: %w", err)
	}

	created, err := b.doJSON("POST", "/jobs", body, &ionqCreateJobResponse{})
	if err != nil {
		return nil, fmt.Errorf("ionq: creando el job: %w", err)
	}
	jobID := created.(*ionqCreateJobResponse).ID

	for {
		status, err := b.doJSON("GET", "/jobs/"+jobID, nil, &ionqGetJobResponse{})
		if err != nil {
			return nil, fmt.Errorf("ionq: consultando el job %s: %w", jobID, err)
		}
		job := status.(*ionqGetJobResponse)
		switch job.Status {
		case "completed":
			return b.fetchHistogram(jobID, shots)
		case "failed", "canceled":
			msg := job.Status
			if job.Failure != nil {
				msg = job.Failure.Error
			}
			return nil, fmt.Errorf("ionq: job %s terminó con estado '%s': %s", jobID, job.Status, msg)
		default: // submitted, ready, running
			time.Sleep(b.poll)
		}
	}
}

// fetchHistogram reads the sparse probability histogram returned by
// GET /jobs/{id}/results?decimal=true and converts it to shot counts.
func (b *TrappedIonBackend) fetchHistogram(jobID string, shots int) (map[int]int, error) {
	raw := map[string]float64{}
	if _, err := b.doJSON("GET", "/jobs/"+jobID+"/results?decimal=true", nil, &raw); err != nil {
		return nil, fmt.Errorf("ionq: leyendo resultados del job %s: %w", jobID, err)
	}
	counts := make(map[int]int, len(raw))
	for key, prob := range raw {
		idx, err := strconv.Atoi(key)
		if err != nil {
			return nil, fmt.Errorf("ionq: índice de estado inesperado '%s' en el histograma: %w", key, err)
		}
		counts[idx] = int(prob*float64(shots) + 0.5)
	}
	return counts, nil
}

// doJSON performs an authenticated IonQ API call and decodes the JSON
// response into out (a pointer), returning out itself for convenience.
func (b *TrappedIonBackend) doJSON(method, path string, body []byte, out any) (any, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, b.baseURL+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "apiKey "+b.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return nil, fmt.Errorf("decodificando respuesta: %w (body=%s)", err, string(respBody))
	}
	return out, nil
}

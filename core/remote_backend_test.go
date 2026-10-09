package core

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"

	pb "linlang-go/proto/quantumnode"
)

// quantumNodeServer mirrors cmd/quantum-node's handlers, duplicated here
// (rather than imported — cmd/quantum-node is package main) so the test
// can start a real gRPC server over a real TCP listener and dial it with
// the actual RemoteBackend client, exercising the full wire round trip:
// gate/state marshalling, the RPC itself, and unmarshalling the result.
type quantumNodeServer struct {
	pb.UnimplementedQuantumNodeServer
	backend QuantumBackend
}

func (s *quantumNodeServer) Apply(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	gate := testProtoToGate(req.Gate)
	state := testProtoToState(req.State)
	result, err := s.backend.Apply(gate, state)
	if err != nil {
		return nil, err
	}
	return &pb.ApplyResponse{State: stateToProto(result)}, nil
}

func (s *quantumNodeServer) Measure(ctx context.Context, req *pb.MeasureRequest) (*pb.MeasureResponse, error) {
	state := testProtoToState(req.State)
	outcome, probs, err := s.backend.Measure(state)
	if err != nil {
		return nil, err
	}
	return &pb.MeasureResponse{Outcome: int32(outcome), Probabilities: probs}, nil
}

func (s *quantumNodeServer) Info(ctx context.Context, req *pb.InfoRequest) (*pb.InfoResponse, error) {
	topo := s.backend.Topology()
	edges := make([]int32, 0, len(topo.Edges)*2)
	for _, e := range topo.Edges {
		edges = append(edges, int32(e[0]), int32(e[1]))
	}
	noise := s.backend.NoiseModel()
	return &pb.InfoResponse{
		Name:           s.backend.Name(),
		SupportedGates: s.backend.SupportedGates(),
		TopologyQubits: int32(topo.Qubits),
		TopologyEdges:  edges,
		T1:             noise.T1,
		T2:             noise.T2,
		ReadoutError:   noise.ReadoutError,
	}, nil
}

func testProtoToGate(g *pb.GateMatrix) *Gate {
	dim := int(g.Dim)
	matrix := make([][]complex128, dim)
	idx := 0
	for i := range matrix {
		matrix[i] = make([]complex128, dim)
		for j := range matrix[i] {
			c := g.Entries[idx]
			matrix[i][j] = complex(c.Real, c.Imag)
			idx++
		}
	}
	space := NewHilbertSpace("Remote", dim)
	return NewGate(g.Name, space, space, matrix)
}

func testProtoToState(sv *pb.StateVector) *QuantumState {
	dim := int(sv.Dim)
	amps := make([]complex128, dim)
	for i, c := range sv.Amplitudes {
		amps[i] = complex(c.Real, c.Imag)
	}
	return NewQuantumState(NewHilbertSpace("Remote", dim), amps)
}

// startTestQuantumNode starts a real gRPC server on an ephemeral localhost
// port and returns its address plus a cleanup func.
func startTestQuantumNode(t *testing.T, backend QuantumBackend) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := grpc.NewServer()
	pb.RegisterQuantumNodeServer(srv, &quantumNodeServer{backend: backend})
	go srv.Serve(lis)
	t.Cleanup(srv.Stop)
	return lis.Addr().String()
}

func TestRemoteBackendApplyMatchesLocalSimulator(t *testing.T) {
	addr := startTestQuantumNode(t, NewSimulatorBackend())

	remote, err := NewRemoteBackend(addr)
	if err != nil {
		t.Fatalf("NewRemoteBackend: %v", err)
	}
	defer remote.Close()

	if remote.Name() != "simulator" {
		t.Fatalf("expected remote Name()='simulator', got %q", remote.Name())
	}

	q := NewHilbertSpace("Q", 2)
	ket0 := NewQuantumState(q, []complex128{1, 0})
	h := BuiltinH(q)

	remoteResult, err := remote.Apply(h, ket0)
	if err != nil {
		t.Fatalf("remote Apply: %v", err)
	}
	localResult := h.Apply(ket0)

	for i := range localResult.Amplitudes {
		diff := remoteResult.Amplitudes[i] - localResult.Amplitudes[i]
		if r, im := real(diff), imag(diff); r*r+im*im > 1e-18 {
			t.Fatalf("remote Apply diverges from local: remote=%v local=%v",
				remoteResult.Amplitudes, localResult.Amplitudes)
		}
	}
	if remoteResult.Space != h.Codomain {
		t.Fatal("expected the remote result to be reattached to the caller's own Gate.Codomain")
	}
}

func TestRemoteBackendMeasureMatchesLocalSimulator(t *testing.T) {
	addr := startTestQuantumNode(t, NewSimulatorBackend())
	remote, err := NewRemoteBackend(addr)
	if err != nil {
		t.Fatalf("NewRemoteBackend: %v", err)
	}
	defer remote.Close()

	q := NewHilbertSpace("Q", 2)
	state := NewQuantumState(q, []complex128{0.6, 0.8})

	outcome, probs, err := remote.Measure(state)
	if err != nil {
		t.Fatalf("remote Measure: %v", err)
	}
	wantOutcome, wantProbs := state.Measure()
	if outcome != wantOutcome {
		t.Fatalf("remote outcome=%d, want %d", outcome, wantOutcome)
	}
	for i := range probs {
		if probs[i] != wantProbs[i] {
			t.Fatalf("remote probs=%v, want %v", probs, wantProbs)
		}
	}
}

func TestRemoteBackendInfoReflectsNoiseModel(t *testing.T) {
	noisy := NewNoisyBackend(NoiseModel{T1: 1e-5, T2: 2e-5, ReadoutError: 0.07}, 1e-6)
	addr := startTestQuantumNode(t, noisy)

	remote, err := NewRemoteBackend(addr)
	if err != nil {
		t.Fatalf("NewRemoteBackend: %v", err)
	}
	defer remote.Close()

	got := remote.NoiseModel()
	want := noisy.NoiseModel()
	if got != want {
		t.Fatalf("remote NoiseModel=%+v, want %+v", got, want)
	}
}

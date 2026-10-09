// cmd/quantum-node runs a single simulated QPU as a gRPC service (issue
// #20): "each container = a simulated QPU". It wraps a local
// core.QuantumBackend (the ideal simulator by default) and exposes it over
// the network via proto/quantumnode's QuantumNode service, so
// core.RemoteBackend — running inside the LinLang runtime on another
// machine, or in another container — can call Apply/Measure/Info exactly
// as it would a local backend.
//
// Run:  go run ./cmd/quantum-node/ -addr :50051
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"linlang-go/core"
	pb "linlang-go/proto/quantumnode"
)

type nodeServer struct {
	pb.UnimplementedQuantumNodeServer
	backend core.QuantumBackend
}

func (s *nodeServer) Apply(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	gate := protoToGate(req.Gate)
	state := protoToStateLocal(req.State)
	result, err := s.backend.Apply(gate, state)
	if err != nil {
		return nil, err
	}
	return &pb.ApplyResponse{State: stateToProtoLocal(result)}, nil
}

func (s *nodeServer) Measure(ctx context.Context, req *pb.MeasureRequest) (*pb.MeasureResponse, error) {
	state := protoToStateLocal(req.State)
	outcome, probs, err := s.backend.Measure(state)
	if err != nil {
		return nil, err
	}
	return &pb.MeasureResponse{Outcome: int32(outcome), Probabilities: probs}, nil
}

func (s *nodeServer) Info(ctx context.Context, req *pb.InfoRequest) (*pb.InfoResponse, error) {
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

// protoToGate/protoToStateLocal synthesize a fresh HilbertSpace server-side
// for each call: the server's gate/state math is purely dimension-driven
// (matrix-vector product, Born-rule measurement), so it never needs the
// caller's actual space identity — only RemoteBackend.Apply (the client)
// reattaches the result to the caller's real Gate.Codomain.
func protoToGate(g *pb.GateMatrix) *core.Gate {
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
	space := core.NewHilbertSpace("Remote", dim)
	return core.NewGate(g.Name, space, space, matrix)
}

func protoToStateLocal(sv *pb.StateVector) *core.QuantumState {
	dim := int(sv.Dim)
	amps := make([]complex128, dim)
	for i, c := range sv.Amplitudes {
		amps[i] = complex(c.Real, c.Imag)
	}
	return core.NewQuantumState(core.NewHilbertSpace("Remote", dim), amps)
}

func stateToProtoLocal(s *core.QuantumState) *pb.StateVector {
	amps := make([]*pb.Complex, len(s.Amplitudes))
	for i, c := range s.Amplitudes {
		amps[i] = &pb.Complex{Real: real(c), Imag: imag(c)}
	}
	return &pb.StateVector{Dim: int32(len(s.Amplitudes)), Amplitudes: amps}
}

func main() {
	addr := flag.String("addr", ":50051", "dirección de escucha gRPC")
	flag.Parse()

	backend := core.NewSimulatorBackend()
	srv := grpc.NewServer()
	pb.RegisterQuantumNodeServer(srv, &nodeServer{backend: backend})
	reflection.Register(srv) // grpcurl/evans discoverability for ops debugging

	lis, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("quantum-node: listen %s: %v", *addr, err)
	}
	fmt.Printf("🔬 quantum-node escuchando en %s (backend=%s)\n", *addr, backend.Name())
	if err := srv.Serve(lis); err != nil {
		log.Fatalf("quantum-node: serve: %v", err)
	}
}

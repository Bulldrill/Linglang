package core

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "linlang-go/proto/quantumnode"
)

// RemoteBackend is a QuantumBackend that delegates Apply/Measure to a
// quantum-node gRPC server (issue #20) — "each container is a simulated
// QPU" realized as a network-callable QuantumBackend, so a distributed
// scheduler (ScheduleCircuit, #18) routes operations to whichever
// node/container currently owns a qubit (HilbertRegistry, #16) exactly as
// transparently as it would route to a local, in-process backend: both
// satisfy the same QuantumBackend interface.
type RemoteBackend struct {
	addr   string
	conn   *grpc.ClientConn
	client pb.QuantumNodeClient
	info   *pb.InfoResponse
}

// NewRemoteBackend dials addr (host:port) and fetches the node's static
// capabilities once via the Info RPC, caching them for Name/
// SupportedGates/Topology/NoiseModel so those methods never block on the
// network.
func NewRemoteBackend(addr string) (*RemoteBackend, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("remote backend: dial %s: %w", addr, err)
	}
	client := pb.NewQuantumNodeClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	info, err := client.Info(ctx, &pb.InfoRequest{})
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("remote backend: Info(%s): %w", addr, err)
	}

	return &RemoteBackend{addr: addr, conn: conn, client: client, info: info}, nil
}

// Close releases the underlying gRPC connection.
func (b *RemoteBackend) Close() error { return b.conn.Close() }

func (b *RemoteBackend) Name() string { return b.info.Name }

func (b *RemoteBackend) Apply(gate *Gate, state *QuantumState) (*QuantumState, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := b.client.Apply(ctx, &pb.ApplyRequest{
		Gate:  gateToProto(gate),
		State: stateToProto(state),
	})
	if err != nil {
		return nil, fmt.Errorf("remote backend %s: Apply: %w", b.addr, err)
	}
	return protoToState(resp.State, gate.Codomain), nil
}

func (b *RemoteBackend) Measure(state *QuantumState) (int, []float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := b.client.Measure(ctx, &pb.MeasureRequest{State: stateToProto(state)})
	if err != nil {
		return 0, nil, fmt.Errorf("remote backend %s: Measure: %w", b.addr, err)
	}
	return int(resp.Outcome), resp.Probabilities, nil
}

func (b *RemoteBackend) SupportedGates() []string { return b.info.SupportedGates }

func (b *RemoteBackend) Topology() Topology {
	var edges [][2]int
	for i := 0; i+1 < len(b.info.TopologyEdges); i += 2 {
		edges = append(edges, [2]int{int(b.info.TopologyEdges[i]), int(b.info.TopologyEdges[i+1])})
	}
	return Topology{Qubits: int(b.info.TopologyQubits), Edges: edges}
}

func (b *RemoteBackend) NoiseModel() NoiseModel {
	return NoiseModel{T1: b.info.T1, T2: b.info.T2, ReadoutError: b.info.ReadoutError}
}

// ── wire <-> core conversions ────────────────────────────────────────────────

func gateToProto(g *Gate) *pb.GateMatrix {
	n := len(g.Matrix)
	entries := make([]*pb.Complex, 0, n*n)
	for _, row := range g.Matrix {
		for _, c := range row {
			entries = append(entries, &pb.Complex{Real: real(c), Imag: imag(c)})
		}
	}
	return &pb.GateMatrix{Name: g.Name, Dim: int32(n), Entries: entries}
}

func stateToProto(s *QuantumState) *pb.StateVector {
	amps := make([]*pb.Complex, len(s.Amplitudes))
	for i, c := range s.Amplitudes {
		amps[i] = &pb.Complex{Real: real(c), Imag: imag(c)}
	}
	return &pb.StateVector{Dim: int32(len(s.Amplitudes)), Amplitudes: amps}
}

func protoToState(sv *pb.StateVector, space *HilbertSpace) *QuantumState {
	amps := make([]complex128, len(sv.Amplitudes))
	for i, c := range sv.Amplitudes {
		amps[i] = complex(c.Real, c.Imag)
	}
	return NewQuantumState(space, amps)
}

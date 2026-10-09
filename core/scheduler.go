package core

import "fmt"

// ClusterGraph is the graph of distributed nodes connected by
// QuantumChannels — the cluster-level analogue of Topology (#1), which
// describes connectivity *within* a single QPU.
type ClusterGraph struct {
	edges map[NodeID]map[NodeID]bool
}

func NewClusterGraph() *ClusterGraph {
	return &ClusterGraph{edges: map[NodeID]map[NodeID]bool{}}
}

// AddChannel registers that ch connects its two nodes.
func (g *ClusterGraph) AddChannel(ch QuantumChannel) {
	g.connect(NodeID(ch.NodeA()), NodeID(ch.NodeB()))
}

func (g *ClusterGraph) connect(a, b NodeID) {
	if g.edges[a] == nil {
		g.edges[a] = map[NodeID]bool{}
	}
	if g.edges[b] == nil {
		g.edges[b] = map[NodeID]bool{}
	}
	g.edges[a][b] = true
	g.edges[b][a] = true
}

// Connected reports whether a and b have a direct QuantumChannel.
func (g *ClusterGraph) Connected(a, b NodeID) bool {
	if a == b {
		return true
	}
	return g.edges[a] != nil && g.edges[a][b]
}

// TeleportStep records one qubit migration the scheduler determined is
// necessary between two circuit nodes. A cross-node move can only ever be
// a Teleport (#14), never a SWAP (#9): no-cloning forbids physically
// copying a qubit's amplitudes across a network link the way SWAP permutes
// them within a single register (see docs/teleportation.md).
type TeleportStep struct {
	Qubit    int // logical qubit index being migrated
	From, To NodeID
}

// ScheduleCircuit partitions circuit across the nodes named by ownerOf
// (logical qubit index -> the node currently holding it) and cluster (which
// pairs of nodes have a QuantumChannel), returning the sequence of
// TeleportSteps required so every multi-qubit gate ends up with all its
// qubits co-located on one node, plus the resulting final ownership.
//
// This is the distributed analogue of RouteCircuit (#9) — same greedy,
// qubit-by-qubit co-location strategy, with Teleport standing in for SWAP.
// It is also, by construction, an instance of graph embedding: mapping a
// circuit's logical qubit-interaction structure onto the physical cluster
// topology (#19). As the roadmap note in the thesis (Épica 2) already
// states, minimizing the number of cross-node teleportations for an
// arbitrary circuit and cluster graph is the NP-hard graph-embedding
// problem itself. ScheduleCircuit does not attempt that minimization — it
// greedily co-locates each gate's qubits onto its first qubit's current
// node — but it always produces a correct partition plus the exact
// Teleport sequence that partition requires.
func ScheduleCircuit(circuit *QIRCircuit, ownerOf []NodeID, cluster *ClusterGraph) ([]TeleportStep, map[int]NodeID, error) {
	owner := make([]NodeID, len(ownerOf))
	copy(owner, ownerOf)
	var steps []TeleportStep

	for _, n := range circuit.TopoOrder() {
		if len(n.Qubits) < 2 {
			continue // single-qubit gates never require a cross-node move
		}
		target := owner[n.Qubits[0]]
		for _, q := range n.Qubits[1:] {
			if owner[q] == target {
				continue
			}
			if !cluster.Connected(owner[q], target) {
				return nil, nil, fmt.Errorf(
					"scheduler: no hay QuantumChannel entre '%s' y '%s' (requerido para el qubit %d)",
					owner[q], target, q)
			}
			steps = append(steps, TeleportStep{Qubit: q, From: owner[q], To: target})
			owner[q] = target
		}
	}

	final := make(map[int]NodeID, len(owner))
	for i, o := range owner {
		final[i] = o
	}
	return steps, final, nil
}

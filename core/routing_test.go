package core

import "testing"

func linearTopology(n int) Topology {
	edges := make([][2]int, 0, n-1)
	for i := 0; i < n-1; i++ {
		edges = append(edges, [2]int{i, i + 1})
	}
	return Topology{Qubits: n, Edges: edges}
}

func TestRouteCircuitInsertsSwapForDistantQubits(t *testing.T) {
	reg2 := NewHilbertSpace("Reg2", 4)
	topo := linearTopology(4) // 0-1-2-3

	c := NewQIRCircuit(4)
	c.AddGate(BuiltinCNOT(reg2), 0, 2) // not adjacent on a line

	routed, mapping, err := RouteCircuit(c, topo)
	if err != nil {
		t.Fatalf("RouteCircuit error: %v", err)
	}
	if len(routed.Nodes) != 2 {
		t.Fatalf("expected 1 SWAP + 1 CNOT = 2 nodes, got %d", len(routed.Nodes))
	}
	if routed.Nodes[0].Gate.Name != "SWAP" {
		t.Fatalf("expected first node to be a SWAP, got %s", routed.Nodes[0].Gate.Name)
	}
	cnotNode := routed.Nodes[1]
	a, b := cnotNode.Qubits[0], cnotNode.Qubits[1]
	if !topo.Connected(a, b) {
		t.Fatalf("routed CNOT acts on non-adjacent physical qubits %d,%d", a, b)
	}
	// Logical qubit 0 and logical qubit 2 must end up on the physical
	// qubits the CNOT actually touches.
	if mapping[0] != a && mapping[0] != b {
		t.Fatalf("logical qubit 0 (mapped to %d) is not one of the routed CNOT's qubits (%d,%d)", mapping[0], a, b)
	}
}

func TestRouteCircuitNoSwapWhenAlreadyAdjacent(t *testing.T) {
	reg2 := NewHilbertSpace("Reg2", 4)
	topo := linearTopology(4)

	c := NewQIRCircuit(4)
	c.AddGate(BuiltinCNOT(reg2), 1, 2) // already adjacent

	routed, _, err := RouteCircuit(c, topo)
	if err != nil {
		t.Fatalf("RouteCircuit error: %v", err)
	}
	if len(routed.Nodes) != 1 {
		t.Fatalf("expected no SWAPs inserted, got %d nodes", len(routed.Nodes))
	}
}

func TestRouteCircuitNoSwapOnFullyConnectedTopology(t *testing.T) {
	reg2 := NewHilbertSpace("Reg2", 4)
	c := NewQIRCircuit(4)
	c.AddGate(BuiltinCNOT(reg2), 0, 3)

	routed, _, err := RouteCircuit(c, Topology{}) // nil Edges = fully connected
	if err != nil {
		t.Fatalf("RouteCircuit error: %v", err)
	}
	if len(routed.Nodes) != 1 {
		t.Fatalf("fully connected topology should never need SWAPs, got %d nodes", len(routed.Nodes))
	}
}

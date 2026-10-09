package core

import "fmt"

// QIRNode is a single gate application in a QIRCircuit's dependency DAG.
type QIRNode struct {
	ID     int
	Gate   *Gate
	Qubits []int      // register indices this gate acts on
	Deps   []*QIRNode // nodes that must execute before this one
}

// QIRCircuit is the Quantum Intermediate Representation: a hardware-
// independent DAG of gate applications over a fixed-size qubit register.
// It sits between LinLang/Q source (ket/apply/tensor/gate calls) and a
// QuantumBackend, giving optimization passes (gate fusion, SWAP insertion,
// scheduling) a representation to work on before a concrete backend ever
// sees the circuit.
//
// The DAG structure falls out of qubit dependencies: two gates that touch
// disjoint qubits are independent nodes and may be reordered, scheduled in
// parallel, or mapped to different physical qubits without changing the
// circuit's semantics.
type QIRCircuit struct {
	NumQubits int
	Nodes     []*QIRNode

	lastWriter map[int]*QIRNode // qubit index -> most recent node touching it
}

// NewQIRCircuit creates an empty circuit over a register of numQubits qubits.
func NewQIRCircuit(numQubits int) *QIRCircuit {
	return &QIRCircuit{
		NumQubits:  numQubits,
		lastWriter: make(map[int]*QIRNode),
	}
}

// AddGate appends a gate application on qubits, wiring a dependency edge
// from every node that most recently touched one of those qubits.
func (c *QIRCircuit) AddGate(gate *Gate, qubits ...int) *QIRNode {
	node := &QIRNode{
		ID:     len(c.Nodes),
		Gate:   gate,
		Qubits: qubits,
	}
	seen := make(map[*QIRNode]bool)
	for _, q := range qubits {
		if dep, ok := c.lastWriter[q]; ok && !seen[dep] {
			node.Deps = append(node.Deps, dep)
			seen[dep] = true
		}
		c.lastWriter[q] = node
	}
	c.Nodes = append(c.Nodes, node)
	return node
}

// TopoOrder returns the circuit's nodes in a dependency-respecting order.
// AddGate always wires Deps to already-added nodes, so insertion order is
// already topological; this method exists so optimization passes that
// reorder or replace c.Nodes can recover a valid execution schedule.
func (c *QIRCircuit) TopoOrder() []*QIRNode {
	visited := make(map[*QIRNode]bool, len(c.Nodes))
	order := make([]*QIRNode, 0, len(c.Nodes))
	var visit func(n *QIRNode)
	visit = func(n *QIRNode) {
		if visited[n] {
			return
		}
		visited[n] = true
		for _, d := range n.Deps {
			visit(d)
		}
		order = append(order, n)
	}
	for _, n := range c.Nodes {
		visit(n)
	}
	return order
}

// Depth returns the circuit depth: the number of gates on the longest
// dependency chain. Hardware-independent optimizations aim to minimise this
// before the circuit is handed to a QuantumBackend.
func (c *QIRCircuit) Depth() int {
	depth := make(map[*QIRNode]int, len(c.Nodes))
	max := 0
	for _, n := range c.TopoOrder() {
		d := 1
		for _, dep := range n.Deps {
			if depth[dep]+1 > d {
				d = depth[dep] + 1
			}
		}
		depth[n] = d
		if d > max {
			max = d
		}
	}
	return max
}

// Execute runs every node in topological order against backend — the
// bridge from this hardware-independent IR to a concrete QuantumBackend.
func (c *QIRCircuit) Execute(backend QuantumBackend, initial *QuantumState) (*QuantumState, error) {
	state := initial
	for _, n := range c.TopoOrder() {
		var err error
		state, err = backend.Apply(n.Gate, state)
		if err != nil {
			return nil, fmt.Errorf("qir: nodo #%d (%s): %w", n.ID, n.Gate.Name, err)
		}
	}
	return state, nil
}

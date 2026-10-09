package core

import "fmt"

// RouteCircuit inserts SWAP gates so every multi-qubit gate in circuit acts
// on topology-adjacent physical qubits, using a greedy shortest-path
// heuristic (issue #9). It is not globally optimal — minimizing total SWAP
// count over an arbitrary topology is the NP-hard graph-embedding problem
// tracked separately as issue #19 — but it is always correct: the returned
// circuit, executed under the returned final logical→physical mapping,
// computes exactly the same unitary (up to qubit relabelling) that circuit
// would compute on a fully-connected device.
func RouteCircuit(circuit *QIRCircuit, topo Topology) (*QIRCircuit, map[int]int, error) {
	n := circuit.NumQubits
	mapping := make([]int, n) // mapping[logical] = physical
	for i := range mapping {
		mapping[i] = i
	}

	out := NewQIRCircuit(n)
	swapSpace := NewHilbertSpace("SWAP", 4)
	swapGate := BuiltinSWAP(swapSpace)

	for _, node := range circuit.TopoOrder() {
		phys := make([]int, len(node.Qubits))
		for i, lq := range node.Qubits {
			phys[i] = mapping[lq]
		}

		if len(phys) == 2 && !topo.Connected(phys[0], phys[1]) {
			path, err := shortestPath(topo, phys[0], phys[1], n)
			if err != nil {
				return nil, nil, fmt.Errorf("routing: %w", err)
			}
			for i := 0; i < len(path)-2; i++ {
				a, b := path[i], path[i+1]
				out.AddGate(swapGate, a, b)
				swapPhysicalAssignment(mapping, a, b)
			}
			phys[0] = mapping[node.Qubits[0]]
			phys[1] = mapping[node.Qubits[1]]
		}

		out.AddGate(node.Gate, phys...)
	}

	final := make(map[int]int, n)
	for lq, pq := range mapping {
		final[lq] = pq
	}
	return out, final, nil
}

// shortestPath returns a path of physical qubit indices from src to dst
// over topo's connectivity graph (BFS; unweighted). If topo is fully
// connected (Edges == nil) the direct path [src, dst] is returned.
func shortestPath(topo Topology, src, dst, numQubits int) ([]int, error) {
	if topo.Edges == nil {
		return []int{src, dst}, nil
	}

	adj := make(map[int][]int, numQubits)
	for _, e := range topo.Edges {
		adj[e[0]] = append(adj[e[0]], e[1])
		adj[e[1]] = append(adj[e[1]], e[0])
	}

	visited := map[int]bool{src: true}
	prev := map[int]int{}
	queue := []int{src}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur == dst {
			path := []int{dst}
			for path[len(path)-1] != src {
				path = append(path, prev[path[len(path)-1]])
			}
			for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
				path[i], path[j] = path[j], path[i]
			}
			return path, nil
		}
		for _, next := range adj[cur] {
			if !visited[next] {
				visited[next] = true
				prev[next] = cur
				queue = append(queue, next)
			}
		}
	}
	return nil, fmt.Errorf("no hay camino entre los qubits físicos %d y %d en la topología", src, dst)
}

// swapPhysicalAssignment exchanges which logical qubits are assigned to
// physical positions a and b, reflecting a SWAP gate applied between them.
func swapPhysicalAssignment(mapping []int, a, b int) {
	for lq := range mapping {
		switch mapping[lq] {
		case a:
			mapping[lq] = b
		case b:
			mapping[lq] = a
		}
	}
}

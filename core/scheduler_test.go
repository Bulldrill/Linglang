package core

import "testing"

func TestScheduleCircuitInsertsTeleportForCrossNodeGate(t *testing.T) {
	reg2 := NewHilbertSpace("Reg2", 4)
	c := NewQIRCircuit(2)
	c.AddGate(BuiltinCNOT(reg2), 0, 1)

	cluster := NewClusterGraph()
	q := NewHilbertSpace("Q", 2)
	cluster.AddChannel(NewLocalChannel("alice", "bob", q))

	owner := []NodeID{"alice", "bob"} // qubit 0 on alice, qubit 1 on bob
	steps, final, err := ScheduleCircuit(c, owner, cluster)
	if err != nil {
		t.Fatalf("ScheduleCircuit error: %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("expected exactly 1 teleport step, got %d (%v)", len(steps), steps)
	}
	if steps[0].Qubit != 1 || steps[0].From != "bob" || steps[0].To != "alice" {
		t.Fatalf("unexpected teleport step: %+v", steps[0])
	}
	if final[0] != "alice" || final[1] != "alice" {
		t.Fatalf("expected both qubits co-located on alice after scheduling, got %v", final)
	}
}

func TestScheduleCircuitNoTeleportWhenAlreadyCoLocated(t *testing.T) {
	reg2 := NewHilbertSpace("Reg2", 4)
	c := NewQIRCircuit(2)
	c.AddGate(BuiltinCNOT(reg2), 0, 1)

	owner := []NodeID{"alice", "alice"}
	steps, _, err := ScheduleCircuit(c, owner, NewClusterGraph())
	if err != nil {
		t.Fatalf("ScheduleCircuit error: %v", err)
	}
	if len(steps) != 0 {
		t.Fatalf("expected no teleport steps when already co-located, got %v", steps)
	}
}

func TestScheduleCircuitErrorsWithoutAChannel(t *testing.T) {
	reg2 := NewHilbertSpace("Reg2", 4)
	c := NewQIRCircuit(2)
	c.AddGate(BuiltinCNOT(reg2), 0, 1)

	owner := []NodeID{"alice", "mallory"} // no channel between these nodes
	if _, _, err := ScheduleCircuit(c, owner, NewClusterGraph()); err == nil {
		t.Fatal("expected an error when no QuantumChannel connects the required nodes")
	}
}

func TestScheduleCircuitSkipsSingleQubitGates(t *testing.T) {
	q := NewHilbertSpace("Q", 2)
	c := NewQIRCircuit(2)
	c.AddGate(BuiltinH(q), 0)
	c.AddGate(BuiltinX(q), 1)

	owner := []NodeID{"alice", "bob"} // disconnected, but no 2-qubit gate needs it
	steps, _, err := ScheduleCircuit(c, owner, NewClusterGraph())
	if err != nil {
		t.Fatalf("ScheduleCircuit error: %v", err)
	}
	if len(steps) != 0 {
		t.Fatalf("single-qubit gates should never require teleportation, got %v", steps)
	}
}

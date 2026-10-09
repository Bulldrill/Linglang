package core

import (
	"fmt"
	"math"
	"sync"
)

// BellPair is a freshly-provisioned entangled pair |Φ+⟩ shared by a
// QuantumChannel's two nodes. Entanglement is a joint property, so the pair
// is held as a single joint QuantumState over a dim-4 space rather than two
// independent per-node states; QubitA/QubitB record which tensor slot
// belongs to which node.
type BellPair struct {
	Space  *HilbertSpace // dim-4 joint space
	State  *QuantumState // |Φ+⟩ = 1/√2(|00⟩+|11⟩)
	QubitA int           // tensor slot (0) belonging to NodeA
	QubitB int           // tensor slot (1) belonging to NodeB
}

// QuantumChannel represents a quantum channel between two distributed
// nodes. It is the base abstraction the distributed-qubits protocols
// (teleportation, entanglement swapping) are built on: it can pre-provision
// a shared Bell pair and carry the classical correction bits a receiving
// node needs to complete a protocol.
//
// No-cloning means a channel never "copies" a qubit: ShareBellPair
// generates a fresh entangled resource, it does not duplicate an existing
// one. Migrating an existing qubit between nodes is Teleport, built on top
// of a QuantumChannel.
type QuantumChannel interface {
	NodeA() string
	NodeB() string

	// ShareBellPair pre-provisions a fresh Bell pair and returns it.
	ShareBellPair() (*BellPair, error)

	// SendClassical transmits classical bits from one end of the channel to
	// the other. from must be NodeA() or NodeB().
	SendClassical(from string, bits []int) error

	// RecvClassical blocks until classical bits addressed to "to" have
	// arrived, then returns them. to must be NodeA() or NodeB().
	RecvClassical(to string) ([]int, error)
}

// LocalChannel is the reference QuantumChannel implementation: both ends
// live in the same process — as they do in the current single-node LinLang
// runtime — so classical bits travel through an in-memory queue rather than
// a real network link. It is the quantum analogue of store.MemoryStore.
type LocalChannel struct {
	nodeA, nodeB string
	qubitSpace   *HilbertSpace

	mu      sync.Mutex
	inboxes map[string][][]int // destination node -> queued bit messages
}

// NewLocalChannel creates a channel between nodeA and nodeB that shares
// Bell pairs over qubitSpace (which must have Dim == 2).
func NewLocalChannel(nodeA, nodeB string, qubitSpace *HilbertSpace) *LocalChannel {
	return &LocalChannel{
		nodeA:      nodeA,
		nodeB:      nodeB,
		qubitSpace: qubitSpace,
		inboxes:    make(map[string][][]int),
	}
}

func (c *LocalChannel) NodeA() string { return c.nodeA }
func (c *LocalChannel) NodeB() string { return c.nodeB }

func (c *LocalChannel) ShareBellPair() (*BellPair, error) {
	if c.qubitSpace.Dim != 2 {
		return nil, fmt.Errorf("local channel: se esperaba un qubit (dim=2), dim=%d", c.qubitSpace.Dim)
	}
	pairSpace := NewHilbertSpace(c.qubitSpace.Name+"_bell", 4)
	inv := complex(1/math.Sqrt2, 0)
	bell := NewQuantumState(pairSpace, []complex128{inv, 0, 0, inv})
	return &BellPair{Space: pairSpace, State: bell, QubitA: 0, QubitB: 1}, nil
}

func (c *LocalChannel) other(node string) (string, error) {
	switch node {
	case c.nodeA:
		return c.nodeB, nil
	case c.nodeB:
		return c.nodeA, nil
	default:
		return "", fmt.Errorf("local channel: nodo desconocido '%s' (esperado '%s' o '%s')", node, c.nodeA, c.nodeB)
	}
}

func (c *LocalChannel) SendClassical(from string, bits []int) error {
	to, err := c.other(from)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.inboxes[to] = append(c.inboxes[to], bits)
	return nil
}

func (c *LocalChannel) RecvClassical(to string) ([]int, error) {
	if _, err := c.other(to); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	queue := c.inboxes[to]
	if len(queue) == 0 {
		return nil, fmt.Errorf("local channel: no hay mensajes clásicos pendientes para '%s'", to)
	}
	bits := queue[0]
	c.inboxes[to] = queue[1:]
	return bits, nil
}

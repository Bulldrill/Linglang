package core

import (
	"fmt"
	"sync"
)

// NodeID identifies a participant in the distributed LinLang/Q runtime.
type NodeID string

// DistributedHilbertSpace is a HilbertSpace annotated with which node
// currently hosts its amplitudes.
type DistributedHilbertSpace struct {
	*HilbertSpace
	Owner NodeID
}

// HilbertRegistry is the distributed analogue of a single-process
// Runtime's HilbertSpaces map: a directory of which node currently owns
// each named Hilbert space across the whole cluster. It lets the runtime
// decide, for any operation on a given space, whether it can execute
// locally or needs a QuantumChannel (#15) to the owning node — the
// bookkeeping layer Teleport (#14) needs to know *where* to migrate a
// qubit to/from.
type HilbertRegistry struct {
	mu     sync.RWMutex
	spaces map[string]*DistributedHilbertSpace
}

func NewHilbertRegistry() *HilbertRegistry {
	return &HilbertRegistry{spaces: make(map[string]*DistributedHilbertSpace)}
}

// Register declares that space is currently hosted on owner.
func (r *HilbertRegistry) Register(space *HilbertSpace, owner NodeID) *DistributedHilbertSpace {
	r.mu.Lock()
	defer r.mu.Unlock()
	d := &DistributedHilbertSpace{HilbertSpace: space, Owner: owner}
	r.spaces[space.Name] = d
	return d
}

// Lookup returns the registered entry for name, if any.
func (r *HilbertRegistry) Lookup(name string) (*DistributedHilbertSpace, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.spaces[name]
	return d, ok
}

// Owner returns which node currently hosts the space named name.
func (r *HilbertRegistry) Owner(name string) (NodeID, error) {
	d, ok := r.Lookup(name)
	if !ok {
		return "", fmt.Errorf("hilbert registry: espacio '%s' no registrado", name)
	}
	return d.Owner, nil
}

// Transfer updates ownership after a migration (e.g. following a
// successful Teleport) without altering the space's dimension or identity
// — only its location in the cluster changes.
func (r *HilbertRegistry) Transfer(name string, newOwner NodeID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.spaces[name]
	if !ok {
		return fmt.Errorf("hilbert registry: espacio '%s' no registrado", name)
	}
	d.Owner = newOwner
	return nil
}

// Local reports whether name's current owner is node — the question every
// operation on a distributed qubit must answer before deciding whether it
// can run in-process or needs a QuantumChannel to Owner(name).
func (r *HilbertRegistry) Local(name string, node NodeID) (bool, error) {
	owner, err := r.Owner(name)
	if err != nil {
		return false, err
	}
	return owner == node, nil
}

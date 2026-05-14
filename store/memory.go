package store

import (
	"sync"

	"linlang-go/core"
)

// MemoryStore es un backend en memoria, thread-safe.
// Útil para tests, demos y ejecución sin persistencia.
// DSN: memory://
type MemoryStore struct {
	mu   sync.RWMutex
	data map[string]map[float64][]float64 // space → id → values
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{data: make(map[string]map[float64][]float64)}
}

func (m *MemoryStore) Upsert(space *core.Space, values []float64) error {
	id := values[0]
	cp := make([]float64, len(values))
	copy(cp, values)

	m.mu.Lock()
	if m.data[space.Name] == nil {
		m.data[space.Name] = make(map[float64][]float64)
	}
	m.data[space.Name][id] = cp
	m.mu.Unlock()
	return nil
}

func (m *MemoryStore) Query(space *core.Space, filter func([]float64) bool) ([]*core.Vector, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*core.Vector
	for _, vals := range m.data[space.Name] {
		if filter == nil || filter(vals) {
			cp := make([]float64, len(vals))
			copy(cp, vals)
			result = append(result, core.NewVector(space, cp))
		}
	}
	return result, nil
}

func (m *MemoryStore) Delete(space *core.Space, id float64) error {
	m.mu.Lock()
	if m.data[space.Name] != nil {
		delete(m.data[space.Name], id)
	}
	m.mu.Unlock()
	return nil
}

func (m *MemoryStore) Close() error { return nil }

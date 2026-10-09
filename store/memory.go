package store

import (
	"sync"

	"linlang-go/core"
)

// storedRow is one persisted vector's data: numeric values plus any
// String-typed dimension values (issue #24).
type storedRow struct {
	values []float64
	strs   map[string]string
}

// MemoryStore es un backend en memoria, thread-safe.
// Útil para tests, demos y ejecución sin persistencia.
// DSN: memory://
type MemoryStore struct {
	mu   sync.RWMutex
	data map[string]map[float64]*storedRow // space → id → row
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{data: make(map[string]map[float64]*storedRow)}
}

func (m *MemoryStore) Upsert(space *core.Space, values []float64, strs map[string]string) error {
	id := values[0]
	cp := make([]float64, len(values))
	copy(cp, values)
	cpStrs := make(map[string]string, len(strs))
	for k, v := range strs {
		cpStrs[k] = v
	}

	m.mu.Lock()
	if m.data[space.Name] == nil {
		m.data[space.Name] = make(map[float64]*storedRow)
	}
	m.data[space.Name][id] = &storedRow{values: cp, strs: cpStrs}
	m.mu.Unlock()
	return nil
}

func (m *MemoryStore) Query(space *core.Space, filter func([]float64) bool) ([]*core.Vector, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*core.Vector
	for _, row := range m.data[space.Name] {
		if filter == nil || filter(row.values) {
			cp := make([]float64, len(row.values))
			copy(cp, row.values)
			v := core.NewVector(space, cp)
			for dim, val := range row.strs {
				v.SetString(dim, val)
			}
			result = append(result, v)
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

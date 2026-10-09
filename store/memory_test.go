package store

import (
	"testing"

	"linlang-go/core"
)

func TestMemoryStoreRoundTripsStringDimensions(t *testing.T) {
	space := core.NewTypedSpace("Tarea", []string{"id", "titulo", "prioridad"},
		[]core.DimType{core.Real, core.String, core.Real})

	s := NewMemoryStore()
	if err := s.Upsert(space, []float64{1, 0, 3}, map[string]string{"titulo": "comprar pan"}); err != nil {
		t.Fatalf("Upsert error: %v", err)
	}

	results, err := s.Query(space, nil)
	if err != nil {
		t.Fatalf("Query error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	title, ok := results[0].GetString("titulo")
	if !ok || title != "comprar pan" {
		t.Fatalf("expected titulo='comprar pan' after round-trip, got %q ok=%v", title, ok)
	}
	if results[0].Values[0] != 1 || results[0].Values[2] != 3 {
		t.Fatalf("expected Real dimensions to round-trip too, got %v", results[0].Values)
	}
}

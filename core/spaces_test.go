package core

import "testing"

func TestSpaceDimTypeDefaultsToReal(t *testing.T) {
	s := NewSpace("Tarea", []string{"id", "prioridad"})
	if s.DimType("id") != Real {
		t.Fatalf("expected Real by default, got %v", s.DimType("id"))
	}
	if s.DimType("desconocida") != Real {
		t.Fatalf("expected Real for an unknown dimension, got %v", s.DimType("desconocida"))
	}
}

func TestNewTypedSpaceTracksStringDims(t *testing.T) {
	s := NewTypedSpace("Tarea", []string{"id", "titulo", "prioridad"}, []DimType{Real, String, Real})
	if s.DimType("titulo") != String {
		t.Fatalf("expected titulo to be String, got %v", s.DimType("titulo"))
	}
	if s.DimType("id") != Real || s.DimType("prioridad") != Real {
		t.Fatal("expected id and prioridad to remain Real")
	}
}

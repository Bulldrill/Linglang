package core

import "testing"

func TestVectorStringDimensionRoundTrip(t *testing.T) {
	sp := NewTypedSpace("Tarea", []string{"id", "titulo", "prioridad"}, []DimType{Real, String, Real})
	v := NewVector(sp, []float64{1, 0, 3})
	v.SetString("titulo", "comprar pan")

	s, ok := v.GetString("titulo")
	if !ok || s != "comprar pan" {
		t.Fatalf("expected GetString('titulo')='comprar pan', got %q ok=%v", s, ok)
	}
	if _, ok := v.GetString("inexistente"); ok {
		t.Fatal("expected ok=false for a dimension never set")
	}
}

func TestVectorDisplayMixesStringsAndNumbers(t *testing.T) {
	sp := NewTypedSpace("Tarea", []string{"id", "titulo", "prioridad"}, []DimType{Real, String, Real})
	v := NewVector(sp, []float64{1, 0, 3})
	v.SetString("titulo", "comprar pan")

	display := v.Display()
	if display[0] != 1.0 || display[2] != 3.0 {
		t.Fatalf("expected Real slots as float64, got %v", display)
	}
	if display[1] != "comprar pan" {
		t.Fatalf("expected String slot as string 'comprar pan', got %v (%T)", display[1], display[1])
	}
}

func TestVectorArithmeticIgnoresStringSlots(t *testing.T) {
	sp := NewTypedSpace("Tarea", []string{"id", "titulo", "prioridad"}, []DimType{Real, String, Real})
	v := NewVector(sp, []float64{1, 0, 3})
	v.SetString("titulo", "comprar pan")

	// norm() must only see the Real slots (id=1, prioridad=3): sqrt(1+9).
	want := 3.1622776601683795
	if got := v.Norm(); got < want-1e-9 || got > want+1e-9 {
		t.Fatalf("expected Norm()=%.9f ignoring the String slot, got %.9f", want, got)
	}
}

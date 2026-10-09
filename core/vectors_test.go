package core

import (
	"math"
	"testing"
)

func TestVectorAdd(t *testing.T) {
	sp := NewSpace("Personas", []string{"edad", "ingresos"})
	v1 := NewVector(sp, []float64{25, 40000})
	v2 := NewVector(sp, []float64{30, 55000})

	sum, err := v1.Add(v2)
	if err != nil {
		t.Fatalf("Add error: %v", err)
	}
	if sum.Values[0] != 55 || sum.Values[1] != 95000 {
		t.Fatalf("unexpected sum: %v", sum.Values)
	}
}

func TestVectorAddRejectsIncompatibleSpaces(t *testing.T) {
	sp1 := NewSpace("Personas", []string{"edad"})
	sp2 := NewSpace("Mascotas", []string{"edad"})
	v1 := NewVector(sp1, []float64{25})
	v2 := NewVector(sp2, []float64{3})

	if _, err := v1.Add(v2); err == nil {
		t.Fatal("expected an error adding vectors from different spaces")
	}
}

func TestVectorDot(t *testing.T) {
	sp := NewSpace("V", []string{"a", "b"})
	v1 := NewVector(sp, []float64{1, 2})
	v2 := NewVector(sp, []float64{3, 4})
	if got := v1.Dot(v2); got != 11 {
		t.Fatalf("Dot = %v, want 11", got)
	}
}

func TestVectorScale(t *testing.T) {
	sp := NewSpace("V", []string{"a", "b"})
	v := NewVector(sp, []float64{2, 3})
	scaled := v.Scale(2.5)
	if scaled.Values[0] != 5 || scaled.Values[1] != 7.5 {
		t.Fatalf("Scale(2.5) = %v, want [5 7.5]", scaled.Values)
	}
}

func TestVectorNorm(t *testing.T) {
	sp := NewSpace("V", []string{"a", "b"})
	v := NewVector(sp, []float64{3, 4})
	if got := v.Norm(); math.Abs(got-5) > 1e-9 {
		t.Fatalf("Norm = %v, want 5", got)
	}
}

func TestVectorProjectKeepsMatchingDimsAndZerosTheRest(t *testing.T) {
	src := NewSpace("Personas", []string{"energia", "ingresos"})
	v := NewVector(src, []float64{0.8, 40000})

	target := NewSpace("Perfil", []string{"energia", "altura"})
	proj, err := v.Project(target)
	if err != nil {
		t.Fatalf("Project error: %v", err)
	}
	if proj.Values[0] != 0.8 {
		t.Fatalf("expected matching dim 'energia' to carry over, got %v", proj.Values[0])
	}
	if proj.Values[1] != 0 {
		t.Fatalf("expected non-matching dim 'altura' to default to 0, got %v", proj.Values[1])
	}
}

func TestVectorGetPanicsOnUnknownDimension(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected Get to panic for an unknown dimension")
		}
	}()
	sp := NewSpace("V", []string{"a"})
	v := NewVector(sp, []float64{1})
	v.Get("b")
}

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

package core

import "testing"

func TestTransformApplyInvokesFunction(t *testing.T) {
	d1 := NewSpace("Personas", []string{"energia"})
	d2 := NewSpace("Mascotas", []string{"edad"})
	cod := NewSpace("Adopciones", []string{"compat"})

	tx := NewTransform("adoptar", d1, d2, cod, func(p, m *Vector) *Vector {
		return NewVector(cod, []float64{p.Values[0] * m.Values[0]})
	})

	p := NewVector(d1, []float64{0.8})
	m := NewVector(d2, []float64{5})

	result := tx.Apply(p, m)
	if result.Space != cod {
		t.Fatalf("expected result in codomain %v, got %v", cod, result.Space)
	}
	if want := 4.0; result.Values[0] != want {
		t.Fatalf("expected compat=%v, got %v", want, result.Values[0])
	}
}

func TestNewTransformFieldsAreSet(t *testing.T) {
	d1 := NewSpace("A", []string{"x"})
	d2 := NewSpace("B", []string{"y"})
	cod := NewSpace("C", []string{"z"})
	tx := NewTransform("f", d1, d2, cod, func(p, m *Vector) *Vector { return p })

	if tx.Name != "f" || tx.Domain1 != d1 || tx.Domain2 != d2 || tx.Codomain != cod {
		t.Fatalf("NewTransform did not populate fields correctly: %+v", tx)
	}
}

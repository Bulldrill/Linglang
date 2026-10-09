package core

import "testing"

func evalFloat(t *testing.T, expr string, p, m *Vector) float64 {
	t.Helper()
	return ParseExpr(expr).Eval(p, m)
}

func TestParseExprArithmeticPrecedence(t *testing.T) {
	cases := map[string]float64{
		"3 + 4 * 2":   11,
		"(3 + 4) * 2": 14,
		"10 / 2 - 1":  4,
		"-5 + 2":      -3,
		"2 * -3":      -6,
	}
	for expr, want := range cases {
		if got := evalFloat(t, expr, nil, nil); got != want {
			t.Errorf("ParseExpr(%q).Eval() = %v, want %v", expr, got, want)
		}
	}
}

func TestParseExprDivisionByZeroIsZero(t *testing.T) {
	if got := evalFloat(t, "5 / 0", nil, nil); got != 0 {
		t.Fatalf("expected division by zero to evaluate to 0, got %v", got)
	}
}

func TestParseExprFieldAccess(t *testing.T) {
	sp := NewSpace("Personas", []string{"energia"})
	p := NewVector(sp, []float64{0.8})
	sp2 := NewSpace("Mascotas", []string{"edad"})
	m := NewVector(sp2, []float64{5})

	got := evalFloat(t, "p.energia * m.edad / 10", p, m)
	want := 0.8 * 5 / 10
	if got != want {
		t.Fatalf("p.energia * m.edad / 10 = %v, want %v", got, want)
	}
}

func TestParseExprFieldAccessOnNilVectorIsZero(t *testing.T) {
	if got := evalFloat(t, "p.energia", nil, nil); got != 0 {
		t.Fatalf("expected p.energia on nil p to be 0, got %v", got)
	}
}

func TestFieldExprIdPrefersSpaceDimensionOverInternalCounter(t *testing.T) {
	// Space declares its own "id" dimension: p.id must read that value,
	// not the auto-increment Vector.ID (regression test for 4a85000).
	sp := NewSpace("Tarea", []string{"id", "prioridad"})
	p := NewVector(sp, []float64{42, 3})

	if got := evalFloat(t, "p.id", p, nil); got != 42 {
		t.Fatalf("p.id = %v, want 42 (the declared dimension, not v.ID=%d)", got, p.ID)
	}
}

func TestFieldExprIdFallsBackToInternalCounterWhenNoSuchDimension(t *testing.T) {
	sp := NewSpace("Personas", []string{"energia"}) // no "id" dimension declared
	p := NewVector(sp, []float64{0.8})

	if got := evalFloat(t, "p.id", p, nil); got != float64(p.ID) {
		t.Fatalf("p.id = %v, want the internal counter %d", got, p.ID)
	}
}

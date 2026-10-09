package core

import "testing"

func TestConditionalEvaluateRunsActionWhenTrue(t *testing.T) {
	ran := false
	cond := NewConditional("x", func() float64 { return 5 }, func(v float64) bool { return v > 3 },
		func() { ran = true })

	cond.Evaluate()
	if !ran {
		t.Fatal("expected Action to run when Condition is satisfied")
	}
}

func TestConditionalEvaluateSkipsActionWhenFalse(t *testing.T) {
	ran := false
	cond := NewConditional("x", func() float64 { return 2 }, func(v float64) bool { return v > 3 },
		func() { ran = true })

	cond.Evaluate()
	if ran {
		t.Fatal("expected Action NOT to run when Condition is not satisfied")
	}
}

func TestNewVectorConditionalReadsVectorDimension(t *testing.T) {
	sp := NewSpace("Tarea", []string{"id", "prioridad"})
	v := NewVector(sp, []float64{1, 3})
	ran := false

	cond := NewVectorConditional(v, "prioridad", func(x float64) bool { return x == 3 }, func() { ran = true })
	if cond.Label != "Tarea.prioridad" {
		t.Fatalf("expected label 'Tarea.prioridad', got %q", cond.Label)
	}
	cond.Evaluate()
	if !ran {
		t.Fatal("expected Action to run: prioridad is 3")
	}
}

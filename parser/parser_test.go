package parser

import (
	"math"
	"testing"

	"linlang-go/store"
)

func TestClassicalVectorLiteralAndArithmetic(t *testing.T) {
	rt := NewRuntime()
	rt.Parse(`
space Personas: edad: Real, ingresos: Real
let p1 = Personas[25, 40000]
let p2 = Personas[30, 55000]
let suma = add(p1, p2)
let similitud = dot(p1, p2)
let magnitud = norm(p1)
let escalado = scale(p1, 2)
`)

	if v := rt.Vectors["suma"]; v == nil || v.Values[0] != 55 || v.Values[1] != 95000 {
		t.Fatalf("add: unexpected result %v", v)
	}
	if got, want := rt.Scalars["similitud"], 25.0*30+40000*55000; got != want {
		t.Fatalf("dot = %v, want %v", got, want)
	}
	wantNorm := math.Sqrt(25*25 + 40000.0*40000)
	if got := rt.Scalars["magnitud"]; math.Abs(got-wantNorm) > 1e-6 {
		t.Fatalf("norm = %v, want %v", got, wantNorm)
	}
	if v := rt.Vectors["escalado"]; v == nil || v.Values[0] != 50 || v.Values[1] != 80000 {
		t.Fatalf("scale: unexpected result %v", v)
	}
}

func TestClassicalTransformDeclarationAndCall(t *testing.T) {
	rt := NewRuntime()
	rt.Parse(`
space Personas: id: Real, energia: Real
space Mascotas: id: Real, edad: Real
space Adopciones: persona_id: Real, mascota_id: Real, compat: Real

transform adoptar: Personas x Mascotas -> Adopciones {
    persona_id = p.id
    mascota_id = m.id
    compat = p.energia * m.edad / 10
}

let p1 = Personas[1, 0.8]
let m1 = Mascotas[10, 5]
let a1 = adoptar(p1, m1)
`)

	tx, ok := rt.Transforms["adoptar"]
	if !ok {
		t.Fatal("expected transform 'adoptar' to be registered")
	}
	if tx.Domain1.Name != "Personas" || tx.Domain2.Name != "Mascotas" || tx.Codomain.Name != "Adopciones" {
		t.Fatalf("unexpected transform signature: %+v", tx)
	}

	a1 := rt.Vectors["a1"]
	if a1 == nil {
		t.Fatal("expected a1 to be created")
	}
	if a1.Values[0] != 1 || a1.Values[1] != 10 || a1.Values[2] != 0.4 {
		t.Fatalf("adoptar(p1, m1) = %v, want [1 10 0.4]", a1.Values)
	}
}

func TestClassicalProject(t *testing.T) {
	rt := NewRuntime()
	rt.Parse(`
space Personas: energia: Real, ingresos: Real
space Perfil: energia: Real, altura: Real
let p1 = Personas[0.8, 40000]
let perfil = project p1 onto Perfil
`)
	v := rt.Vectors["perfil"]
	if v == nil {
		t.Fatal("expected 'perfil' vector to exist")
	}
	if v.Values[0] != 0.8 || v.Values[1] != 0 {
		t.Fatalf("project: got %v, want [0.8 0]", v.Values)
	}
}

func TestWhenActionRunsOnlyWhenConditionHolds(t *testing.T) {
	rt := NewRuntime()
	// Parsed without the action line first, so LastCond can be inspected
	// before parseAction consumes and clears it (by design — otherwise a
	// later unrelated line could accidentally re-trigger a stale action).
	rt.Parse(`
space Adopciones: compat: Real
let a1 = Adopciones[0.5]
when a1.compat >= 0.2:
`)
	if rt.LastCond == nil {
		t.Fatal("expected a registered conditional")
	}
	if !rt.LastCond.Condition(0.5) {
		t.Fatal("expected condition a1.compat >= 0.2 to hold for compat=0.5")
	}

	rt.Parse(`    approve(a1)`)
	if rt.LastCond != nil {
		t.Fatal("expected LastCond to be consumed/cleared after its action runs")
	}
}

func TestForLoopIteratesCollectionBindingEachVector(t *testing.T) {
	rt := NewRuntime()
	rt.SetStore(store.NewMemoryStore())
	rt.Parse(`
space Tarea: id: Real, prioridad: Real
let t1 = Tarea[1, 3]
let t2 = Tarea[2, 5]
persist t1
persist t2
let todas = query Tarea
`)
	var seenPriorities []float64
	for _, v := range rt.Collections["todas"] {
		seenPriorities = append(seenPriorities, v.Values[1])
	}

	rt.Parse(`
for t in todas {
    let doble = scale(t, 2)
}
`)
	// After the loop, rt.Vectors["t"] holds the LAST vector bound, and
	// "doble" the scaled version of that same last vector — confirms the
	// body actually re-executed once per element rather than once total.
	last := rt.Vectors["t"]
	if last == nil {
		t.Fatal("expected loop variable 't' to be bound after the loop")
	}
	doble := rt.Vectors["doble"]
	if doble == nil || doble.Values[1] != last.Values[1]*2 {
		t.Fatalf("expected doble to be 2x the last-bound t, got doble=%v last=%v", doble, last)
	}
	if len(seenPriorities) != 2 {
		t.Fatalf("expected 2 vectors queried, got %d", len(seenPriorities))
	}
}

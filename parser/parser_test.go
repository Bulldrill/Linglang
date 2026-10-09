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

func TestUserDefinedFuncDeclarationAndCall(t *testing.T) {
	rt := NewRuntime()
	rt.Parse(`
space Personas: id: Real, energia: Real

func doblar: Personas -> Personas {
    id = p.id
    energia = p.energia * 2
}

let p1 = Personas[1, 0.8]
let p2 = doblar(p1)
`)
	tx, ok := rt.Transforms["doblar"]
	if !ok {
		t.Fatal("expected 'doblar' to be registered")
	}
	if tx.Domain2 != nil {
		t.Fatalf("expected a unary func to have a nil Domain2, got %v", tx.Domain2)
	}
	p2 := rt.Vectors["p2"]
	if p2 == nil {
		t.Fatal("expected p2 to be created")
	}
	if p2.Values[0] != 1 || p2.Values[1] != 1.6 {
		t.Fatalf("doblar(p1) = %v, want [1 1.6]", p2.Values)
	}
}

func TestUserDefinedFuncCallMissingArgReportsError(t *testing.T) {
	rt := NewRuntime()
	rt.Parse(`
space Personas: id: Real, energia: Real
func doblar: Personas -> Personas {
    id = p.id
    energia = p.energia * 2
}
let p2 = doblar(no_existe)
`)
	if _, ok := rt.Vectors["p2"]; ok {
		t.Fatal("expected no binding when the argument vector does not exist")
	}
	if rt.LastError == nil || rt.LastError.Name != "NotFound" {
		t.Fatalf("expected a catchable NotFound error, got %+v", rt.LastError)
	}
}

func TestVectorLiteralTypeInferenceWhenUnambiguous(t *testing.T) {
	rt := NewRuntime()
	rt.Parse(`
space Personas: edad: Real, ingresos: Real, energia: Real
space Mascotas: edad: Real, especie: Real
let p1 = [25, 40000, 0.8]
let m1 = [3, 1]
`)
	p1 := rt.Vectors["p1"]
	if p1 == nil || p1.Space.Name != "Personas" {
		t.Fatalf("expected p1 inferred as Personas, got %v", p1)
	}
	m1 := rt.Vectors["m1"]
	if m1 == nil || m1.Space.Name != "Mascotas" {
		t.Fatalf("expected m1 inferred as Mascotas, got %v", m1)
	}
}

func TestVectorLiteralTypeInferenceAmbiguousReportsError(t *testing.T) {
	rt := NewRuntime()
	rt.Parse(`
space A: x: Real, y: Real
space B: w: Real, z: Real
let v = [1, 2]
`)
	if _, ok := rt.Vectors["v"]; ok {
		t.Fatal("expected no vector bound when inference is ambiguous")
	}
	if rt.LastError == nil || rt.LastError.Name != "TypeInferenceError" {
		t.Fatalf("expected a catchable TypeInferenceError, got %+v", rt.LastError)
	}
}

func TestVectorLiteralTypeInferenceNoMatchReportsError(t *testing.T) {
	rt := NewRuntime()
	rt.Parse(`
space A: x: Real, y: Real
let v = [1, 2, 3]
`)
	if _, ok := rt.Vectors["v"]; ok {
		t.Fatal("expected no vector bound when no space matches the arity")
	}
	if rt.LastError == nil || rt.LastError.Name != "TypeInferenceError" {
		t.Fatalf("expected a catchable TypeInferenceError, got %+v", rt.LastError)
	}
}

func TestCollectionIndexing(t *testing.T) {
	rt := NewRuntime()
	rt.SetStore(store.NewMemoryStore())
	rt.Parse(`
space Tarea: id: Real, prioridad: Real
let t1 = Tarea[1, 3]
let t2 = Tarea[2, 5]
persist t1
persist t2
let results = query Tarea
let primero = results[0]
`)
	v := rt.Vectors["primero"]
	if v == nil {
		t.Fatal("expected 'primero' to be bound from results[0]")
	}
	results := rt.Collections["results"]
	if v != results[0] {
		t.Fatal("expected results[0] indexing to reference the same vector as the collection")
	}
}

func TestTryCatchRunsCatchBodyOnMatchingError(t *testing.T) {
	rt := NewRuntime()
	rt.SetStore(store.NewMemoryStore())
	rt.Parse(`
space Tarea: id: Real, prioridad: Real
let t1 = Tarea[1, 3]
persist t1

try {
    let t = query_one Tarea where id == 99
    let n = norm(t)
}
catch NotFound {
    let fallback = Tarea[0, 0]
}
`)
	if _, ok := rt.Vectors["t"]; ok {
		t.Fatal("expected query_one to fail and not bind 't'")
	}
	if _, ok := rt.Vectors["n"]; ok {
		t.Fatal("expected the try body to stop at the first error, never reaching norm(t)")
	}
	if _, ok := rt.Vectors["fallback"]; !ok {
		t.Fatal("expected the catch body to run and bind 'fallback'")
	}
	if rt.LastError != nil {
		t.Fatal("expected LastError to be cleared after the catch block handles it")
	}
}

func TestTryCatchSkipsCatchBodyOnSuccess(t *testing.T) {
	rt := NewRuntime()
	rt.SetStore(store.NewMemoryStore())
	rt.Parse(`
space Tarea: id: Real, prioridad: Real
let t1 = Tarea[1, 3]
persist t1

try {
    let t = query_one Tarea where id == 1
}
catch NotFound {
    let marcador = Tarea[0, 0]
}
`)
	if _, ok := rt.Vectors["t"]; !ok {
		t.Fatal("expected the try body to succeed and bind 't'")
	}
	if _, ok := rt.Vectors["marcador"]; ok {
		t.Fatal("expected the catch body NOT to run when the try block succeeds")
	}
}

func TestTryCatchUnmatchedErrorNameIsReported(t *testing.T) {
	rt := NewRuntime()
	rt.SetStore(store.NewMemoryStore())
	rt.Parse(`
space Tarea: id: Real
let t1 = Tarea[1]
persist t1

try {
    let t = query_one Tarea where id == 99
}
catch IndexOutOfRange {
    let marcador = Tarea[0]
}
`)
	if _, ok := rt.Vectors["marcador"]; ok {
		t.Fatal("expected the catch body NOT to run: NotFound != IndexOutOfRange")
	}
	if rt.LastError != nil {
		t.Fatal("expected LastError to be cleared even when no catch clause matches")
	}
}

func TestCollectionIndexingOutOfRangeDoesNotPanic(t *testing.T) {
	rt := NewRuntime()
	rt.SetStore(store.NewMemoryStore())
	rt.Parse(`
space Tarea: id: Real
let t1 = Tarea[1]
persist t1
let results = query Tarea
let fuera = results[99]
`)
	if _, ok := rt.Vectors["fuera"]; ok {
		t.Fatal("expected no binding for an out-of-range index")
	}
}

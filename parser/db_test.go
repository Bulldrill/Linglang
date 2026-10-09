package parser

import (
	"testing"

	"linlang-go/store"
)

func TestPersistQueryDropRoundTrip(t *testing.T) {
	rt := NewRuntime()
	rt.SetStore(store.NewMemoryStore())
	rt.Parse(`
space Tarea: id: Real, prioridad: Real, estado: Real
let t1 = Tarea[1, 3, 0]
let t2 = Tarea[2, 1, 0]
persist t1
persist t2
let todas = query Tarea
let t1_de_nuevo = query_one Tarea where id == 1
`)
	if len(rt.Collections["todas"]) != 2 {
		t.Fatalf("expected 2 persisted vectors, got %d", len(rt.Collections["todas"]))
	}
	if v := rt.Vectors["t1_de_nuevo"]; v == nil || v.Values[0] != 1 {
		t.Fatalf("query_one: expected to find id==1, got %v", v)
	}

	rt.Parse(`drop t1`)
	rt.Parse(`let despues = query Tarea`)
	if len(rt.Collections["despues"]) != 1 {
		t.Fatalf("expected 1 vector after drop, got %d", len(rt.Collections["despues"]))
	}
}

func TestQueryMultiConditionAndOr(t *testing.T) {
	rt := NewRuntime()
	rt.SetStore(store.NewMemoryStore())
	rt.Parse(`
space Tarea: id: Real, prioridad: Real, estado: Real, categoria: Real
let t1 = Tarea[1, 3, 0, 1]
let t2 = Tarea[2, 1, 0, 2]
let t3 = Tarea[3, 3, 1, 1]
persist t1
persist t2
persist t3
let urgentes = query Tarea where prioridad == 3 and estado == 0
let mix = query Tarea where prioridad == 1 or categoria == 1
`)
	if len(rt.Collections["urgentes"]) != 1 {
		t.Fatalf("expected 1 result for 'prioridad==3 and estado==0', got %d", len(rt.Collections["urgentes"]))
	}
	if len(rt.Collections["mix"]) != 3 {
		t.Fatalf("expected 3 results for 'prioridad==1 or categoria==1', got %d", len(rt.Collections["mix"]))
	}
}

func TestFilterOverCollectionIncludingStringEquality(t *testing.T) {
	rt := NewRuntime()
	rt.SetStore(store.NewMemoryStore())
	rt.Parse(`
space Tarea: id: Real, titulo: String, prioridad: Real, categoria: String
let t1 = Tarea[1, "comprar pan", 3, "hogar"]
let t2 = Tarea[2, "terminar informe", 1, "trabajo"]
let t3 = Tarea[3, "lavar el auto", 2, "hogar"]
persist t1
persist t2
persist t3
let todas = query Tarea
let hogar = filter(todas, categoria == "hogar")
let urgentes = filter(todas, prioridad == 3)
`)
	if len(rt.Collections["hogar"]) != 2 {
		t.Fatalf("expected 2 results for categoria==\"hogar\", got %d", len(rt.Collections["hogar"]))
	}
	for _, v := range rt.Collections["hogar"] {
		if s, _ := v.GetString("categoria"); s != "hogar" {
			t.Fatalf("filter leaked a non-matching vector: categoria=%q", s)
		}
	}
	if len(rt.Collections["urgentes"]) != 1 {
		t.Fatalf("expected 1 result for prioridad==3, got %d", len(rt.Collections["urgentes"]))
	}
}

func TestMapAppliesTransformOverCollection(t *testing.T) {
	rt := NewRuntime()
	rt.SetStore(store.NewMemoryStore())
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
let p2 = Personas[2, 0.5]
let m1 = Mascotas[10, 5]
persist p1
persist p2
let personas = query Personas
let resultados = map(personas, adoptar, m1)
`)
	results := rt.Collections["resultados"]
	if len(results) != 2 {
		t.Fatalf("expected 2 mapped results, got %d", len(results))
	}
	for _, v := range results {
		if v.Space.Name != "Adopciones" {
			t.Fatalf("expected mapped vectors in codomain 'Adopciones', got %s", v.Space.Name)
		}
	}
}

func TestCountCollectionHelper(t *testing.T) {
	rt := NewRuntime()
	rt.SetStore(store.NewMemoryStore())
	rt.Parse(`
space Tarea: id: Real
let t1 = Tarea[1]
let t2 = Tarea[2]
persist t1
persist t2
let todas = query Tarea
`)
	n, ok := rt.countCollection("todas")
	if !ok || n != 2 {
		t.Fatalf("countCollection = %v, %v; want 2, true", n, ok)
	}
}

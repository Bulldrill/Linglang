package parser

import (
	"testing"

	"linlang-go/store"
)

// BenchmarkParseClassical exercises the line-by-line interpreter loop
// (ParseLine) over a representative classical program: space/transform
// declarations, vector literals, and transform application.
func BenchmarkParseClassical(b *testing.B) {
	code := `
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
`
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		NewRuntime().Parse(code)
	}
}

// BenchmarkParseQuantum exercises the quantum extension: gate
// auto-registration, ket/apply/measure.
func BenchmarkParseQuantum(b *testing.B) {
	code := `
hilbert Qubit: dim 2
let ket0 = ket(Qubit, 1, 0)
let plus = apply(H_Qubit, ket0)
let m = measure(plus)
`
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		NewRuntime().Parse(code)
	}
}

// BenchmarkPersistQuery exercises the persistence round trip against
// MemoryStore: the path every cmd/todo-server request takes.
func BenchmarkPersistQuery(b *testing.B) {
	code := `
space Tarea: id: Real, prioridad: Real, estado: Real, categoria: Real
let t1 = Tarea[1, 3, 0, 1]
persist t1
let t = query_one Tarea where id == 1
`
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		rt := NewRuntime()
		rt.SetStore(store.NewMemoryStore())
		rt.Parse(code)
	}
}

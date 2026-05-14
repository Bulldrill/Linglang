// parser/db.go
//
// Primitivas de base de datos nativas de LinLang.
//
// Nuevas instrucciones del lenguaje:
//
//	persist varName
//	    Guarda el vector en el backend configurado (upsert por id).
//
//	drop varName
//	    Elimina el vector del backend y lo borra del runtime.
//
//	let x = query Space
//	    Carga todos los vectores del espacio → rt.Collections["x"]
//
//	let x = query Space where dim OP value
//	    Carga vectores filtrados por condición algebraica.
//
//	let x = query_one Space where dim OP value
//	    Carga el primer vector que satisface la condición → rt.Vectors["x"]
//
// La conexión al backend se configura mediante LINLANG_DB y es
// completamente transparente para el programa .lin.
package parser

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"linlang-go/core"
	"linlang-go/store"
)

// ── persist ───────────────────────────────────────────────────────────────────

// Syntax: persist varName
func (rt *Runtime) parsePersist(line string) {
	varName := strings.TrimSpace(strings.TrimPrefix(line, "persist "))
	v := rt.Vectors[varName]
	if v == nil {
		fmt.Printf("[❌] persist: vector '%s' no encontrado\n", varName)
		return
	}
	if rt.Store == nil {
		fmt.Printf("[❌] persist: no hay store configurado (usa runtime.SetStore)\n")
		return
	}
	if err := rt.Store.Upsert(v.Space, v.Values); err != nil {
		fmt.Printf("[❌] persist %s: %v\n", varName, err)
		return
	}
	fmt.Printf("[💾] persist %s → '%s'  [%s=%.0f]\n",
		varName, v.Space.Name, v.Space.Dimensions[0], v.Values[0])
}

// ── drop ──────────────────────────────────────────────────────────────────────

// Syntax: drop varName
func (rt *Runtime) parseDrop(line string) {
	varName := strings.TrimSpace(strings.TrimPrefix(line, "drop "))
	v := rt.Vectors[varName]
	if v == nil {
		fmt.Printf("[❌] drop: vector '%s' no encontrado en el runtime\n", varName)
		return
	}
	if rt.Store == nil {
		fmt.Printf("[❌] drop: no hay store configurado\n")
		return
	}
	id := v.Values[0]
	if err := rt.Store.Delete(v.Space, id); err != nil {
		fmt.Printf("[❌] drop %s: %v\n", varName, err)
		return
	}
	delete(rt.Vectors, varName)
	fmt.Printf("[🗑️]  drop %s ← '%s'  [%s=%.0f]\n",
		varName, v.Space.Name, v.Space.Dimensions[0], id)
}

// ── query / query_one ─────────────────────────────────────────────────────────

// parseQuery handles both:
//
//	let x = query Space
//	let x = query Space where dim OP value
//
// Results go to rt.Collections[varName] (slice of vectors).
func (rt *Runtime) parseQuery(varName, rhs string) {
	space, filter, label, ok := rt.parseQueryExpr(rhs, "query ")
	if !ok {
		return
	}
	vectors, err := rt.Store.Query(space, filter)
	if err != nil {
		fmt.Printf("[❌] query %s: %v\n", space.Name, err)
		return
	}
	// Sort by first dimension (id) for deterministic output
	sort.Slice(vectors, func(i, j int) bool {
		return vectors[i].Values[0] < vectors[j].Values[0]
	})
	rt.Collections[varName] = vectors
	suffix := ""
	if label != "" {
		suffix = " where " + label
	}
	fmt.Printf("[🔍] %s = query %s%s  → %d vector(es)\n",
		varName, space.Name, suffix, len(vectors))
}

// parseQueryOne loads a single vector into rt.Vectors (not rt.Collections).
// Syntax: let x = query_one Space where dim OP value
func (rt *Runtime) parseQueryOne(varName, rhs string) {
	space, filter, label, ok := rt.parseQueryExpr(rhs, "query_one ")
	if !ok {
		return
	}
	vectors, err := rt.Store.Query(space, filter)
	if err != nil {
		fmt.Printf("[❌] query_one %s: %v\n", space.Name, err)
		return
	}
	if len(vectors) == 0 {
		fmt.Printf("[⚠️] query_one %s where %s → sin resultados\n", space.Name, label)
		return
	}
	rt.Vectors[varName] = vectors[0]
	fmt.Printf("[🔍] %s = query_one %s where %s  → %v\n",
		varName, space.Name, label, vectors[0].Values)
}

// parseQueryExpr is the shared parsing logic for query/query_one.
// Returns (space, filterFn, filterLabel, ok).
func (rt *Runtime) parseQueryExpr(rhs, prefix string) (*core.Space, func([]float64) bool, string, bool) {
	rhs = strings.TrimPrefix(rhs, prefix)

	// Split on " where "
	var spaceName, condExpr string
	if idx := strings.Index(rhs, " where "); idx >= 0 {
		spaceName = strings.TrimSpace(rhs[:idx])
		condExpr = strings.TrimSpace(rhs[idx+7:])
	} else {
		spaceName = strings.TrimSpace(rhs)
	}

	space := rt.Spaces[spaceName]
	if space == nil {
		fmt.Printf("[❌] query: espacio '%s' no encontrado\n", spaceName)
		return nil, nil, "", false
	}
	if rt.Store == nil {
		fmt.Printf("[❌] query: no hay store configurado\n")
		return nil, nil, "", false
	}

	if condExpr == "" {
		return space, nil, "", true
	}

	// Parse the condition: dim OP value
	op, before, after := extractOperator(condExpr)
	if op == "" {
		fmt.Printf("[⚠️] query: operador no reconocido en '%s'\n", condExpr)
		return space, nil, "", true // return all, ignore malformed condition
	}
	dimName := strings.TrimSpace(before)
	val, err := strconv.ParseFloat(strings.TrimSpace(after), 64)
	if err != nil {
		fmt.Printf("[⚠️] query: valor no numérico '%s'\n", after)
		return space, nil, "", true
	}

	// Find dimension index
	dimIdx := -1
	for i, d := range space.Dimensions {
		if d == dimName {
			dimIdx = i
			break
		}
	}
	if dimIdx < 0 {
		fmt.Printf("[⚠️] query: dimensión '%s' no existe en espacio '%s'\n", dimName, spaceName)
		return space, nil, "", true
	}

	filter := makeVecFilter(op, dimIdx, val)
	label := fmt.Sprintf("%s %s %.4g", dimName, op, val)
	return space, filter, label, true
}

// ── count ─────────────────────────────────────────────────────────────────────

// countCollection returns the size of a named collection.
// Used by resolveConditionLHS for: when count(results) > 0:
func (rt *Runtime) countCollection(name string) (float64, bool) {
	if coll, ok := rt.Collections[name]; ok {
		return float64(len(coll)), true
	}
	return 0, false
}

// ── printCollection ───────────────────────────────────────────────────────────

// printCollection prints all vectors in a named collection.
// Returns true if the name was a collection.
func (rt *Runtime) printCollection(name string) bool {
	coll, ok := rt.Collections[name]
	if !ok {
		return false
	}
	if len(coll) == 0 {
		fmt.Printf("[📌] %s = [] (colección vacía)\n", name)
		return true
	}
	fmt.Printf("[📌] %s  (%d vectores en '%s'):\n", name, len(coll), coll[0].Space.Name)
	for i, v := range coll {
		dims := v.Space.Dimensions
		fmt.Printf("      [%d]  ", i)
		for j, d := range dims {
			fmt.Printf("%s=%.4g", d, v.Values[j])
			if j < len(dims)-1 {
				fmt.Printf("  ")
			}
		}
		fmt.Println()
	}
	return true
}

// ── helpers ───────────────────────────────────────────────────────────────────

// makeVecFilter builds a filter predicate for a specific dimension index.
func makeVecFilter(op string, dimIdx int, val float64) func([]float64) bool {
	cond := makeCondition(op, val)
	return func(values []float64) bool {
		if dimIdx >= len(values) {
			return false
		}
		return cond(values[dimIdx])
	}
}

// SetStore attaches a persistence backend to the runtime.
// Once set, persist / drop / query work transparently in .lin programs.
func (rt *Runtime) SetStore(b store.Backend) {
	rt.Store = b
}

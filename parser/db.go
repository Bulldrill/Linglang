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
//	let x = query Space where dim1 OP v1 and dim2 OP v2 or dim3 OP v3
//	    Multi-condición (issue #26): "and" liga más fuerte que "or",
//	    como en lógica booleana ordinaria — "a and b or c" es (a and b) or c.
//
//	let x = query_one Space where dim OP value
//	    Carga el primer vector que satisface la condición → rt.Vectors["x"]
//
//	let x = filter(collection, dim OP value [and/or ...])
//	    Filtra una colección ya cargada en memoria (issue #25).
//
//	let x = map(collection, transformName, otherVector)
//	    Aplica un transform binario registrado a cada vector de la colección
//	    contra otherVector, produciendo una nueva colección (issue #25).
//
//	for v in collection { ... }
//	    Itera la colección, ligando v a cada vector por turno (issue #25).
//	    Ver parser.go: parseForDecl / finalizeFor.
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

	// Multi-condition: "and" binds tighter than "or" (issue #26), matching
	// ordinary boolean-logic precedence — "a and b or c and d" means
	// (a and b) or (c and d).
	filter, label, errMsg := buildMultiConditionFilter(space, condExpr)
	if errMsg != "" {
		fmt.Printf("[⚠️] query: %s\n", errMsg)
		return space, nil, "", true // return all, ignore malformed condition
	}
	return space, filter, label, true
}

// buildMultiConditionFilter parses a `where` clause combining one or more
// "dim OP value" comparisons with "and"/"or" into a single predicate. On
// success errMsg is "". On failure it names which clause was malformed and
// why, mirroring the specific diagnostics parseQueryExpr used to print
// inline before this supported more than one clause.
func buildMultiConditionFilter(space *core.Space, expr string) (filter func([]float64) bool, label string, errMsg string) {
	var orFns []func([]float64) bool
	var orLabels []string

	for _, group := range strings.Split(expr, " or ") {
		var andFns []func([]float64) bool
		var andLabels []string

		for _, clause := range strings.Split(group, " and ") {
			clause = strings.TrimSpace(clause)
			fn, clauseLabel, err := singleConditionFilter(space, clause)
			if err != "" {
				return nil, "", err
			}
			andFns = append(andFns, fn)
			andLabels = append(andLabels, clauseLabel)
		}

		orFns = append(orFns, allOf(andFns))
		orLabels = append(orLabels, strings.Join(andLabels, " and "))
	}

	return anyOf(orFns), strings.Join(orLabels, " or "), ""
}

// singleConditionFilter parses one "dim OP value" clause.
func singleConditionFilter(space *core.Space, clause string) (filter func([]float64) bool, label string, errMsg string) {
	op, before, after := extractOperator(clause)
	if op == "" {
		return nil, "", fmt.Sprintf("operador no reconocido en '%s'", clause)
	}
	dimName := strings.TrimSpace(before)
	val, err := strconv.ParseFloat(strings.TrimSpace(after), 64)
	if err != nil {
		return nil, "", fmt.Sprintf("valor no numérico '%s'", strings.TrimSpace(after))
	}

	dimIdx := -1
	for i, d := range space.Dimensions {
		if d == dimName {
			dimIdx = i
			break
		}
	}
	if dimIdx < 0 {
		return nil, "", fmt.Sprintf("dimensión '%s' no existe en espacio '%s'", dimName, space.Name)
	}

	return makeVecFilter(op, dimIdx, val), fmt.Sprintf("%s %s %.4g", dimName, op, val), ""
}

// allOf/anyOf combine per-clause filters into conjunctions/disjunctions.
func allOf(fns []func([]float64) bool) func([]float64) bool {
	return func(v []float64) bool {
		for _, f := range fns {
			if !f(v) {
				return false
			}
		}
		return true
	}
}

func anyOf(fns []func([]float64) bool) func([]float64) bool {
	return func(v []float64) bool {
		for _, f := range fns {
			if f(v) {
				return true
			}
		}
		return false
	}
}

// ── filter / map over in-memory collections (issue #25) ─────────────────────

// Syntax: let x = filter(collection, dim OP value [and/or ...])
// Filters an already-loaded collection in memory — unlike query's `where`,
// this never touches rt.Store, so it also works on collections built by
// map() or by a previous filter().
func (rt *Runtime) applyFilter(varName string, args []string) bool {
	if len(args) < 2 {
		fmt.Printf("[❌] filter: uso: filter(coleccion, dim OP valor)\n")
		return true
	}
	collName := strings.TrimSpace(args[0])
	coll, ok := rt.Collections[collName]
	if !ok {
		fmt.Printf("[❌] filter: colección '%s' no encontrada\n", collName)
		return true
	}
	if len(coll) == 0 {
		rt.Collections[varName] = []*core.Vector{}
		fmt.Printf("[✔️] %s = filter(%s, ...)  → 0 vector(es)\n", varName, collName)
		return true
	}

	condExpr := strings.TrimSpace(strings.Join(args[1:], ","))
	filterFn, label, errMsg := buildMultiConditionFilter(coll[0].Space, condExpr)
	if errMsg != "" {
		fmt.Printf("[❌] filter: %s\n", errMsg)
		return true
	}

	result := make([]*core.Vector, 0, len(coll))
	for _, v := range coll {
		if filterFn(v.Values) {
			result = append(result, v)
		}
	}
	rt.Collections[varName] = result
	fmt.Printf("[✔️] %s = filter(%s, %s)  → %d vector(es)\n", varName, collName, label, len(result))
	return true
}

// Syntax: let x = map(collection, transformName, otherVector)
// Applies a registered binary transform (transform Name: D1 x D2 -> Cod)
// elementwise over collection against the fixed otherVector, producing a
// new collection — "map" in the sense LinLang's transforms already use: a
// relation between two vectors, held constant on one side across the loop.
func (rt *Runtime) applyMap(varName string, args []string) bool {
	if len(args) < 3 {
		fmt.Printf("[❌] map: uso: map(coleccion, transformName, otroVector)\n")
		return true
	}
	collName := strings.TrimSpace(args[0])
	txName := strings.TrimSpace(args[1])
	otherName := strings.TrimSpace(args[2])

	coll, ok := rt.Collections[collName]
	if !ok {
		fmt.Printf("[❌] map: colección '%s' no encontrada\n", collName)
		return true
	}
	tx, ok := rt.Transforms[txName]
	if !ok {
		fmt.Printf("[❌] map: transform '%s' no registrado\n", txName)
		return true
	}
	other := rt.Vectors[otherName]
	if other == nil {
		fmt.Printf("[❌] map: vector '%s' no encontrado\n", otherName)
		return true
	}

	result := make([]*core.Vector, 0, len(coll))
	for _, v := range coll {
		result = append(result, tx.Apply(v, other))
	}
	rt.Collections[varName] = result
	fmt.Printf("[✔️] %s = map(%s, %s, %s)  → %d vector(es)\n", varName, collName, txName, otherName, len(result))
	return true
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

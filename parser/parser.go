package parser

import (
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"linlang-go/core"
	"linlang-go/store"
)

// ── Runtime ───────────────────────────────────────────────────────────────────

// Runtime holds the full interpreter state for a LinLang program.
type Runtime struct {
	// ── Classical fields ─────────────────────────────────────────────────────
	Spaces     map[string]*core.Space
	Vectors    map[string]*core.Vector
	Scalars    map[string]float64 // scalar results: dot, norm, quantum properties…
	Transforms map[string]*core.Transform
	LastCond   *core.Conditional

	// multi-line classical transform state
	inTransform bool
	pendingTx   *pendingTransform

	// ── Quantum extension ─────────────────────────────────────────────────────
	HilbertSpaces   map[string]*core.HilbertSpace
	QuantumStates   map[string]*core.QuantumState
	DensityMatrices map[string]*core.DensityMatrix
	Gates           map[string]*core.Gate
	Measurements    map[string]int // classical outcomes from measure()

	// Backend is the QuantumBackend used by apply()/measure()/teleport().
	// nil until first needed, at which point it is resolved from an
	// explicit `technology: <name>` directive or, failing that, from
	// LINLANG_QUANTUM_BACKEND (default: the local simulator). See
	// quantumBackend() in quantum.go.
	Backend core.QuantumBackend

	// Histograms holds shots(psi, n) results: outcome -> count, the
	// stochastic Born-rule sampling counterpart to the deterministic
	// Measurements map above (issue #10).
	Histograms map[string]map[int]int
	rng        *rand.Rand

	// multi-line gate declaration state
	inGate      bool
	pendingGate *pendingGateDecl

	// multi-line `for x in collection { ... }` loop state (issue #25)
	inFor      bool
	pendingFor *pendingForLoop

	// multi-line `try { } catch Name { }` state (issue #33)
	inTry         bool
	awaitingCatch bool
	inCatch       bool
	pendingTry    *pendingTryBlock

	// LastError is the most recent catchable runtime error, set by fail()
	// and consumed by finalizeTry(). nil outside of try-block execution.
	LastError *RuntimeError

	// ── Persistence ────────────────────────────────────────────────────────────
	Store       store.Backend             // nil = no persistence
	Collections map[string][]*core.Vector // results of query statements

	// included tracks paths already loaded by `include "..."` (issue #29),
	// so re-including a file (directly or via a cycle) is a silent no-op
	// instead of re-declaring every space/transform in it.
	included map[string]bool
}

// RuntimeError is a catchable runtime error (issue #33): Name is what a
// `catch Name { }` clause matches against, Message is the same diagnostic
// text that would otherwise just be printed with "[❌]".
type RuntimeError struct {
	Name    string
	Message string
}

// pendingTryBlock accumulates a try/catch statement across both its
// bodies until the catch block's closing '}'.
type pendingTryBlock struct {
	tryBody   []string
	catchName string
	catchBody []string
}

// pendingForLoop accumulates a `for` loop body until its closing '}'.
type pendingForLoop struct {
	varName  string
	collName string
	body     []string
}

// pendingTransform accumulates a transform declaration until its closing '}'.
type pendingTransform struct {
	name     string
	dom1     string
	dom2     string
	codomain string
	mappings map[string]core.Expr // output dimension → expression
}

// pendingGateDecl accumulates a gate declaration until its closing '}'.
type pendingGateDecl struct {
	name string
	dom  string
	cod  string
	rows [][]complex128
}

func NewRuntime() *Runtime {
	return &Runtime{
		Spaces:          map[string]*core.Space{},
		Vectors:         map[string]*core.Vector{},
		Scalars:         map[string]float64{},
		Transforms:      map[string]*core.Transform{},
		HilbertSpaces:   map[string]*core.HilbertSpace{},
		QuantumStates:   map[string]*core.QuantumState{},
		DensityMatrices: map[string]*core.DensityMatrix{},
		Gates:           map[string]*core.Gate{},
		Measurements:    map[string]int{},
		Collections:     map[string][]*core.Vector{},
		Histograms:      map[string]map[int]int{},
		rng:             rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// ── Top-level dispatch ────────────────────────────────────────────────────────

func (rt *Runtime) ParseLine(line string) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return
	}
	// Strip inline comments: everything from the first " #" onwards
	if idx := strings.Index(trimmed, " #"); idx >= 0 {
		trimmed = strings.TrimSpace(trimmed[:idx])
	}

	// ── Inside a classical transform body ───────────────────────────────────
	if rt.inTransform {
		if trimmed == "}" {
			rt.finalizeTransform()
			return
		}
		if idx := strings.Index(trimmed, "="); idx > 0 {
			dim := strings.TrimSpace(trimmed[:idx])
			expr := strings.TrimSpace(trimmed[idx+1:])
			rt.pendingTx.mappings[dim] = core.ParseExpr(expr)
		}
		return
	}

	// ── Inside a `try { }` body ───────────────────────────────────────────────
	if rt.inTry {
		if trimmed == "}" {
			rt.inTry = false
			rt.awaitingCatch = true
			return
		}
		rt.pendingTry.tryBody = append(rt.pendingTry.tryBody, trimmed)
		return
	}

	// ── Between `try { }` and `catch Name { }` ───────────────────────────────
	if rt.awaitingCatch {
		rt.awaitingCatch = false
		if !strings.HasPrefix(trimmed, "catch ") {
			fmt.Printf("[❌] try: se esperaba 'catch <Nombre> {' tras el bloque try, se encontró: %s\n", trimmed)
			rt.pendingTry = nil
			return
		}
		name := strings.TrimPrefix(trimmed, "catch ")
		name = strings.TrimSuffix(strings.TrimSpace(name), "{")
		rt.pendingTry.catchName = strings.TrimSpace(name)
		rt.inCatch = true
		return
	}

	// ── Inside a `catch Name { }` body ───────────────────────────────────────
	if rt.inCatch {
		if trimmed == "}" {
			rt.inCatch = false
			rt.finalizeTry()
			return
		}
		rt.pendingTry.catchBody = append(rt.pendingTry.catchBody, trimmed)
		return
	}

	// ── Inside a `for` loop body ─────────────────────────────────────────────
	if rt.inFor {
		if trimmed == "}" {
			rt.finalizeFor()
			return
		}
		rt.pendingFor.body = append(rt.pendingFor.body, trimmed)
		return
	}

	// ── Inside a quantum gate body ────────────────────────────────────────────
	if rt.inGate {
		if trimmed == "}" {
			rt.finalizeGate()
			return
		}
		if strings.HasPrefix(trimmed, "row ") {
			rt.parseGateRow(trimmed)
		}
		return
	}

	switch {
	case strings.HasPrefix(trimmed, "hilbert "):
		rt.parseHilbert(trimmed)

	case strings.HasPrefix(trimmed, "gate "):
		rt.parseGateDecl(trimmed)

	case strings.HasPrefix(trimmed, "technology"):
		rt.parseTechnology(trimmed)

	case strings.HasPrefix(trimmed, "include "):
		rt.parseInclude(trimmed)

	case strings.HasPrefix(trimmed, "for "):
		rt.parseForDecl(trimmed)

	case trimmed == "try {" || trimmed == "try{":
		rt.inTry = true
		rt.pendingTry = &pendingTryBlock{}

	case strings.HasPrefix(trimmed, "space "):
		rt.parseSpace(trimmed)

	case strings.HasPrefix(trimmed, "transform "):
		rt.parseTransformDecl(trimmed)

	case strings.HasPrefix(trimmed, "let "):
		rt.parseLet(trimmed)

	case strings.HasPrefix(trimmed, "when "):
		rt.parseWhen(trimmed)

	case strings.HasPrefix(trimmed, "print "):
		rt.parsePrint(trimmed)

	case strings.HasPrefix(trimmed, "persist "):
		rt.parsePersist(trimmed)

	case strings.HasPrefix(trimmed, "drop "):
		rt.parseDrop(trimmed)

	default:
		// Pending action following a `when` block
		if rt.LastCond != nil && strings.Contains(trimmed, "(") {
			rt.parseAction(trimmed)
		}
	}
}

func (rt *Runtime) Parse(code string) {
	for _, line := range strings.Split(code, "\n") {
		rt.ParseLine(line)
	}
}

// InBlock reports whether the Runtime is mid-way through a multi-line
// block (transform/gate/for/try/catch) — i.e. whether the next ParseLine
// call continues that block rather than starting a new top-level
// statement. A REPL (issue #30) uses this to show a continuation prompt
// instead of re-prompting for a fresh statement.
func (rt *Runtime) InBlock() bool {
	return rt.inTransform || rt.inGate || rt.inFor || rt.inTry || rt.awaitingCatch || rt.inCatch
}

// ── include ───────────────────────────────────────────────────────────────────

// Syntax: include "path/to/file.lin"  (issue #29)
// Loads and parses path into the same Runtime, so its space/transform/gate
// declarations become available to the including program — a module
// system built directly on Parse, not a separate compilation unit.
// Re-including an already-loaded path (directly, or via a cycle) is a
// silent no-op. Paths are resolved relative to the process's working
// directory, the same convention main.go already uses for the top-level
// .lin file — not relative to the including file's own directory.
func (rt *Runtime) parseInclude(line string) {
	path := strings.TrimPrefix(line, "include ")
	path = strings.TrimSpace(path)
	if len(path) >= 2 && path[0] == '"' && path[len(path)-1] == '"' {
		path = path[1 : len(path)-1]
	}

	if rt.included == nil {
		rt.included = map[string]bool{}
	}
	if rt.included[path] {
		return
	}
	rt.included[path] = true

	code, err := os.ReadFile(path)
	if err != nil {
		rt.fail("ImportError", "include '%s': %v", path, err)
		return
	}
	fmt.Printf("[📦] include %s\n", path)
	rt.Parse(string(code))
}

// ── space ─────────────────────────────────────────────────────────────────────

// Syntax: space Name: dim1: Type, dim2: Type, ...
// Type is "Real" (default if omitted) or "String" (issue #24) — a String
// dimension is carried as metadata (e.g. a task's title) and excluded from
// vector arithmetic (Add, Dot, Scale, Norm, Project).
func (rt *Runtime) parseSpace(line string) {
	line = strings.TrimPrefix(line, "space ")
	parts := strings.SplitN(line, ":", 2)
	name := strings.TrimSpace(parts[0])
	dims := []string{}
	types := []core.DimType{}
	if len(parts) > 1 {
		for _, d := range strings.Split(parts[1], ",") {
			fields := strings.SplitN(strings.TrimSpace(d), ":", 2)
			dimName := strings.TrimSpace(fields[0])
			dimType := core.Real
			if len(fields) > 1 && strings.TrimSpace(fields[1]) == "String" {
				dimType = core.String
			}
			dims = append(dims, dimName)
			types = append(types, dimType)
		}
	}
	rt.Spaces[name] = core.NewTypedSpace(name, dims, types)
	fmt.Printf("[✔️] Espacio creado: %s  dims=%v  types=%v\n", name, dims, types)
}

// ── transform declaration ─────────────────────────────────────────────────────

// Syntax:
//
//	transform name: Dom1 x Dom2 -> Codomain {
//	    dim = expr
//	    ...
//	}
func (rt *Runtime) parseTransformDecl(line string) {
	line = strings.TrimPrefix(line, "transform ")
	line = strings.TrimSuffix(strings.TrimSpace(line), "{")
	line = strings.TrimSpace(line)

	parts := strings.SplitN(line, ":", 2)
	name := strings.TrimSpace(parts[0])

	sig := strings.TrimSpace(parts[1])
	arrowParts := strings.SplitN(sig, "->", 2)
	codomain := strings.TrimSpace(arrowParts[1])

	domParts := strings.Split(arrowParts[0], "x")
	dom1 := strings.TrimSpace(domParts[0])
	dom2 := ""
	if len(domParts) > 1 {
		dom2 = strings.TrimSpace(domParts[1])
	}

	rt.inTransform = true
	rt.pendingTx = &pendingTransform{
		name:     name,
		dom1:     dom1,
		dom2:     dom2,
		codomain: codomain,
		mappings: map[string]core.Expr{},
	}
	fmt.Printf("[🔧] Definiendo transform: %s: %s × %s → %s\n", name, dom1, dom2, codomain)
}

func (rt *Runtime) finalizeTransform() {
	tx := rt.pendingTx
	coSpace, ok := rt.Spaces[tx.codomain]
	if !ok {
		fmt.Printf("[❌] transform '%s': espacio codomain '%s' no encontrado\n", tx.name, tx.codomain)
		rt.inTransform = false
		rt.pendingTx = nil
		return
	}
	dom1Space := rt.Spaces[tx.dom1]
	var dom2Space *core.Space
	if tx.dom2 != "" {
		dom2Space = rt.Spaces[tx.dom2]
	}

	// Capture immutable copies for the closure
	mappings := tx.mappings
	dims := coSpace.Dimensions
	capturedCo := coSpace

	rt.Transforms[tx.name] = core.NewTransform(tx.name, dom1Space, dom2Space, capturedCo,
		func(p, m *core.Vector) *core.Vector {
			vals := make([]float64, len(dims))
			for i, dim := range dims {
				if expr, ok := mappings[dim]; ok {
					vals[i] = expr.Eval(p, m)
				}
			}
			return core.NewVector(capturedCo, vals)
		})

	fmt.Printf("[✔️] Transform '%s' registrado\n", tx.name)
	rt.inTransform = false
	rt.pendingTx = nil
}

// ── let ───────────────────────────────────────────────────────────────────────

// Handles all forms of `let varName = rhs`
func (rt *Runtime) parseLet(line string) {
	line = strings.TrimPrefix(line, "let ")
	eqIdx := strings.Index(line, "=")
	if eqIdx < 0 {
		fmt.Println("[⚠️] let: falta '=':", line)
		return
	}
	varName := strings.TrimSpace(line[:eqIdx])
	rhs := strings.TrimSpace(line[eqIdx+1:])

	switch {
	case strings.HasPrefix(rhs, "project "):
		rt.parseProject(varName, rhs)

	case strings.HasPrefix(rhs, "query_one "):
		rt.parseQueryOne(varName, rhs)

	case strings.HasPrefix(rhs, "query "):
		rt.parseQuery(varName, rhs)

	case strings.Contains(rhs, "("):
		rt.parseFuncCall(varName, rhs)

	case strings.Contains(rhs, "["):
		// Disambiguate SpaceName[v1, v2, ...] (vector literal) from
		// collection[idx] (indexing, issue #27): a bracketed name is an
		// index expression only if that name is an already-loaded
		// collection, never a declared space.
		name := strings.TrimSpace(rhs[:strings.Index(rhs, "[")])
		if _, ok := rt.Collections[name]; ok {
			rt.parseCollectionIndex(varName, name, rhs)
		} else {
			rt.parseVectorLiteral(varName, rhs)
		}

	default:
		fmt.Printf("[⚠️] let %s: expresión no reconocida: %s\n", varName, rhs)
	}
}

// Collection indexing: let x = collection[idx]  (issue #27)
func (rt *Runtime) parseCollectionIndex(varName, collName, rhs string) {
	openIdx := strings.Index(rhs, "[")
	closeIdx := strings.Index(rhs, "]")
	idxStr := strings.TrimSpace(rhs[openIdx+1 : closeIdx])

	idx, err := strconv.Atoi(idxStr)
	if err != nil {
		fmt.Printf("[❌] %s[%s]: índice no numérico\n", collName, idxStr)
		return
	}
	coll := rt.Collections[collName]
	if idx < 0 || idx >= len(coll) {
		rt.fail("IndexOutOfRange", "%s[%d]: fuera de rango (len=%d)", collName, idx, len(coll))
		return
	}
	rt.Vectors[varName] = coll[idx]
	fmt.Printf("[✔️] %s = %s[%d]  →  %v\n", varName, collName, idx, coll[idx].Display())
}

// Vector literal: SpaceName[v1, v2, ...]
// A token quoted in "double quotes" is a String-typed dimension's value
// (issue #24); everything else is parsed as Real.
func (rt *Runtime) parseVectorLiteral(varName, rhs string) {
	openIdx := strings.Index(rhs, "[")
	closeIdx := strings.Index(rhs, "]")
	spName := strings.TrimSpace(rhs[:openIdx])
	valStr := strings.TrimSpace(rhs[openIdx+1 : closeIdx])

	sp, ok := rt.Spaces[spName]
	if !ok {
		fmt.Printf("[❌] Espacio no encontrado: %s\n", spName)
		return
	}

	tokens := splitRespectingQuotes(valStr)
	nums := make([]float64, len(tokens))
	strVals := map[string]string{}
	for i, raw := range tokens {
		tok := strings.TrimSpace(raw)
		if len(tok) >= 2 && tok[0] == '"' && tok[len(tok)-1] == '"' {
			if i < len(sp.Dimensions) {
				strVals[sp.Dimensions[i]] = tok[1 : len(tok)-1]
			}
			continue
		}
		f, _ := strconv.ParseFloat(tok, 64)
		nums[i] = f
	}

	vec := core.NewVector(sp, nums)
	for dim, val := range strVals {
		vec.SetString(dim, val)
	}
	rt.Vectors[varName] = vec
	fmt.Printf("[✔️] Vector %s creado en espacio %s  %v\n", varName, spName, vec.Display())
}

// Function call: fn(arg1, arg2, ...)
// Built-ins: dot, norm, scale, add (classical) + ket, apply, tensor, measure,
// density, partial_trace, braket, purity (quantum).
// Falls back to registered transforms.
func (rt *Runtime) parseFuncCall(varName, rhs string) {
	fnName := strings.TrimSpace(rhs[:strings.Index(rhs, "(")])
	argsStr := rhs[strings.Index(rhs, "(")+1 : strings.LastIndex(rhs, ")")]
	args := splitArgs(argsStr)

	// ── Quantum built-ins (delegated to quantum.go) ──────────────────────────
	if rt.parseQuantumFuncCall(varName, fnName, args) {
		return
	}

	switch fnName {

	case "filter":
		rt.applyFilter(varName, args)

	case "map":
		rt.applyMap(varName, args)

	case "dot":
		// dot(v1, v2) → scalar
		v1 := rt.Vectors[strings.TrimSpace(args[0])]
		v2 := rt.Vectors[strings.TrimSpace(args[1])]
		if v1 == nil || v2 == nil {
			rt.fail("NotFound", "dot: vector(es) no encontrado(s)")
			return
		}
		result := v1.Dot(v2)
		rt.Scalars[varName] = result
		fmt.Printf("[✔️] %s = dot(%s, %s) = %.6f\n",
			varName, strings.TrimSpace(args[0]), strings.TrimSpace(args[1]), result)

	case "norm":
		// norm(v) → scalar
		v := rt.Vectors[strings.TrimSpace(args[0])]
		if v == nil {
			rt.fail("NotFound", "norm: vector '%s' no encontrado", strings.TrimSpace(args[0]))
			return
		}
		result := v.Norm()
		rt.Scalars[varName] = result
		fmt.Printf("[✔️] %s = norm(%s) = %.6f\n", varName, strings.TrimSpace(args[0]), result)

	case "scale":
		// scale(v, factor) → vector in same space
		v := rt.Vectors[strings.TrimSpace(args[0])]
		if v == nil {
			rt.fail("NotFound", "scale: vector '%s' no encontrado", strings.TrimSpace(args[0]))
			return
		}
		factor := 1.0
		if len(args) > 1 {
			factorStr := strings.TrimSpace(args[1])
			if s, ok := rt.Scalars[factorStr]; ok {
				factor = s
			} else {
				factor, _ = strconv.ParseFloat(factorStr, 64)
			}
		}
		rt.Vectors[varName] = v.Scale(factor)
		fmt.Printf("[✔️] %s = scale(%s, %.4f)  →  %v\n",
			varName, strings.TrimSpace(args[0]), factor, rt.Vectors[varName].Values)

	case "add":
		// add(v1, v2) → vector in same space
		v1 := rt.Vectors[strings.TrimSpace(args[0])]
		v2 := rt.Vectors[strings.TrimSpace(args[1])]
		if v1 == nil || v2 == nil {
			rt.fail("NotFound", "add: vector(es) no encontrado(s)")
			return
		}
		result, err := v1.Add(v2)
		if err != nil {
			fmt.Printf("[❌] add: %v\n", err)
			return
		}
		rt.Vectors[varName] = result
		fmt.Printf("[✔️] %s = add(%s, %s)  →  %v\n",
			varName, strings.TrimSpace(args[0]), strings.TrimSpace(args[1]), result.Values)

	default:
		// Registered transform call
		tx, ok := rt.Transforms[fnName]
		if !ok {
			fmt.Printf("[❌] Función/transform no reconocido: '%s'\n", fnName)
			return
		}
		v1 := rt.Vectors[strings.TrimSpace(args[0])]
		v2 := rt.Vectors[strings.TrimSpace(args[1])]
		rt.Vectors[varName] = tx.Apply(v1, v2)
		fmt.Printf("[✔️] %s = %s(%s, %s)  →  %v\n",
			varName, fnName,
			strings.TrimSpace(args[0]), strings.TrimSpace(args[1]),
			rt.Vectors[varName].Values)
	}
}

// Projection: project v onto SpaceName
func (rt *Runtime) parseProject(varName, rhs string) {
	rhs = strings.TrimPrefix(rhs, "project ")
	ontoParts := strings.SplitN(rhs, " onto ", 2)
	if len(ontoParts) < 2 {
		fmt.Println("[⚠️] project: sintaxis: project <vector> onto <espacio>")
		return
	}
	srcName := strings.TrimSpace(ontoParts[0])
	targetName := strings.TrimSpace(ontoParts[1])

	v := rt.Vectors[srcName]
	if v == nil {
		rt.fail("NotFound", "project: vector '%s' no encontrado", srcName)
		return
	}
	targetSpace := rt.Spaces[targetName]
	if targetSpace == nil {
		rt.fail("NotFound", "project: espacio '%s' no encontrado", targetName)
		return
	}
	projected, _ := v.Project(targetSpace)
	rt.Vectors[varName] = projected
	fmt.Printf("[✔️] %s = project %s onto %s  →  %v\n",
		varName, srcName, targetName, projected.Values)
}

// ── for ───────────────────────────────────────────────────────────────────────

// Syntax: for <var> in <collection> {
func (rt *Runtime) parseForDecl(line string) {
	line = strings.TrimPrefix(line, "for ")
	line = strings.TrimSuffix(strings.TrimSpace(line), "{")
	line = strings.TrimSpace(line)

	parts := strings.SplitN(line, " in ", 2)
	if len(parts) < 2 {
		fmt.Printf("[❌] for: sintaxis: for <var> in <coleccion> {\n")
		return
	}
	rt.inFor = true
	rt.pendingFor = &pendingForLoop{
		varName:  strings.TrimSpace(parts[0]),
		collName: strings.TrimSpace(parts[1]),
	}
	fmt.Printf("[🔁] for %s in %s {\n", rt.pendingFor.varName, rt.pendingFor.collName)
}

// finalizeFor executes the accumulated body once per vector in the target
// collection, binding pendingFor.varName to each vector in turn via
// rt.Vectors — "iteración" over collections as first-class values (issue
// #25). Each body line is re-dispatched through ParseLine, so the body can
// contain anything a top-level line can: prints, further let-expressions,
// transforms, conditionals.
//
// Limitation: the body collector only tracks the outermost '{'/'}' pair —
// it does not nest, so a for-body cannot itself contain another brace
// block (gate/transform/for). Not needed by any current example; revisit
// if that becomes necessary.
func (rt *Runtime) finalizeFor() {
	pf := rt.pendingFor
	rt.inFor, rt.pendingFor = false, nil

	coll, ok := rt.Collections[pf.collName]
	if !ok {
		fmt.Printf("[❌] for: colección '%s' no encontrada\n", pf.collName)
		return
	}
	for _, v := range coll {
		rt.Vectors[pf.varName] = v
		for _, bodyLine := range pf.body {
			rt.ParseLine(bodyLine)
		}
	}
	fmt.Printf("[✔️] for %s in %s  → %d iteración(es)\n", pf.varName, pf.collName, len(coll))
}

// ── try / catch ──────────────────────────────────────────────────────────────

// fail records a catchable runtime error (issue #33) and prints the usual
// "[❌]" diagnostic. A try/catch never changes what gets printed — only
// whether execution stops and a catch block runs — so code outside any
// try block behaves exactly as it did before this error became catchable.
//
// Wired into a representative subset of "not found" / "out of range"
// error sites (query_one, collection indexing, vector/space lookups in
// project and the classical dot/norm/scale/add builtins) rather than
// every print site in the interpreter — those are the runtime conditions
// a program actually wants to recover from, as opposed to malformed
// syntax, which is a program bug to fix, not a condition to catch.
func (rt *Runtime) fail(name, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	rt.LastError = &RuntimeError{Name: name, Message: msg}
	fmt.Printf("[❌] %s\n", msg)
}

// finalizeTry runs the accumulated try-body line by line, stopping at the
// first line that calls fail(). If none did, the catch block never runs —
// try/catch is a no-op for a successful try. If one did and its error name
// matches the catch clause (or the catch clause is empty, catching
// anything), the catch-body runs instead.
func (rt *Runtime) finalizeTry() {
	pt := rt.pendingTry
	rt.pendingTry = nil
	rt.LastError = nil

	for _, line := range pt.tryBody {
		rt.ParseLine(line)
		if rt.LastError != nil {
			break
		}
	}

	if rt.LastError == nil {
		return
	}
	caught := *rt.LastError
	rt.LastError = nil

	if pt.catchName != "" && pt.catchName != caught.Name {
		fmt.Printf("[❌] try: error '%s' no coincide con catch '%s' — sin manejar\n", caught.Name, pt.catchName)
		return
	}
	for _, line := range pt.catchBody {
		rt.ParseLine(line)
	}
}

// ── when ──────────────────────────────────────────────────────────────────────

// Syntax: when LHS OP value:
// LHS can be:
//   - vec.dim         (classical vector dimension)
//   - scalarName      (any value in Scalars map: dot, norm, purity, etc.)
//   - purity(rho)     (quantum density matrix purity)
//   - qnorm(psi)      (quantum state norm)
//
// Supported operators: >, <, >=, <=, ==, !=
func (rt *Runtime) parseWhen(line string) {
	line = strings.TrimPrefix(line, "when ")
	line = strings.TrimSuffix(strings.TrimSpace(line), ":")

	op, before, after := extractOperator(line)
	if op == "" {
		fmt.Println("[⚠️] when: operador relacional no encontrado en:", line)
		return
	}

	lhs := strings.TrimSpace(before)
	compVal, err := strconv.ParseFloat(strings.TrimSpace(after), 64)
	if err != nil {
		fmt.Printf("[⚠️] when: valor no numérico: %s\n", after)
		return
	}

	// Resolve LHS to a float64 getter closure
	getter, label, ok := rt.resolveConditionLHS(lhs)
	if !ok {
		fmt.Printf("[❌] when: no se puede resolver '%s'\n", lhs)
		return
	}

	rt.LastCond = core.NewConditional(label, getter, makeCondition(op, compVal), func() {})
	fmt.Printf("[✔️] Condición registrada: %s %s %.4f\n", label, op, compVal)
}

// makeCondition builds the predicate function for a given operator and threshold.
func makeCondition(op string, val float64) func(float64) bool {
	switch op {
	case ">":
		return func(x float64) bool { return x > val }
	case "<":
		return func(x float64) bool { return x < val }
	case ">=":
		return func(x float64) bool { return x >= val }
	case "<=":
		return func(x float64) bool { return x <= val }
	case "==":
		return func(x float64) bool { return x == val }
	case "!=":
		return func(x float64) bool { return x != val }
	}
	return func(x float64) bool { return false }
}

// extractOperator scans s for a relational operator and returns (op, before, after).
// Two-character operators are checked before single-character ones to avoid false matches.
func extractOperator(s string) (op, before, after string) {
	for _, o := range []string{">=", "<=", "==", "!="} {
		if idx := strings.Index(s, o); idx >= 0 {
			return o, s[:idx], s[idx+2:]
		}
	}
	for _, o := range []string{">", "<"} {
		if idx := strings.Index(s, o); idx >= 0 {
			return o, s[:idx], s[idx+1:]
		}
	}
	return "", s, ""
}

// ── action (line following a `when` block) ────────────────────────────────────

// Syntax: actionName(varName)
// Built-in actions: approve, reject — anything else prints a notification.
func (rt *Runtime) parseAction(line string) {
	fnName := strings.TrimSpace(line[:strings.Index(line, "(")])
	argStr := strings.TrimSpace(line[strings.Index(line, "(")+1 : strings.LastIndex(line, ")")])

	capturedArg := argStr
	capturedFn := fnName

	rt.LastCond.Action = func() {
		// Resolve argument across all runtime namespaces
		var repr string
		switch {
		case rt.Vectors[capturedArg] != nil:
			repr = fmt.Sprintf("%v", rt.Vectors[capturedArg])
		case rt.QuantumStates[capturedArg] != nil:
			repr = fmt.Sprintf("|ψ⟩@%s", rt.QuantumStates[capturedArg].Space.Name)
		case rt.DensityMatrices[capturedArg] != nil:
			repr = fmt.Sprintf("%v", rt.DensityMatrices[capturedArg])
		default:
			if s, ok := rt.Scalars[capturedArg]; ok {
				repr = fmt.Sprintf("%.6f", s)
			} else {
				repr = capturedArg
			}
		}
		switch capturedFn {
		case "approve":
			fmt.Printf("[✅] APROBADO: %s\n", repr)
		case "reject":
			fmt.Printf("[🚫] RECHAZADO: %s\n", repr)
		default:
			fmt.Printf("[🔔] %s(%s) ejecutado\n", capturedFn, repr)
		}
	}

	rt.LastCond.Evaluate()
	rt.LastCond = nil
}

// ── print ─────────────────────────────────────────────────────────────────────

// Syntax: print name
func (rt *Runtime) parsePrint(line string) {
	name := strings.TrimSpace(strings.TrimPrefix(line, "print "))
	// Collections (query results)
	if rt.printCollection(name) {
		return
	}
	// Quantum types
	if rt.printQuantum(name) {
		return
	}
	if v, ok := rt.Vectors[name]; ok {
		fmt.Printf("[📌] %s = %v  (espacio: %s)\n", name, v.Display(), v.Space.Name)
		return
	}
	if s, ok := rt.Scalars[name]; ok {
		fmt.Printf("[📌] %s = %.6f\n", name, s)
		return
	}
	fmt.Printf("[⚠️] print: '%s' no encontrado\n", name)
}

// ── helpers ───────────────────────────────────────────────────────────────────

// splitRespectingQuotes splits s on commas that are not inside a
// "double-quoted" string literal (issue #24: a String dimension's value
// may itself contain a comma, e.g. "comprar pan, leche").
func splitRespectingQuotes(s string) []string {
	var parts []string
	inQuotes := false
	start := 0
	for i, c := range s {
		switch c {
		case '"':
			inQuotes = !inQuotes
		case ',':
			if !inQuotes {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, s[start:])
	return parts
}

// splitArgs splits a comma-separated argument string respecting nested parentheses.
func splitArgs(s string) []string {
	var args []string
	depth := 0
	start := 0
	for i, c := range s {
		switch c {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				args = append(args, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	args = append(args, strings.TrimSpace(s[start:]))
	return args
}

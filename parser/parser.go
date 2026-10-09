package parser

import (
	"fmt"
	"math/rand"
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

	// ── Persistence ────────────────────────────────────────────────────────────
	Store       store.Backend             // nil = no persistence
	Collections map[string][]*core.Vector // results of query statements
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

	case strings.HasPrefix(trimmed, "for "):
		rt.parseForDecl(trimmed)

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

// ── space ─────────────────────────────────────────────────────────────────────

// Syntax: space Name: dim1: Type, dim2: Type, ...
func (rt *Runtime) parseSpace(line string) {
	line = strings.TrimPrefix(line, "space ")
	parts := strings.SplitN(line, ":", 2)
	name := strings.TrimSpace(parts[0])
	dims := []string{}
	if len(parts) > 1 {
		for _, d := range strings.Split(parts[1], ",") {
			dimName := strings.Split(strings.TrimSpace(d), ":")[0]
			dims = append(dims, strings.TrimSpace(dimName))
		}
	}
	rt.Spaces[name] = core.NewSpace(name, dims)
	fmt.Printf("[✔️] Espacio creado: %s  dims=%v\n", name, dims)
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
		rt.parseVectorLiteral(varName, rhs)

	default:
		fmt.Printf("[⚠️] let %s: expresión no reconocida: %s\n", varName, rhs)
	}
}

// Vector literal: SpaceName[v1, v2, ...]
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
	nums := []float64{}
	for _, v := range strings.Split(valStr, ",") {
		f, _ := strconv.ParseFloat(strings.TrimSpace(v), 64)
		nums = append(nums, f)
	}
	rt.Vectors[varName] = core.NewVector(sp, nums)
	fmt.Printf("[✔️] Vector %s creado en espacio %s  %v\n", varName, spName, nums)
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
			fmt.Println("[❌] dot: vector(es) no encontrado(s)")
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
			fmt.Printf("[❌] norm: vector '%s' no encontrado\n", strings.TrimSpace(args[0]))
			return
		}
		result := v.Norm()
		rt.Scalars[varName] = result
		fmt.Printf("[✔️] %s = norm(%s) = %.6f\n", varName, strings.TrimSpace(args[0]), result)

	case "scale":
		// scale(v, factor) → vector in same space
		v := rt.Vectors[strings.TrimSpace(args[0])]
		if v == nil {
			fmt.Printf("[❌] scale: vector '%s' no encontrado\n", strings.TrimSpace(args[0]))
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
			fmt.Println("[❌] add: vector(es) no encontrado(s)")
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
		fmt.Printf("[❌] project: vector '%s' no encontrado\n", srcName)
		return
	}
	targetSpace := rt.Spaces[targetName]
	if targetSpace == nil {
		fmt.Printf("[❌] project: espacio '%s' no encontrado\n", targetName)
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
		fmt.Printf("[📌] %s = %v  (espacio: %s)\n", name, v.Values, v.Space.Name)
		return
	}
	if s, ok := rt.Scalars[name]; ok {
		fmt.Printf("[📌] %s = %.6f\n", name, s)
		return
	}
	fmt.Printf("[⚠️] print: '%s' no encontrado\n", name)
}

// ── helpers ───────────────────────────────────────────────────────────────────

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

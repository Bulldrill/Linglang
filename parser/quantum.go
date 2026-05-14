package parser

// quantum.go — LinLang Hilbert-space extension
//
// New keywords:
//   hilbert Name: dim N
//   gate Name: Domain -> Codomain { row [...] ... }
//
// New let-expressions:
//   ket(Space, amp0, amp1, ...)   → QuantumState
//   apply(Gate, |ψ⟩)             → QuantumState
//   tensor(|ψ1⟩, |ψ2⟩)          → QuantumState in product space
//   measure(|ψ⟩)                 → classical int (Born rule)
//   density(|ψ⟩)                 → DensityMatrix ρ = |ψ⟩⟨ψ|
//   partial_trace(ρ, subDim)     → reduced DensityMatrix
//   braket(|φ⟩, |ψ⟩)            → complex inner product ⟨φ|ψ⟩ (real part in Scalars)
//   purity(ρ)                    → Tr(ρ²) stored in Scalars
//
// Built-in gates (auto-loaded per Hilbert space):
//   H, X, Y, Z   → single-qubit (dim=2)
//   CNOT         → two-qubit   (dim=4)

import (
	"fmt"
	"strconv"
	"strings"

	"linlang-go/core"
)

// ── hilbert ───────────────────────────────────────────────────────────────────

// Syntax: hilbert Name: dim N
func (rt *Runtime) parseHilbert(line string) {
	line = strings.TrimPrefix(line, "hilbert ")
	parts := strings.SplitN(line, ":", 2)
	name := strings.TrimSpace(parts[0])
	if len(parts) < 2 {
		fmt.Printf("[❌] hilbert: sintaxis: hilbert <nombre>: dim <N>\n")
		return
	}
	dimStr := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(parts[1]), "dim"))
	dim, err := strconv.Atoi(strings.TrimSpace(dimStr))
	if err != nil || dim < 1 {
		fmt.Printf("[❌] hilbert '%s': dimensión inválida: %s\n", name, parts[1])
		return
	}
	h := core.NewHilbertSpace(name, dim)
	rt.HilbertSpaces[name] = h

	// Auto-register built-in gates that match this dimension.
	// Identity is always registered so kron() works without manual gate declarations.
	rt.Gates["I_"+name] = core.BuiltinI(h)
	switch dim {
	case 2:
		rt.Gates["H_"+name] = core.BuiltinH(h)
		rt.Gates["X_"+name] = core.BuiltinX(h)
		rt.Gates["Y_"+name] = core.BuiltinY(h)
		rt.Gates["Z_"+name] = core.BuiltinZ(h)
	case 4:
		rt.Gates["CNOT_"+name] = core.BuiltinCNOT(h)
	}

	fmt.Printf("[✔️] Espacio de Hilbert: %s  dim=%d\n", name, dim)
}

// ── gate declaration ──────────────────────────────────────────────────────────

// Syntax:
//
//	gate Name: Domain -> Codomain {
//	    row [c₀, c₁, ...]
//	    ...
//	}
func (rt *Runtime) parseGateDecl(line string) {
	line = strings.TrimPrefix(line, "gate ")
	line = strings.TrimSuffix(strings.TrimSpace(line), "{")
	line = strings.TrimSpace(line)

	parts := strings.SplitN(line, ":", 2)
	name := strings.TrimSpace(parts[0])
	if len(parts) < 2 {
		fmt.Printf("[❌] gate '%s': sintaxis: gate <nombre>: <Dom> -> <Cod> {\n", name)
		return
	}
	sig := strings.TrimSpace(parts[1])
	arrowParts := strings.SplitN(sig, "->", 2)
	dom := strings.TrimSpace(arrowParts[0])
	cod := dom // default codomain = domain
	if len(arrowParts) > 1 {
		cod = strings.TrimSpace(arrowParts[1])
	}

	rt.inGate = true
	rt.pendingGate = &pendingGateDecl{
		name: name,
		dom:  dom,
		cod:  cod,
	}
	fmt.Printf("[🔧] Definiendo gate: %s  %s → %s\n", name, dom, cod)
}

// parseGateRow parses a single matrix row inside a gate body.
// Syntax: row [c₀, c₁, ...]
func (rt *Runtime) parseGateRow(line string) {
	line = strings.TrimPrefix(line, "row ")
	line = strings.Trim(line, "[] ")
	parts := strings.Split(line, ",")
	row := make([]complex128, len(parts))
	for i, p := range parts {
		row[i] = parseComplex(strings.TrimSpace(p))
	}
	rt.pendingGate.rows = append(rt.pendingGate.rows, row)
}

func (rt *Runtime) finalizeGate() {
	pg := rt.pendingGate
	domSpace, ok := rt.HilbertSpaces[pg.dom]
	if !ok {
		fmt.Printf("[❌] gate '%s': espacio de Hilbert '%s' no encontrado\n", pg.name, pg.dom)
		rt.inGate, rt.pendingGate = false, nil
		return
	}
	codSpace := domSpace
	if pg.cod != pg.dom {
		if cs, ok2 := rt.HilbertSpaces[pg.cod]; ok2 {
			codSpace = cs
		}
	}

	g := core.NewGate(pg.name, domSpace, codSpace, pg.rows)
	unitary := g.IsUnitary()
	rt.Gates[pg.name] = g

	check := "✓ unitaria"
	if !unitary {
		check = "⚠️  NO unitaria"
	}
	fmt.Printf("[✔️] Gate '%s' registrada  [%s]\n", pg.name, check)
	rt.inGate, rt.pendingGate = false, nil
}

// ── quantum function calls ────────────────────────────────────────────────────

// parseQuantumFuncCall handles all quantum built-ins.
// Returns true if the function was handled, false to fall through to classical.
func (rt *Runtime) parseQuantumFuncCall(varName, fnName string, args []string) bool {
	switch fnName {

	// ── ket(Space, amp₀, amp₁, …) ────────────────────────────────────────────
	case "ket":
		if len(args) < 2 {
			fmt.Printf("[❌] ket: uso: ket(EspacioHilbert, amp0, amp1, ...)\n")
			return true
		}
		h := rt.HilbertSpaces[strings.TrimSpace(args[0])]
		if h == nil {
			fmt.Printf("[❌] ket: espacio de Hilbert '%s' no encontrado\n", strings.TrimSpace(args[0]))
			return true
		}
		amps := make([]complex128, len(args)-1)
		for i, a := range args[1:] {
			amps[i] = parseComplex(strings.TrimSpace(a))
		}
		state := core.NewQuantumState(h, amps)
		rt.QuantumStates[varName] = state
		fmt.Printf("[✔️] %s = ket  ‖ψ‖=%.4f  probs=%v\n",
			varName, state.Norm(), state.Probabilities())
		return true

	// ── apply(GateName, |ψ⟩) ─────────────────────────────────────────────────
	case "apply":
		if len(args) < 2 {
			fmt.Printf("[❌] apply: uso: apply(Gate, estado)\n")
			return true
		}
		gateName := strings.TrimSpace(args[0])
		stateName := strings.TrimSpace(args[1])
		g := rt.Gates[gateName]
		if g == nil {
			fmt.Printf("[❌] apply: gate '%s' no encontrada\n", gateName)
			return true
		}
		psi := rt.QuantumStates[stateName]
		if psi == nil {
			fmt.Printf("[❌] apply: estado cuántico '%s' no encontrado\n", stateName)
			return true
		}
		result := g.Apply(psi)
		rt.QuantumStates[varName] = result
		fmt.Printf("[✔️] %s = apply(%s, %s)  probs=%v\n",
			varName, gateName, stateName, result.Probabilities())
		return true

	// ── tensor(|ψ1⟩, |ψ2⟩) ───────────────────────────────────────────────────
	case "tensor":
		if len(args) < 2 {
			fmt.Printf("[❌] tensor: uso: tensor(estado1, estado2)\n")
			return true
		}
		s1 := rt.QuantumStates[strings.TrimSpace(args[0])]
		s2 := rt.QuantumStates[strings.TrimSpace(args[1])]
		if s1 == nil || s2 == nil {
			fmt.Printf("[❌] tensor: estado(s) no encontrado(s)\n")
			return true
		}
		// Lookup or create the product Hilbert space
		prodName := s1.Space.Name + "x" + s2.Space.Name
		prodSpace, exists := rt.HilbertSpaces[prodName]
		if !exists {
			prodSpace = core.NewHilbertSpace(prodName, s1.Space.Dim*s2.Space.Dim)
			rt.HilbertSpaces[prodName] = prodSpace
			// Auto-register CNOT if product is dim 4
			if prodSpace.Dim == 4 {
				rt.Gates["CNOT_"+prodName] = core.BuiltinCNOT(prodSpace)
			}
		}
		result := s1.Tensor(s2, prodSpace)
		rt.QuantumStates[varName] = result
		fmt.Printf("[✔️] %s = %s ⊗ %s  dim=%d  probs=%v\n",
			varName,
			strings.TrimSpace(args[0]), strings.TrimSpace(args[1]),
			prodSpace.Dim, result.Probabilities())
		return true

	// ── measure(|ψ⟩) → classical outcome ─────────────────────────────────────
	case "measure":
		if len(args) < 1 {
			fmt.Printf("[❌] measure: uso: measure(estado)\n")
			return true
		}
		psi := rt.QuantumStates[strings.TrimSpace(args[0])]
		if psi == nil {
			fmt.Printf("[❌] measure: estado '%s' no encontrado\n", strings.TrimSpace(args[0]))
			return true
		}
		outcome, probs := psi.Measure()
		rt.Measurements[varName] = outcome
		rt.Scalars[varName] = float64(outcome)
		fmt.Printf("[✔️] %s = measure(%s) → |%d⟩  probs=%v\n",
			varName, strings.TrimSpace(args[0]), outcome, probs)
		return true

	// ── density(|ψ⟩) → ρ = |ψ⟩⟨ψ| ───────────────────────────────────────────
	case "density":
		if len(args) < 1 {
			fmt.Printf("[❌] density: uso: density(estado)\n")
			return true
		}
		psi := rt.QuantumStates[strings.TrimSpace(args[0])]
		if psi == nil {
			fmt.Printf("[❌] density: estado '%s' no encontrado\n", strings.TrimSpace(args[0]))
			return true
		}
		rho := psi.ToDensityMatrix()
		rt.DensityMatrices[varName] = rho
		fmt.Printf("[✔️] %s = density(%s)  %v\n", varName, strings.TrimSpace(args[0]), rho)
		return true

	// ── partial_trace(ρ, subDim) ──────────────────────────────────────────────
	case "partial_trace":
		if len(args) < 2 {
			fmt.Printf("[❌] partial_trace: uso: partial_trace(rho, subDim)\n")
			return true
		}
		rho := rt.DensityMatrices[strings.TrimSpace(args[0])]
		if rho == nil {
			fmt.Printf("[❌] partial_trace: matriz '%s' no encontrada\n", strings.TrimSpace(args[0]))
			return true
		}
		subDim, err := strconv.Atoi(strings.TrimSpace(args[1]))
		if err != nil {
			fmt.Printf("[❌] partial_trace: subDim inválido: %s\n", args[1])
			return true
		}
		reduced := rho.PartialTrace(subDim)
		rt.DensityMatrices[varName] = reduced
		fmt.Printf("[✔️] %s = partial_trace(%s, %d)  %v\n",
			varName, strings.TrimSpace(args[0]), subDim, reduced)
		return true

	// ── braket(|φ⟩, |ψ⟩) → ⟨φ|ψ⟩ ────────────────────────────────────────────
	case "braket":
		if len(args) < 2 {
			fmt.Printf("[❌] braket: uso: braket(phi, psi)\n")
			return true
		}
		s1 := rt.QuantumStates[strings.TrimSpace(args[0])]
		s2 := rt.QuantumStates[strings.TrimSpace(args[1])]
		if s1 == nil || s2 == nil {
			fmt.Printf("[❌] braket: estado(s) no encontrado(s)\n")
			return true
		}
		result := s1.Braket(s2)
		rt.Scalars[varName] = real(result)
		fmt.Printf("[✔️] %s = ⟨%s|%s⟩ = (%.4f%+.4fi)\n",
			varName,
			strings.TrimSpace(args[0]), strings.TrimSpace(args[1]),
			real(result), imag(result))
		return true

	// ── purity(ρ) → Tr(ρ²) ───────────────────────────────────────────────────
	case "purity":
		if len(args) < 1 {
			fmt.Printf("[❌] purity: uso: purity(rho)\n")
			return true
		}
		rho := rt.DensityMatrices[strings.TrimSpace(args[0])]
		if rho == nil {
			fmt.Printf("[❌] purity: matriz '%s' no encontrada\n", strings.TrimSpace(args[0]))
			return true
		}
		p := rho.Purity()
		rt.Scalars[varName] = p
		fmt.Printf("[✔️] %s = purity(%s) = %.6f\n", varName, strings.TrimSpace(args[0]), p)
		return true
	// ── kron(G1, G2) → G1 ⊗ G2  (Kronecker / tensor product of gates) ─────────
	case "kron":
		if len(args) < 2 {
			fmt.Printf("[❌] kron: uso: kron(Gate1, Gate2)\n")
			return true
		}
		g1Name := strings.TrimSpace(args[0])
		g2Name := strings.TrimSpace(args[1])
		g1 := rt.Gates[g1Name]
		g2 := rt.Gates[g2Name]
		if g1 == nil || g2 == nil {
			fmt.Printf("[❌] kron: gate(s) no encontrada(s): '%s'='%v', '%s'='%v'\n",
				g1Name, g1, g2Name, g2)
			return true
		}
		prodDim := g1.Domain.Dim * g2.Domain.Dim
		// Find or auto-create the product HilbertSpace
		var prodSpace *core.HilbertSpace
		for _, h := range rt.HilbertSpaces {
			if h.Dim == prodDim {
				prodSpace = h
				break
			}
		}
		if prodSpace == nil {
			prodName := g1.Domain.Name + "⊗" + g2.Domain.Name
			prodSpace = core.NewHilbertSpace(prodName, prodDim)
			rt.HilbertSpaces[prodName] = prodSpace
			// Auto-register CNOT if the product is a 2-qubit (dim 4) space
			if prodDim == 4 {
				rt.Gates["CNOT_"+prodName] = core.BuiltinCNOT(prodSpace)
			}
		}
		newGate := core.KronGate(g1, g2, prodSpace)
		rt.Gates[varName] = newGate
		fmt.Printf("[✔️] %s = kron(%s, %s)  dim=%d  unitary=%v\n",
			varName, g1Name, g2Name, prodDim, newGate.IsUnitary())
		return true

	// ── trace_alice(ρ, bob_dim) → ρ_Bob  ─────────────────────────────────────
	// Traces the FIRST subsystem (Alice), keeping the LAST (Bob, dim=bob_dim).
	// In teleportation: trace_alice(rho3, 2) gives Bob's single-qubit ρ.
	case "trace_alice":
		if len(args) < 2 {
			fmt.Printf("[❌] trace_alice: uso: trace_alice(rho, bob_dim)\n")
			return true
		}
		rho := rt.DensityMatrices[strings.TrimSpace(args[0])]
		if rho == nil {
			fmt.Printf("[❌] trace_alice: matriz '%s' no encontrada\n", strings.TrimSpace(args[0]))
			return true
		}
		keepDim, err := strconv.Atoi(strings.TrimSpace(args[1]))
		if err != nil {
			fmt.Printf("[❌] trace_alice: bob_dim inválido: %s\n", args[1])
			return true
		}
		reduced := rho.PartialTraceA(keepDim)
		rt.DensityMatrices[varName] = reduced
		fmt.Printf("[✔️] %s = trace_alice(%s, %d)  %v\n",
			varName, strings.TrimSpace(args[0]), keepDim, reduced)
		return true
	}

	return false // not a quantum built-in
}

// ── resolveConditionLHS ───────────────────────────────────────────────────────

// resolveConditionLHS returns a float64 getter closure for a `when` LHS.
// Handles (in order):
//  1. purity(rho) / qnorm(psi) / trace(rho)  — quantum property functions
//  2. scalar variable name (Scalars map)
//  3. vec.dim                                  — classical vector dimension
func (rt *Runtime) resolveConditionLHS(lhs string) (getter func() float64, label string, ok bool) {
	// Case 1: function call, e.g. purity(rho), qnorm(psi)
	if strings.Contains(lhs, "(") {
		fnName := strings.TrimSpace(lhs[:strings.Index(lhs, "(")])
		argStr := strings.TrimSpace(lhs[strings.Index(lhs, "(")+1 : strings.LastIndex(lhs, ")")])
		arg := strings.TrimSpace(argStr)
		switch fnName {
		case "purity":
			rho := rt.DensityMatrices[arg]
			if rho != nil {
				return func() float64 { return rho.Purity() }, lhs, true
			}
		case "qnorm":
			psi := rt.QuantumStates[arg]
			if psi != nil {
				return func() float64 { return psi.Norm() }, lhs, true
			}
		case "trace":
			rho := rt.DensityMatrices[arg]
			if rho != nil {
				return func() float64 { return rho.Trace() }, lhs, true
			}
		}
		return nil, lhs, false
	}

	// Case 2: scalar variable (dot, norm, measure outcome, purity, etc.)
	if val, exists := rt.Scalars[lhs]; exists {
		return func() float64 { return val }, lhs, true
	}

	// Case 3: vec.dim  (classical form preserved for backwards compatibility)
	if strings.Contains(lhs, ".") {
		parts := strings.SplitN(lhs, ".", 2)
		v := rt.Vectors[strings.TrimSpace(parts[0])]
		dim := strings.TrimSpace(parts[1])
		if v != nil {
			return func() float64 { return v.Get(dim) }, lhs, true
		}
	}

	return nil, lhs, false
}

// ── parseComplex ──────────────────────────────────────────────────────────────

// parseComplex parses a complex literal:
//
//	"1"           → complex(1, 0)
//	"0.707"       → complex(0.707, 0)
//	"-0.707"      → complex(-0.707, 0)
//	"0.5i"        → complex(0, 0.5)
//	"-1i"         → complex(0, -1)
//	"0.5+0.5i"    → complex(0.5, 0.5)
//	"0.5-0.5i"    → complex(0.5, -0.5)
//	"1+0i"        → complex(1, 0)
func parseComplex(s string) complex128 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}

	// No imaginary component
	if !strings.HasSuffix(s, "i") {
		f, _ := strconv.ParseFloat(s, 64)
		return complex(f, 0)
	}

	// Remove trailing 'i'
	inner := s[:len(s)-1]

	// Find the last '+' or '-' at position > 0 that separates real and imaginary
	splitIdx := -1
	for i := len(inner) - 1; i > 0; i-- {
		if inner[i] == '+' || inner[i] == '-' {
			splitIdx = i
			break
		}
	}

	if splitIdx < 0 {
		// Pure imaginary: "0.707i" or "-0.707i"
		f, _ := strconv.ParseFloat(inner, 64)
		return complex(0, f)
	}

	// Mixed: "a+bi" or "a-bi"
	realPart, _ := strconv.ParseFloat(inner[:splitIdx], 64)
	imagStr := inner[splitIdx:]
	switch imagStr {
	case "+":
		return complex(realPart, 1)
	case "-":
		return complex(realPart, -1)
	}
	imagPart, _ := strconv.ParseFloat(imagStr, 64)
	return complex(realPart, imagPart)
}

// ── printQuantum ──────────────────────────────────────────────────────────────

// printQuantum prints quantum-type variables.
// Returns true if the name was found, false otherwise.
func (rt *Runtime) printQuantum(name string) bool {
	if q, ok := rt.QuantumStates[name]; ok {
		probs := q.Probabilities()
		fmt.Printf("[📌] %s  (|ψ⟩ en %s)\n", name, q.Space.Name)
		fmt.Printf("      amplitudes: %v\n", q.Amplitudes)
		fmt.Printf("      probs:      %v\n", probs)
		fmt.Printf("      ‖ψ‖:        %.6f\n", q.Norm())
		return true
	}
	if d, ok := rt.DensityMatrices[name]; ok {
		fmt.Printf("[📌] %s  %v\n", name, d)
		for i, row := range d.Matrix {
			fmt.Printf("      fila %d: %v\n", i, row)
		}
		return true
	}
	if g, ok := rt.Gates[name]; ok {
		fmt.Printf("[📌] %s  %v\n", name, g)
		return true
	}
	return false
}

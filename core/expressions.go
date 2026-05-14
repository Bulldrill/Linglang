package core

import (
	"strconv"
	"strings"
	"unicode"
)

// ── Token types ───────────────────────────────────────────────────────────────

type tKind int

const (
	tNum    tKind = iota
	tIdent        // identifier (p, m, variable names)
	tDot          // .
	tPlus         // +
	tMinus        // -
	tStar         // *
	tSlash        // /
	tLParen       // (
	tRParen       // )
	tEOF
)

type tok struct {
	k tKind
	v string
}

func tokenize(s string) []tok {
	var out []tok
	rs := []rune(strings.TrimSpace(s))
	i := 0
	for i < len(rs) {
		c := rs[i]
		if unicode.IsSpace(c) {
			i++
			continue
		}
		// Numbers: digits, may contain a decimal point
		if unicode.IsDigit(c) {
			j := i
			for j < len(rs) && (unicode.IsDigit(rs[j]) || rs[j] == '.') {
				j++
			}
			out = append(out, tok{tNum, string(rs[i:j])})
			i = j
			continue
		}
		// Identifiers: letters/digits/underscores
		if unicode.IsLetter(c) || c == '_' {
			j := i
			for j < len(rs) && (unicode.IsLetter(rs[j]) || unicode.IsDigit(rs[j]) || rs[j] == '_') {
				j++
			}
			out = append(out, tok{tIdent, string(rs[i:j])})
			i = j
			continue
		}
		switch c {
		case '.':
			out = append(out, tok{tDot, "."})
		case '+':
			out = append(out, tok{tPlus, "+"})
		case '-':
			out = append(out, tok{tMinus, "-"})
		case '*':
			out = append(out, tok{tStar, "*"})
		case '/':
			out = append(out, tok{tSlash, "/"})
		case '(':
			out = append(out, tok{tLParen, "("})
		case ')':
			out = append(out, tok{tRParen, ")"})
		}
		i++
	}
	out = append(out, tok{tEOF, ""})
	return out
}

// ── AST Nodes ─────────────────────────────────────────────────────────────────

// Expr is an evaluatable arithmetic expression.
// Both input vectors p and m can be nil for unary transforms.
type Expr interface {
	Eval(p, m *Vector) float64
}

// NumExpr is a numeric literal.
type NumExpr struct{ V float64 }

func (e *NumExpr) Eval(_, _ *Vector) float64 { return e.V }

// FieldExpr accesses p.dim or m.dim (e.g. p.energia, m.edad, p.id).
type FieldExpr struct {
	Src string // "p" or "m"
	Dim string // dimension name or "id"
}

func (e *FieldExpr) Eval(p, m *Vector) float64 {
	var v *Vector
	switch e.Src {
	case "p":
		v = p
	case "m":
		v = m
	default:
		return 0
	}
	if v == nil {
		return 0
	}
	if e.Dim == "id" {
		return float64(v.ID)
	}
	return v.Get(e.Dim)
}

// BinExpr is a binary arithmetic operation.
type BinExpr struct {
	Op    string
	Left  Expr
	Right Expr
}

func (e *BinExpr) Eval(p, m *Vector) float64 {
	l, r := e.Left.Eval(p, m), e.Right.Eval(p, m)
	switch e.Op {
	case "+":
		return l + r
	case "-":
		return l - r
	case "*":
		return l * r
	case "/":
		if r == 0 {
			return 0
		}
		return l / r
	}
	return 0
}

// ── Recursive-descent parser ──────────────────────────────────────────────────

type exprParser struct {
	toks []tok
	pos  int
}

func (p *exprParser) peek() tok { return p.toks[p.pos] }
func (p *exprParser) next() tok { t := p.toks[p.pos]; p.pos++; return t }

func (p *exprParser) parse() Expr { return p.addSub() }

// addSub handles +, - (left-associative, lowest precedence)
func (p *exprParser) addSub() Expr {
	left := p.mulDiv()
	for {
		t := p.peek()
		if t.k != tPlus && t.k != tMinus {
			break
		}
		p.next()
		right := p.mulDiv()
		left = &BinExpr{t.v, left, right}
	}
	return left
}

// mulDiv handles *, / (left-associative, higher precedence)
func (p *exprParser) mulDiv() Expr {
	left := p.unary()
	for {
		t := p.peek()
		if t.k != tStar && t.k != tSlash {
			break
		}
		p.next()
		right := p.unary()
		left = &BinExpr{t.v, left, right}
	}
	return left
}

// unary handles unary minus
func (p *exprParser) unary() Expr {
	if p.peek().k == tMinus {
		p.next()
		e := p.atom()
		return &BinExpr{"*", &NumExpr{-1}, e}
	}
	return p.atom()
}

// atom handles literals, field accesses (p.dim), and parenthesised expressions
func (p *exprParser) atom() Expr {
	t := p.peek()
	switch t.k {
	case tNum:
		p.next()
		f, _ := strconv.ParseFloat(t.v, 64)
		return &NumExpr{f}

	case tLParen:
		p.next()
		e := p.parse()
		if p.peek().k == tRParen {
			p.next()
		}
		return e

	case tIdent:
		p.next()
		// Field access: p.dim or m.dim
		if p.peek().k == tDot {
			p.next() // consume '.'
			field := p.next()
			return &FieldExpr{Src: t.v, Dim: field.v}
		}
		// Standalone identifier — treat as zero (future: variable lookup)
		return &NumExpr{0}
	}
	return &NumExpr{0}
}

// ParseExpr is the public entry point.
// Parses an arithmetic expression string that may reference p.dim and m.dim.
func ParseExpr(s string) Expr {
	tokens := tokenize(s)
	ep := &exprParser{toks: tokens}
	return ep.parse()
}

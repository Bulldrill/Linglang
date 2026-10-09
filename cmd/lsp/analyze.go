// cmd/lsp/analyze.go — turns a .lin document into LSP diagnostics and
// hover info by actually running it through parser.Runtime, line by line,
// rather than re-implementing a parallel analyzer (issue #31).
package main

import (
	"bytes"
	"io"
	"os"
	"strconv"
	"strings"

	"linlang-go/parser"
)

// analyzeDocument runs text through a fresh Runtime one ParseLine call at
// a time, capturing what each line printed to stdout: a "[❌]" line
// becomes an Error diagnostic, a "[⚠️]" line becomes a Warning, both
// attributed to the exact source line that produced them — something
// Runtime's own error reporting (print-and-continue) doesn't track
// per-line, so the LSP server recovers it from the side rather than
// requiring a parser.go change to carry positions through.
func analyzeDocument(text string) []diagnostic {
	lines := strings.Split(text, "\n")
	rt := parser.NewRuntime()

	var diags []diagnostic
	for i, line := range lines {
		output := captureStdout(func() { rt.ParseLine(line) })
		for _, out := range strings.Split(output, "\n") {
			switch {
			case strings.Contains(out, "[❌]"):
				diags = append(diags, diagnostic{
					Range:    lineRange(i, len(line)),
					Severity: severityError,
					Source:   "linlang",
					Message:  cleanDiagnosticText(out),
				})
			case strings.Contains(out, "[⚠️]"):
				diags = append(diags, diagnostic{
					Range:    lineRange(i, len(line)),
					Severity: severityWarning,
					Source:   "linlang",
					Message:  cleanDiagnosticText(out),
				})
			}
		}
	}
	return diags
}

func lineRange(line, length int) rangeT {
	return rangeT{Start: position{Line: line, Character: 0}, End: position{Line: line, Character: length}}
}

func cleanDiagnosticText(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "[❌] ")
	s = strings.TrimPrefix(s, "[⚠️] ")
	return s
}

// captureStdout redirects os.Stdout for the duration of fn and returns
// everything written to it. Runtime.ParseLine reports errors via
// fmt.Printf rather than a return value (by original design, unrelated to
// the LSP), so this is how the server observes them without changing that.
func captureStdout(fn func()) string {
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		fn()
		return ""
	}
	os.Stdout = w

	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		done <- buf.String()
	}()

	fn()

	w.Close()
	os.Stdout = orig
	return <-done
}

// hoverInfo runs the full document (to build up declared spaces/
// transforms/vectors) and returns a description for the identifier at
// (line, char), or "" if there isn't one worth showing.
func hoverInfo(text string, line, char int) string {
	lines := strings.Split(text, "\n")
	if line < 0 || line >= len(lines) {
		return ""
	}

	rt := parser.NewRuntime()
	captureStdout(func() { rt.Parse(text) })

	word := wordAt(lines[line], char)
	if word == "" {
		return ""
	}

	if sp, ok := rt.Spaces[word]; ok {
		dims := make([]string, len(sp.Dimensions))
		for i, d := range sp.Dimensions {
			dims[i] = d + ": " + sp.DimType(d).String()
		}
		return "space " + word + ": " + strings.Join(dims, ", ")
	}
	if tx, ok := rt.Transforms[word]; ok {
		dom2 := "(unario)"
		if tx.Domain2 != nil {
			dom2 = tx.Domain2.Name
		}
		dom1 := "?"
		if tx.Domain1 != nil {
			dom1 = tx.Domain1.Name
		}
		cod := "?"
		if tx.Codomain != nil {
			cod = tx.Codomain.Name
		}
		return "transform " + word + ": " + dom1 + " x " + dom2 + " -> " + cod
	}
	if v, ok := rt.Vectors[word]; ok {
		return word + " : " + v.Space.Name + " " + fmtDisplay(v.Display())
	}
	if h, ok := rt.HilbertSpaces[word]; ok {
		return "hilbert " + word + ": dim " + strconv.Itoa(h.Dim)
	}
	if _, ok := rt.Gates[word]; ok {
		return "gate " + word
	}
	return ""
}

func fmtDisplay(vals []any) string {
	parts := make([]string, len(vals))
	for i, v := range vals {
		switch x := v.(type) {
		case string:
			parts[i] = strconv.Quote(x)
		case float64:
			parts[i] = strconv.FormatFloat(x, 'g', -1, 64)
		default:
			parts[i] = ""
		}
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// wordAt returns the identifier-like token (letters, digits, underscore)
// spanning character column char in line.
func wordAt(line string, char int) string {
	runes := []rune(line)
	if char < 0 || char > len(runes) {
		return ""
	}
	isWord := func(r rune) bool {
		return r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
	}
	start, end := char, char
	for start > 0 && isWord(runes[start-1]) {
		start--
	}
	for end < len(runes) && isWord(runes[end]) {
		end++
	}
	if start == end {
		return ""
	}
	return string(runes[start:end])
}

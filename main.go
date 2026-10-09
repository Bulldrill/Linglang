package main

import (
	"bufio"
	"fmt"
	"os"

	"linlang-go/parser"
	"linlang-go/store"
)

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "-i" || os.Args[1] == "repl") {
		runREPL()
		return
	}

	filename := "examples/ejemplo.lin"
	if len(os.Args) > 1 {
		filename = os.Args[1]
	}

	code, err := os.ReadFile(filename)
	if err != nil {
		fmt.Println("Error leyendo archivo:", err)
		return
	}

	rt := parser.NewRuntime()
	rt.Parse(string(code))
}

// runREPL implements the interactive LinLang shell (issue #30): a single
// long-lived Runtime fed one line at a time via ParseLine. Multi-line
// blocks (transform/gate/for/try) need no special REPL-side buffering —
// Runtime.ParseLine already tracks that state across calls, which is
// exactly what lets a REPL just forward each line as it's typed; InBlock
// only drives the continuation-prompt display.
func runREPL() {
	rt := parser.NewRuntime()
	if db, err := store.Open(""); err == nil {
		rt.SetStore(db)
		defer db.Close()
	}

	fmt.Println("LinLang REPL — 'exit' o Ctrl+D para salir")
	scanner := bufio.NewScanner(os.Stdin)
	for {
		if rt.InBlock() {
			fmt.Print("...     ")
		} else {
			fmt.Print("linlang> ")
		}
		if !scanner.Scan() {
			fmt.Println()
			break
		}
		line := scanner.Text()
		if !rt.InBlock() && (line == "exit" || line == "quit") {
			break
		}
		rt.ParseLine(line)
	}
}

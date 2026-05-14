package main

import (
	"fmt"
	"os"

	"linlang-go/parser"
)

func main() {
	filename := "ejemplo.lin"
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

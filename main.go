package main

import (
	"fmt"
	"os"

	"linlang-go/core"
	"linlang-go/parser"
)

func main() {
	rt := parser.NewRuntime()
	rt.Spaces["Adopciones"] = core.NewSpace("Adopciones", []string{"persona_id", "mascota_id", "compat"})
	rt.Transforms["adoptar"] = core.NewTransform("adoptar",
		rt.Spaces["Personas"], rt.Spaces["Mascotas"], rt.Spaces["Adopciones"],
		func(p, m *core.Vector) *core.Vector {
			compat := p.Values[2] * m.Values[0] / 10
			return core.NewVector(rt.Spaces["Adopciones"], []float64{float64(p.ID), float64(m.ID), compat})
		})
	code, err := os.ReadFile("ejemplo.lin")
	if err != nil {
		fmt.Println("Error leyendo archivo:", err)
		return
	}
	rt.Parse(string(code))
}
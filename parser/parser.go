package parser

import (
    "fmt"
    "strings"
    "strconv"
    "linlang-go/core"
)

type Runtime struct {
    Spaces     map[string]*core.Space
    Vectors    map[string]*core.Vector
    Transforms map[string]*core.Transform
    LastCond   *core.Conditional
}

func NewRuntime() *Runtime {
    return &Runtime{
        Spaces:     map[string]*core.Space{},
        Vectors:    map[string]*core.Vector{},
        Transforms: map[string]*core.Transform{},
    }
}

func (rt *Runtime) ParseLine(line string) {
    line = strings.TrimSpace(line)
    if strings.HasPrefix(line, "space") {
        parts := strings.Split(line, ":")
        name := strings.Fields(parts[0])[1]
        dims := []string{}
        for _, d := range strings.Split(parts[1], ",") {
            dims = append(dims, strings.Split(strings.TrimSpace(d), ":")[0])
        }
        rt.Spaces[name] = core.NewSpace(name, dims)
        fmt.Println("[✔️] Espacio creado:", name)
    } else if strings.HasPrefix(line, "let") {
        left, right := strings.Split(line, "=")[0], strings.Split(line, "=")[1]
        varName := strings.TrimSpace(strings.Fields(left)[1])
        if strings.Contains(right, "[") {
            spName := strings.TrimSpace(right[:strings.Index(right, "[")])
            vals := strings.Trim(right[strings.Index(right, "[")+1:strings.Index(right, "]")], " ")
            nums := []float64{}
            for _, v := range strings.Split(vals, ",") {
                f, _ := strconv.ParseFloat(strings.TrimSpace(v), 64)
                nums = append(nums, f)
            }
            rt.Vectors[varName] = core.NewVector(rt.Spaces[spName], nums)
            fmt.Printf("[✔️] Vector %s creado\n", varName)
        } else if strings.Contains(right, "(") {
            fn := right[:strings.Index(right, "(")]
            args := strings.Split(right[strings.Index(right, "(")+1:strings.Index(right, ")")], ",")
            v1 := rt.Vectors[strings.TrimSpace(args[0])]
            v2 := rt.Vectors[strings.TrimSpace(args[1])]
            rt.Vectors[varName] = rt.Transforms[fn].Apply(v1, v2)
            fmt.Printf("[✔️] Vector %s generado vía transformación %s\n", varName, fn)
        }
    } else if strings.HasPrefix(line, "when") {
        cond := line[5:strings.Index(line, ">")]
        value := strings.TrimSpace(line[strings.Index(line, ">")+1:])
        compVal, _ := strconv.ParseFloat(value, 64)
        vecName, dim := strings.Split(strings.TrimSpace(cond), ".")[0], strings.Split(strings.TrimSpace(cond), ".")[1]
        v := rt.Vectors[vecName]
        rt.LastCond = core.NewConditional(v, dim, func(x float64) bool { return x > compVal }, func() {})
        fmt.Printf("[✔️] Condición registrada sobre %s.%s > %f\n", vecName, dim, compVal)
    } else if strings.HasPrefix(line, "approve") {
        varName := strings.TrimSpace(line[strings.Index(line, "(")+1 : strings.Index(line, ")")])
        rt.LastCond.Action = func() {
            fmt.Println("[✅] Aprobado:", rt.Vectors[varName])
        }
        rt.LastCond.Evaluate()
    }
}

func (rt *Runtime) Parse(code string) {
    for _, line := range strings.Split(code, "\n") {
        if strings.TrimSpace(line) != "" && !strings.HasPrefix(strings.TrimSpace(line), "#") {
            rt.ParseLine(line)
        }
    }
}

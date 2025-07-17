
package core

import (
    "fmt"
    "errors"
)

var vectorIDCounter int64 = 0

func nextID() int64 {
    vectorIDCounter++
    return vectorIDCounter
}

type Vector struct {
    ID int64
    Space *Space
    Values []float64
}

func NewVector(space *Space, values []float64) *Vector {
    if len(values) != len(space.Dimensions) {
        panic("dimensiones incorrectas")
    }
    return &Vector{
        ID: nextID(),
        Space: space,
        Values: values,
    }
}

func (v *Vector) String() string {
    return fmt.Sprintf("%s%v", v.Space.Name, v.Values)
}

func (v *Vector) Add(o *Vector) (*Vector, error) {
    if v.Space.Name != o.Space.Name {
        return nil, errors.New("espacios incompatibles")
    }
    res := make([]float64, len(v.Values))
    for i := range v.Values {
        res[i] = v.Values[i] + o.Values[i]
    }
    return NewVector(v.Space, res), nil
}

func (v *Vector) Dot(o *Vector) float64 {
    sum := 0.0
    for i := range v.Values {
        sum += v.Values[i] * o.Values[i]
    }
    return sum
}

func (v *Vector) Get(dim string) float64 {
    for i, d := range v.Space.Dimensions {
        if d == dim {
            return v.Values[i]
        }
    }
    panic("dimensión no encontrada")
}

package core

import (
	"errors"
	"fmt"
	"math"
)

var vectorIDCounter int64 = 0

func nextID() int64 {
	vectorIDCounter++
	return vectorIDCounter
}

type Vector struct {
	ID      int64
	Space   *Space
	Values  []float64
	Strings map[string]string // dimension name -> value, for String-typed dims only (issue #24)
}

func NewVector(space *Space, values []float64) *Vector {
	if len(values) != len(space.Dimensions) {
		panic("dimensiones incorrectas")
	}
	return &Vector{
		ID:     nextID(),
		Space:  space,
		Values: values,
	}
}

func (v *Vector) String() string {
	return fmt.Sprintf("%s%v", v.Space.Name, v.Display())
}

// SetString assigns a String-typed dimension's value.
func (v *Vector) SetString(dim, value string) {
	if v.Strings == nil {
		v.Strings = map[string]string{}
	}
	v.Strings[dim] = value
}

// GetString returns a String-typed dimension's value.
func (v *Vector) GetString(dim string) (string, bool) {
	if v.Strings == nil {
		return "", false
	}
	s, ok := v.Strings[dim]
	return s, ok
}

// Display returns v's per-dimension values for human-readable output:
// a string for String-typed dimensions, a float64 for everything else.
// Values[i] is 0 (unused) at any String-typed dimension's slot.
func (v *Vector) Display() []any {
	out := make([]any, len(v.Values))
	for i, dim := range v.Space.Dimensions {
		if v.Space.DimType(dim) == String {
			if s, ok := v.GetString(dim); ok {
				out[i] = s
				continue
			}
		}
		out[i] = v.Values[i]
	}
	return out
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
	panic("dimensión no encontrada: " + dim)
}

// Scale returns a new vector whose components are multiplied by factor.
func (v *Vector) Scale(factor float64) *Vector {
	res := make([]float64, len(v.Values))
	for i, val := range v.Values {
		res[i] = val * factor
	}
	return NewVector(v.Space, res)
}

// Norm returns the Euclidean (L2) magnitude of the vector.
func (v *Vector) Norm() float64 {
	sum := 0.0
	for _, val := range v.Values {
		sum += val * val
	}
	return math.Sqrt(sum)
}

// Project returns a new vector in targetSpace whose dimensions are populated
// by matching dimension names from v. Dimensions absent in v default to 0.
func (v *Vector) Project(target *Space) (*Vector, error) {
	vals := make([]float64, len(target.Dimensions))
	for i, dim := range target.Dimensions {
		for j, d := range v.Space.Dimensions {
			if d == dim {
				vals[i] = v.Values[j]
				break
			}
		}
		// If the dimension does not exist in v, it stays 0 (zero component)
	}
	return NewVector(target, vals), nil
}

package core

import "fmt"

// DimType is the declared type of a Space dimension. Real participates in
// vector arithmetic (Add, Dot, Scale, Norm, Project); String does not — it
// is carried alongside as metadata (a task's title, a category's label),
// the same role a text column plays next to numeric columns in a SQL
// table (issue #24).
type DimType int

const (
	Real DimType = iota
	String
)

func (t DimType) String() string {
	if t == String {
		return "String"
	}
	return "Real"
}

type Space struct {
	Name       string
	Dimensions []string
	Types      []DimType // parallel to Dimensions; missing/short entries default to Real
}

// NewSpace declares a space whose dimensions are all Real — the pre-#24
// behaviour, preserved for every existing caller.
func NewSpace(name string, dimensions []string) *Space {
	return &Space{Name: name, Dimensions: dimensions, Types: make([]DimType, len(dimensions))}
}

// NewTypedSpace declares a space with an explicit type per dimension.
func NewTypedSpace(name string, dimensions []string, types []DimType) *Space {
	return &Space{Name: name, Dimensions: dimensions, Types: types}
}

// DimType returns dim's declared type, defaulting to Real if dim is
// unknown or Types was never populated for it.
func (s *Space) DimType(dim string) DimType {
	for i, d := range s.Dimensions {
		if d == dim {
			if i < len(s.Types) {
				return s.Types[i]
			}
			return Real
		}
	}
	return Real
}

func (s *Space) String() string {
	return fmt.Sprintf("Space(%s, dims=%v)", s.Name, s.Dimensions)
}

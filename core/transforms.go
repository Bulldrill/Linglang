package core

type Transform struct {
	Name     string
	Domain1  *Space
	Domain2  *Space
	Codomain *Space
	Function func(*Vector, *Vector) *Vector
}

func NewTransform(name string, d1, d2, cod *Space, f func(*Vector, *Vector) *Vector) *Transform {
	return &Transform{
		Name:     name,
		Domain1:  d1,
		Domain2:  d2,
		Codomain: cod,
		Function: f,
	}
}

func (t *Transform) Apply(v1, v2 *Vector) *Vector {
	return t.Function(v1, v2)
}

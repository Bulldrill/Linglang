
package core

import "fmt"

type Space struct {
    Name       string
    Dimensions []string
}

func NewSpace(name string, dimensions []string) *Space {
    return &Space{Name: name, Dimensions: dimensions}
}

func (s *Space) String() string {
    return fmt.Sprintf("Space(%s, dims=%v)", s.Name, s.Dimensions)
}

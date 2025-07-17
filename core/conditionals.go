
package core

import "fmt"

type Conditional struct {
    Vector *Vector
    Dim string
    Condition func(float64) bool
    Action func()
}

func NewConditional(v *Vector, dim string, cond func(float64) bool, action func()) *Conditional {
    return &Conditional{v, dim, cond, action}
}

func (c *Conditional) Evaluate() {
    val := c.Vector.Get(c.Dim)
    if c.Condition(val) {
        fmt.Printf("[✔️] Condición cumplida sobre %s.%s\n", c.Vector.Space.Name, c.Dim)
        c.Action()
    } else {
        fmt.Printf("[❌] Condición NO cumplida sobre %s.%s\n", c.Vector.Space.Name, c.Dim)
    }
}

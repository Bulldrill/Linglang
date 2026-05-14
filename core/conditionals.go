package core

import "fmt"

// Conditional represents a guarded action: when getValue() satisfies Condition,
// execute Action.  The Label is used for diagnostic output.
//
// Using a closure for GetValue (rather than a Vector+Dim pair) lets the same
// type serve both classical vector conditions and quantum property conditions
// such as purity(ρ) or qnorm(|ψ⟩).
type Conditional struct {
	Label     string
	GetValue  func() float64
	Condition func(float64) bool
	Action    func()
}

// NewConditional constructs a general-purpose conditional.
func NewConditional(label string, getValue func() float64, cond func(float64) bool, action func()) *Conditional {
	return &Conditional{
		Label:     label,
		GetValue:  getValue,
		Condition: cond,
		Action:    action,
	}
}

// NewVectorConditional is a convenience constructor for classical vector conditions.
func NewVectorConditional(v *Vector, dim string, cond func(float64) bool, action func()) *Conditional {
	label := fmt.Sprintf("%s.%s", v.Space.Name, dim)
	return NewConditional(label, func() float64 { return v.Get(dim) }, cond, action)
}

// Evaluate tests the condition and executes the action if it is satisfied.
func (c *Conditional) Evaluate() {
	val := c.GetValue()
	if c.Condition(val) {
		fmt.Printf("[✔️] Condición cumplida [%s = %.6f]\n", c.Label, val)
		c.Action()
	} else {
		fmt.Printf("[❌] Condición NO cumplida [%s = %.6f]\n", c.Label, val)
	}
}

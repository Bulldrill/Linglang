package core

import "math/rand"

// Shot performs one stochastic measurement of q using the Born rule:
// outcome i is drawn with probability |αᵢ|², via inverse-CDF sampling over
// Probabilities(). Unlike Measure (deterministic argmax, kept for
// reproducibility by existing callers such as Teleport), Shot is what real
// quantum hardware actually produces on every individual run.
func (q *QuantumState) Shot(rng *rand.Rand) int {
	probs := q.Probabilities()
	r := rng.Float64()
	cum := 0.0
	for i, p := range probs {
		cum += p
		if r < cum {
			return i
		}
	}
	return len(probs) - 1 // floating-point guard for r landing exactly at 1.0
}

// Shots runs n independent Shot() samples and returns the resulting
// outcome histogram — the same shape CircuitRunner.Run (real hardware)
// returns, so simulator results and hardware results are directly
// comparable.
func (q *QuantumState) Shots(n int, rng *rand.Rand) map[int]int {
	counts := make(map[int]int, len(q.Amplitudes))
	for i := 0; i < n; i++ {
		counts[q.Shot(rng)]++
	}
	return counts
}

package core

import "testing"

// countingChannel wraps a QuantumChannel and counts ShareBellPair calls, so
// tests can tell a pool hit from a pool miss.
type countingChannel struct {
	QuantumChannel
	shareCalls int
}

func (c *countingChannel) ShareBellPair() (*BellPair, error) {
	c.shareCalls++
	return c.QuantumChannel.ShareBellPair()
}

func TestBellPoolRefillPreGeneratesPairs(t *testing.T) {
	inner := &countingChannel{QuantumChannel: NewLocalChannel("alice", "bob", NewHilbertSpace("Q", 2))}
	pool := NewBellPool(inner, 3)

	if err := pool.Refill(); err != nil {
		t.Fatalf("Refill error: %v", err)
	}
	if inner.shareCalls != 3 {
		t.Fatalf("expected 3 ShareBellPair calls during Refill, got %d", inner.shareCalls)
	}
	if pool.Available() != 3 {
		t.Fatalf("expected 3 buffered pairs, got %d", pool.Available())
	}
}

func TestBellPoolShareBellPairServesFromBufferFirst(t *testing.T) {
	inner := &countingChannel{QuantumChannel: NewLocalChannel("alice", "bob", NewHilbertSpace("Q", 2))}
	pool := NewBellPool(inner, 2)
	if err := pool.Refill(); err != nil {
		t.Fatalf("Refill error: %v", err)
	}
	callsAfterRefill := inner.shareCalls

	for i := 0; i < 2; i++ {
		if _, err := pool.ShareBellPair(); err != nil {
			t.Fatalf("ShareBellPair error: %v", err)
		}
	}
	if inner.shareCalls != callsAfterRefill {
		t.Fatalf("buffered pairs should not trigger new ShareBellPair calls: before=%d after=%d",
			callsAfterRefill, inner.shareCalls)
	}
	if pool.Available() != 0 {
		t.Fatalf("expected buffer to be drained, got %d remaining", pool.Available())
	}

	// Pool miss: buffer empty, falls back to the wrapped channel.
	if _, err := pool.ShareBellPair(); err != nil {
		t.Fatalf("ShareBellPair (miss) error: %v", err)
	}
	if inner.shareCalls != callsAfterRefill+1 {
		t.Fatalf("expected exactly one extra ShareBellPair call on pool miss, got %d -> %d",
			callsAfterRefill, inner.shareCalls)
	}
}

func TestBellPoolWorksAsATeleportChannel(t *testing.T) {
	q := NewHilbertSpace("Q", 2)
	pool := NewBellPool(NewLocalChannel("alice", "bob", q), 4)
	if err := pool.Refill(); err != nil {
		t.Fatalf("Refill error: %v", err)
	}

	psi := NewQuantumState(q, []complex128{0.6, 0.8})
	result, err := Teleport(psi, pool) // BellPool satisfies QuantumChannel directly
	if err != nil {
		t.Fatalf("Teleport over a BellPool error: %v", err)
	}
	for i := range psi.Amplitudes {
		if result.State.Amplitudes[i] != psi.Amplitudes[i] {
			// allow tiny floating point drift, matching core/teleport_test.go
			diff := result.State.Amplitudes[i] - psi.Amplitudes[i]
			if r, im := real(diff), imag(diff); r*r+im*im > 1e-18 {
				t.Fatalf("teleport over pooled channel mismatch at %d: got %v want %v", i, result.State.Amplitudes, psi.Amplitudes)
			}
		}
	}
	if pool.Available() != 3 {
		t.Fatalf("expected the pool to have served 1 pre-provisioned pair (3 left), got %d", pool.Available())
	}
}

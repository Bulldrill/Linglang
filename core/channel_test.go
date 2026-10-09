package core

import "testing"

func TestLocalChannelSharesValidBellPair(t *testing.T) {
	qubit := NewHilbertSpace("Q", 2)
	ch := NewLocalChannel("alice", "bob", qubit)
	pair, err := ch.ShareBellPair()
	if err != nil {
		t.Fatalf("ShareBellPair error: %v", err)
	}
	if pair.Space.Dim != 4 {
		t.Fatalf("expected dim-4 joint space, got %d", pair.Space.Dim)
	}
	if n := pair.State.Norm(); n < 0.999999 || n > 1.000001 {
		t.Fatalf("Bell pair state not normalised: norm=%f", n)
	}
	probs := pair.State.Probabilities()
	if probs[1] > 1e-9 || probs[2] > 1e-9 {
		t.Fatalf("expected zero amplitude on |01>,|10>, got probs=%v", probs)
	}
}

func TestLocalChannelClassicalRoundTrip(t *testing.T) {
	ch := NewLocalChannel("alice", "bob", NewHilbertSpace("Q", 2))
	if err := ch.SendClassical("alice", []int{1, 0}); err != nil {
		t.Fatalf("SendClassical error: %v", err)
	}
	bits, err := ch.RecvClassical("bob")
	if err != nil {
		t.Fatalf("RecvClassical error: %v", err)
	}
	if bits[0] != 1 || bits[1] != 0 {
		t.Fatalf("got bits=%v, want [1 0]", bits)
	}
	if _, err := ch.RecvClassical("bob"); err == nil {
		t.Fatal("expected error receiving from an empty inbox")
	}
}

func TestLocalChannelRejectsUnknownNode(t *testing.T) {
	ch := NewLocalChannel("alice", "bob", NewHilbertSpace("Q", 2))
	if err := ch.SendClassical("mallory", []int{1}); err == nil {
		t.Fatal("expected error sending from an unknown node")
	}
}

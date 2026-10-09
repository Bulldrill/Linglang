package core

import "testing"

func TestHilbertRegistryRegisterAndOwner(t *testing.T) {
	r := NewHilbertRegistry()
	q := NewHilbertSpace("Qubit", 2)
	r.Register(q, NodeID("alice"))

	owner, err := r.Owner("Qubit")
	if err != nil {
		t.Fatalf("Owner error: %v", err)
	}
	if owner != "alice" {
		t.Fatalf("expected owner 'alice', got %q", owner)
	}

	local, err := r.Local("Qubit", "alice")
	if err != nil || !local {
		t.Fatalf("expected Local('Qubit','alice')=true, got %v, err=%v", local, err)
	}
	local, err = r.Local("Qubit", "bob")
	if err != nil || local {
		t.Fatalf("expected Local('Qubit','bob')=false, got %v, err=%v", local, err)
	}
}

func TestHilbertRegistryTransferMigratesOwnership(t *testing.T) {
	r := NewHilbertRegistry()
	q := NewHilbertSpace("Qubit", 2)
	r.Register(q, NodeID("alice"))

	if err := r.Transfer("Qubit", "bob"); err != nil {
		t.Fatalf("Transfer error: %v", err)
	}
	owner, err := r.Owner("Qubit")
	if err != nil || owner != "bob" {
		t.Fatalf("expected owner 'bob' after transfer, got %q, err=%v", owner, err)
	}
}

func TestHilbertRegistryUnknownSpaceErrors(t *testing.T) {
	r := NewHilbertRegistry()
	if _, err := r.Owner("Nope"); err == nil {
		t.Fatal("expected error for unregistered space")
	}
	if err := r.Transfer("Nope", "bob"); err == nil {
		t.Fatal("expected error transferring an unregistered space")
	}
}

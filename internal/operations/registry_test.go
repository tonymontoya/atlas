package operations

import (
	"testing"
)

func defaultTestRegistry(t *testing.T) *Registry {
	t.Helper()
	registry, err := NewRegistry(CollectHostEvidence{}, DestroyOSD{}, VerifyOSD{})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	return registry
}

func TestRegistryResolvesOperationTypes(t *testing.T) {
	registry, err := NewRegistry(CollectHostEvidence{})
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	op, ok := registry.Get("CollectHostEvidence")
	if !ok {
		t.Fatal("CollectHostEvidence not registered")
	}
	if op.OperationType() != "CollectHostEvidence" {
		t.Fatalf("operation type = %q, want CollectHostEvidence", op.OperationType())
	}
	if _, ok := registry.Get("RestartDaemon"); ok {
		t.Fatal("unregistered operation resolved")
	}
}

func TestRegistryRejectsDuplicateOperationTypes(t *testing.T) {
	_, err := NewRegistry(CollectHostEvidence{}, CollectHostEvidence{})
	if err == nil {
		t.Fatal("accepted a duplicate operation type")
	}
}

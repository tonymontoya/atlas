package workflows

import (
	"testing"
)

func TestReplaceOSDDefinition(t *testing.T) {
	definition := ReplaceOSD()

	if definition.ID != "replace-osd" {
		t.Fatalf("id = %q, want replace-osd", definition.ID)
	}
	if definition.Version != 1 {
		t.Fatalf("version = %d, want 1", definition.Version)
	}

	want := []Step{
		JobStep{
			ID:            "collect-evidence",
			OperationType: "CollectHostEvidence",
			Retry:         RetryPolicy{MaxAttempts: 3},
		},
		ApprovalGate{ID: "approve-destroy"},
		JobStep{
			ID:            "destroy-osd",
			OperationType: "DestroyOSD",
			Retry:         RetryPolicy{MaxAttempts: 1},
		},
		TaskStep{
			ID:      "replace-device",
			Summary: "Replace the failed Storage Device",
		},
		JobStep{
			ID:            "verify-osd",
			OperationType: "VerifyOSD",
			Retry:         RetryPolicy{MaxAttempts: 3},
		},
	}
	if len(definition.Steps) != len(want) {
		t.Fatalf("steps = %d, want %d: %v", len(definition.Steps), len(want), definition.Steps)
	}
	for i, step := range definition.Steps {
		if step != want[i] {
			t.Fatalf("step %d = %#v, want %#v", i, step, want[i])
		}
	}
}

func TestDefaultRegistryRegistersReplaceOSD(t *testing.T) {
	var registry Registry
	registry, err := DefaultRegistry(testOperations(t))
	if err != nil {
		t.Fatalf("DefaultRegistry: %v", err)
	}

	definition, ok := registry.Get("replace-osd", 1)
	if !ok {
		t.Fatal("replace-osd v1 not registered")
	}
	if definition.ID != "replace-osd" {
		t.Fatalf("id = %q, want replace-osd", definition.ID)
	}
}

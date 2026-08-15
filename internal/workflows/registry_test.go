package workflows

import (
	"strings"
	"testing"

	"github.com/tonymontoya/ceph-atlas/internal/operations"
)

func testOperations(t *testing.T) *operations.Registry {
	t.Helper()
	registry, err := operations.DefaultRegistry()
	if err != nil {
		t.Fatalf("operations.DefaultRegistry: %v", err)
	}
	return registry
}

func TestCodeRegistryResolvesDefinitionByIDAndVersion(t *testing.T) {
	definition := Definition{
		ID:      "replace-osd",
		Version: 1,
		Steps: []Step{
			JobStep{
				ID:            "collect-evidence",
				OperationType: "CollectHostEvidence",
				Retry:         RetryPolicy{MaxAttempts: 3},
			},
		},
	}
	registry, err := NewCodeRegistry(testOperations(t), definition)
	if err != nil {
		t.Fatalf("NewCodeRegistry: %v", err)
	}

	got, ok := registry.Get("replace-osd", 1)
	if !ok {
		t.Fatal("replace-osd v1 not registered")
	}
	if got.ID != "replace-osd" || got.Version != 1 {
		t.Fatalf("definition = %s v%d, want replace-osd v1", got.ID, got.Version)
	}
	if _, ok := registry.Get("replace-osd", 2); ok {
		t.Fatal("unregistered version resolved")
	}
	if _, ok := registry.Get("drain-host", 1); ok {
		t.Fatal("unregistered definition resolved")
	}
}

func TestCodeRegistryRejectsUnresolvableJobOperations(t *testing.T) {
	emptyOperations, err := operations.NewRegistry()
	if err != nil {
		t.Fatalf("operations.NewRegistry: %v", err)
	}
	definition := Definition{
		ID:      "replace-osd",
		Version: 1,
		Steps: []Step{
			JobStep{
				ID:            "collect-evidence",
				OperationType: "CollectHostEvidence",
				Retry:         RetryPolicy{MaxAttempts: 3},
			},
		},
	}

	_, err = NewCodeRegistry(emptyOperations, definition)
	if err == nil {
		t.Fatal("accepted a Job step whose operation type does not resolve")
	}
}

func validDefinition(t *testing.T) Definition {
	t.Helper()
	return Definition{
		ID:      "replace-osd",
		Version: 1,
		Steps: []Step{
			JobStep{
				ID:            "collect-evidence",
				OperationType: "CollectHostEvidence",
				Retry:         RetryPolicy{MaxAttempts: 3},
			},
		},
	}
}

func TestNewCodeRegistryRejectsInvalidDefinitions(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*Definition)
		wantError string
	}{
		{"missing definition id", func(d *Definition) { d.ID = "" }, "definition id is required"},
		{"zero definition version", func(d *Definition) { d.Version = 0 }, "definition version must be positive"},
		{"no steps", func(d *Definition) { d.Steps = nil }, "definition must declare at least one step"},
		{"job without id", func(d *Definition) {
			job := d.Steps[0].(JobStep)
			job.ID = ""
			d.Steps[0] = job
		}, "step id is required"},
		{"job without operation type", func(d *Definition) {
			job := d.Steps[0].(JobStep)
			job.OperationType = ""
			d.Steps[0] = job
		}, "operation type is required"},
		{"job without retry attempts", func(d *Definition) {
			job := d.Steps[0].(JobStep)
			job.Retry = RetryPolicy{MaxAttempts: 0}
			d.Steps[0] = job
		}, "maxAttempts must be positive"},
		{"duplicate step ids", func(d *Definition) {
			d.Steps = append(d.Steps, JobStep{
				ID:            "collect-evidence",
				OperationType: "VerifyOSD",
				Retry:         RetryPolicy{MaxAttempts: 3},
			})
		}, "step id \"collect-evidence\" used twice"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			definition := validDefinition(t)
			test.mutate(&definition)

			_, err := NewCodeRegistry(testOperations(t), definition)
			if err == nil {
				t.Fatalf("accepted %s", test.name)
			}
			if !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("error = %q, want it to mention %q", err, test.wantError)
			}
		})
	}
}

func TestNewCodeRegistryRejectsDuplicateDefinitions(t *testing.T) {
	_, err := NewCodeRegistry(testOperations(t), validDefinition(t), validDefinition(t))
	if err == nil {
		t.Fatal("accepted the same definition id and version twice")
	}
}

func TestCodeRegistryAcceptsApprovalGatesAndTaskSteps(t *testing.T) {
	definition := Definition{
		ID:      "replace-osd",
		Version: 1,
		Steps: []Step{
			JobStep{
				ID:            "collect-evidence",
				OperationType: "CollectHostEvidence",
				Retry:         RetryPolicy{MaxAttempts: 3},
			},
			ApprovalGate{ID: "approve-destroy"},
			TaskStep{ID: "replace-device", Summary: "Replace the failed Storage Device"},
		},
	}
	registry, err := NewCodeRegistry(testOperations(t), definition)
	if err != nil {
		t.Fatalf("NewCodeRegistry: %v", err)
	}

	got, ok := registry.Get("replace-osd", 1)
	if !ok {
		t.Fatal("replace-osd v1 not registered")
	}
	if len(got.Steps) != 3 {
		t.Fatalf("steps = %d, want 3", len(got.Steps))
	}
	if _, ok := got.Steps[1].(ApprovalGate); !ok {
		t.Fatalf("step 1 = %T, want ApprovalGate", got.Steps[1])
	}
	if _, ok := got.Steps[2].(TaskStep); !ok {
		t.Fatalf("step 2 = %T, want TaskStep", got.Steps[2])
	}
}

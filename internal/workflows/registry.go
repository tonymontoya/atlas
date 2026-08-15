package workflows

import (
	"errors"
	"fmt"

	"github.com/tonymontoya/ceph-atlas/internal/operations"
)

// Definition is a versioned Workflow definition (ADR-0017): ordered steps
// compiled into Atlas and referenced by Workflow Instances as id plus
// version.
type Definition struct {
	ID      string
	Version int
	Steps   []Step
}

// Step is one ordered step in a Workflow definition: a Job specification,
// an Approval Gate, or a human Task.
type Step interface {
	step()
}

// JobStep is an executable Job specification referencing a typed operation
// from the agent operation contract (ADR-0018).
type JobStep struct {
	ID            string
	OperationType string
	Retry         RetryPolicy
}

func (JobStep) step() {}

// ApprovalGate is a Workflow Gate (ADR-0020): execution pauses at this
// step until an Approval bound to it allows the instance to continue.
type ApprovalGate struct {
	ID string
}

func (ApprovalGate) step() {}

// TaskStep is a unit of human work inside a Workflow definition; the
// instance pauses at it as waiting_for_operator (ADR-0019).
type TaskStep struct {
	ID      string
	Summary string
}

func (TaskStep) step() {}

// RetryPolicy governs how a failed Job transitions back to pending
// (ADR-0019).
type RetryPolicy struct {
	MaxAttempts int
}

// Registry resolves Workflow definitions by id and version. The seam a
// future DB-backed definition source can implement (ADR-0017).
type Registry interface {
	Get(id string, version int) (Definition, bool)
}

// CodeRegistry is a Registry built from definitions compiled into the
// Atlas binary.
type CodeRegistry struct {
	definitions map[definitionKey]Definition
}

type definitionKey struct {
	id      string
	version int
}

// NewCodeRegistry builds a CodeRegistry from definitions. Definitions are
// validated and every Job step's operation type must resolve in the typed
// operation registry, so an unresolvable reference fails here — at
// startup — rather than at dispatch time.
func NewCodeRegistry(ops *operations.Registry, definitions ...Definition) (*CodeRegistry, error) {
	registry := &CodeRegistry{
		definitions: make(map[definitionKey]Definition, len(definitions)),
	}
	for _, definition := range definitions {
		if err := definition.validate(ops); err != nil {
			return nil, fmt.Errorf("workflow %s v%d: %w", definition.ID, definition.Version, err)
		}
		key := definitionKey{id: definition.ID, version: definition.Version}
		if _, exists := registry.definitions[key]; exists {
			return nil, fmt.Errorf("workflow %s v%d registered twice", definition.ID, definition.Version)
		}
		registry.definitions[key] = definition
	}
	return registry, nil
}

func (d Definition) validate(ops *operations.Registry) error {
	if d.ID == "" {
		return errors.New("definition id is required")
	}
	if d.Version < 1 {
		return errors.New("definition version must be positive")
	}
	if len(d.Steps) == 0 {
		return errors.New("definition must declare at least one step")
	}
	stepIDs := make(map[string]bool, len(d.Steps))
	for _, step := range d.Steps {
		var stepID string
		switch step := step.(type) {
		case JobStep:
			stepID = step.ID
			if step.OperationType == "" {
				return fmt.Errorf("job %s: operation type is required", step.ID)
			}
			if _, ok := ops.Get(step.OperationType); !ok {
				return fmt.Errorf("job %q references unknown operation type %q", step.ID, step.OperationType)
			}
			if step.Retry.MaxAttempts < 1 {
				return fmt.Errorf("job %s: maxAttempts must be positive", step.ID)
			}
		case ApprovalGate:
			stepID = step.ID
		case TaskStep:
			stepID = step.ID
			if step.Summary == "" {
				return fmt.Errorf("task %s: summary is required", step.ID)
			}
		default:
			return fmt.Errorf("unsupported step type %T", step)
		}
		if stepID == "" {
			return errors.New("step id is required")
		}
		if stepIDs[stepID] {
			return fmt.Errorf("step id %q used twice", stepID)
		}
		stepIDs[stepID] = true
	}
	return nil
}

// Get returns the registered definition for an id and version.
func (r *CodeRegistry) Get(id string, version int) (Definition, bool) {
	definition, ok := r.definitions[definitionKey{id: id, version: version}]
	return definition, ok
}

package operations

import (
	"fmt"
	"reflect"
)

// Operation is the typed agent operation contract (ADR-0018): each
// operation is a struct with JSON-tagged, validated fields. Atlas Agents
// execute only registered operations (ADR-0003).
type Operation interface {
	// OperationType returns the stable operation type string.
	OperationType() string
	// Validate reports whether the operation's fields are valid.
	Validate() error
}

// Registry maps operation type strings to operation prototypes.
type Registry struct {
	prototypes map[string]Operation
}

// NewRegistry builds a Registry from operation prototypes. It rejects
// prototypes with an empty operation type, pointer prototypes (register
// values), and duplicate registrations.
func NewRegistry(prototypes ...Operation) (*Registry, error) {
	registry := &Registry{prototypes: make(map[string]Operation, len(prototypes))}
	for _, prototype := range prototypes {
		operationType := prototype.OperationType()
		if operationType == "" {
			return nil, fmt.Errorf("operation %T has an empty operation type", prototype)
		}
		if reflect.TypeOf(prototype).Kind() == reflect.Pointer {
			return nil, fmt.Errorf("operation %s must be registered as a value type", operationType)
		}
		if _, exists := registry.prototypes[operationType]; exists {
			return nil, fmt.Errorf("operation type %q registered twice", operationType)
		}
		registry.prototypes[operationType] = prototype
	}
	return registry, nil
}

// DefaultRegistry returns a fresh registry with the operations the
// Replace OSD Workflow needs. Adding an operation means adding a Go type
// and registering it here (ADR-0018).
func DefaultRegistry() (*Registry, error) {
	return NewRegistry(
		CollectHostEvidence{},
		DestroyOSD{},
		VerifyOSD{},
	)
}

// Get returns the registered prototype for an operation type.
func (r *Registry) Get(operationType string) (Operation, bool) {
	prototype, ok := r.prototypes[operationType]
	return prototype, ok
}

// OperationTypes returns the registered operation type strings.
func (r *Registry) OperationTypes() []string {
	types := make([]string, 0, len(r.prototypes))
	for operationType := range r.prototypes {
		types = append(types, operationType)
	}
	return types
}

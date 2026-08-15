package agent

import (
	"context"
	"fmt"

	"github.com/tonymontoya/ceph-atlas/internal/operations"
)

// Fake is the in-process fake Atlas Agent (ADR-0022): it decodes every
// request envelope through the real typed-operation registry and
// answers a deterministic happy-path outcome. It never touches a
// cluster. Failure scenarios for retry and idempotency work arrive with
// the resilience slice.
type Fake struct {
	registry *operations.Registry
}

// NewFake builds the fake agent over an operation registry.
func NewFake(registry *operations.Registry) *Fake {
	return &Fake{registry: registry}
}

// Dispatch decodes the envelope through the real wire path and reports
// the deterministic happy-path outcome for the operation it carries. A
// malformed or invalid envelope is an error: the dispatcher's requests
// must satisfy the typed-operation contract.
func (f *Fake) Dispatch(_ context.Context, envelope []byte) (Result, error) {
	request, err := operations.DecodeRequest(f.registry, envelope)
	if err != nil {
		return Result{}, fmt.Errorf("fake agent rejected request: %w", err)
	}
	return Result{
		Outcome: OutcomeSucceeded,
		Detail:  fmt.Sprintf("%s executed by the fake Atlas Agent", request.Envelope.OperationType),
	}, nil
}

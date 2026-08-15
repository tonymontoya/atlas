package agent

import (
	"context"
	"fmt"
	"sync"

	"github.com/tonymontoya/ceph-atlas/internal/operations"
)

// Fake scenario names (in the style of ATLAS_FAKE_SCENARIO). The default
// empty scenario answers the deterministic happy path.
const (
	// ScenarioDispatchFailsOnce fails the very first executed dispatch and
	// succeeds afterwards: a transient Job failure the retry policy
	// recovers from.
	ScenarioDispatchFailsOnce = "dispatch-fails-once"
	// ScenarioJobFailure fails every first execution of an idempotency
	// key, retries included: the retry budget exhausts and the instance
	// fails terminally.
	ScenarioJobFailure = "job-failure"
)

// Fake is the in-process fake Atlas Agent (ADR-0022): it decodes every
// request envelope through the real typed-operation registry and answers
// deterministic outcomes scripted by its scenario. It never touches a
// cluster. Like a contract-honest agent, it executes a Job at most once
// per idempotency key and replays the remembered outcome for duplicate
// dispatches, so a re-dispatch after a restart never repeats work
// (ADR-0018).
type Fake struct {
	registry   *operations.Registry
	scenario   string
	mu         sync.Mutex
	outcomes   map[string]Result
	executions int
	failedOnce bool
}

// NewFake builds the happy-path fake agent over an operation registry.
func NewFake(registry *operations.Registry) *Fake {
	return &Fake{registry: registry, outcomes: make(map[string]Result)}
}

// NewFakeWithScenario builds the fake agent with a failure scenario
// (ATLAS_FAKE_AGENT_SCENARIO). Unknown scenario names are rejected here,
// at startup, rather than mid-dispatch.
func NewFakeWithScenario(registry *operations.Registry, scenario string) (*Fake, error) {
	switch scenario {
	case "", ScenarioDispatchFailsOnce, ScenarioJobFailure:
	default:
		return nil, fmt.Errorf("unknown fake agent scenario %q", scenario)
	}
	return &Fake{
		registry: registry,
		scenario: scenario,
		outcomes: make(map[string]Result),
	}, nil
}

// Dispatch decodes the envelope through the real wire path and answers
// the scripted outcome for the operation it carries. A duplicate dispatch
// under a key the fake already executed replays the remembered outcome
// without executing again. A malformed or invalid envelope is an error:
// the dispatcher's requests must satisfy the typed-operation contract.
func (f *Fake) Dispatch(_ context.Context, envelope []byte) (Result, error) {
	request, err := operations.DecodeRequest(f.registry, envelope)
	if err != nil {
		return Result{}, fmt.Errorf("fake agent rejected request: %w", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	key := request.Envelope.IdempotencyKey
	if remembered, ok := f.outcomes[key]; ok {
		return Result{
			Outcome: remembered.Outcome,
			Detail:  fmt.Sprintf("%s (replayed by idempotency key)", remembered.Detail),
		}, nil
	}

	f.executions++
	result := f.script(request.Envelope.OperationType)
	f.outcomes[key] = result
	return result, nil
}

// ExecutionCount reports how many dispatches the fake actually
// executed; replays do not count.
func (f *Fake) ExecutionCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.executions
}

// script answers the outcome for one first-executed dispatch under the
// configured scenario.
func (f *Fake) script(operationType string) Result {
	switch f.scenario {
	case ScenarioJobFailure:
		return Result{
			Outcome: OutcomeFailed,
			Detail:  fmt.Sprintf("%s failed by the fake Atlas Agent (scenario %s)", operationType, f.scenario),
		}
	case ScenarioDispatchFailsOnce:
		if !f.failedOnce {
			f.failedOnce = true
			return Result{
				Outcome: OutcomeFailed,
				Detail:  fmt.Sprintf("%s failed once by the fake Atlas Agent (scenario %s)", operationType, f.scenario),
			}
		}
	}
	return Result{
		Outcome: OutcomeSucceeded,
		Detail:  fmt.Sprintf("%s executed by the fake Atlas Agent", operationType),
	}
}

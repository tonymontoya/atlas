package agent

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/tonymontoya/ceph-atlas/internal/operations"
)

func testRegistry(t *testing.T) *operations.Registry {
	t.Helper()
	registry, err := operations.DefaultRegistry()
	if err != nil {
		t.Fatalf("default registry: %v", err)
	}
	return registry
}

const collectEvidenceEnvelope = `{
	"workflowInstanceId": 7,
	"jobId": 11,
	"actor": {"subject": "write-operator", "displayName": "Write Operator"},
	"approval": {"approvalId": 3, "approver": "write-operator"},
	"idempotencyKey": "instance-7-job-11-attempt-1",
	"auditCorrelationId": "wf-7-job-11",
	"operationType": "CollectHostEvidence",
	"parameters": {"host": "storage-01", "requestType": "replace-osd"}
}`

func TestFakeDispatchSucceedsOnValidEnvelope(t *testing.T) {
	fake := NewFake(testRegistry(t))

	result, err := fake.Dispatch(context.Background(), []byte(collectEvidenceEnvelope))
	if err != nil {
		t.Fatalf("Dispatch error: %v", err)
	}
	if result.Outcome != OutcomeSucceeded {
		t.Fatalf("outcome = %q, want succeeded", result.Outcome)
	}
	if result.Detail == "" {
		t.Fatal("detail is empty")
	}
}

func TestFakeDispatchRejectsMalformedEnvelope(t *testing.T) {
	fake := NewFake(testRegistry(t))

	for name, envelope := range map[string]string{
		"not json":        `{`,
		"unknown op":      `{"workflowInstanceId":1,"jobId":1,"actor":{"subject":"a"},"idempotencyKey":"k","auditCorrelationId":"c","operationType":"Nonsense"}`,
		"invalid params":  `{"workflowInstanceId":1,"jobId":1,"actor":{"subject":"a"},"idempotencyKey":"k","auditCorrelationId":"c","operationType":"DestroyOSD","parameters":{"clusterFsid":"","osdId":3}}`,
		"missing job id":  `{"workflowInstanceId":1,"actor":{"subject":"a"},"idempotencyKey":"k","auditCorrelationId":"c","operationType":"CollectHostEvidence","parameters":{"host":"h","requestType":"r"}}`,
		"missing idemkey": `{"workflowInstanceId":1,"jobId":1,"actor":{"subject":"a"},"auditCorrelationId":"c","operationType":"CollectHostEvidence","parameters":{"host":"h","requestType":"r"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := fake.Dispatch(context.Background(), []byte(envelope)); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

// A duplicate dispatch under the same idempotency key must not repeat the
// work: the fake remembers the outcome per key and replays it.
func TestFakeDispatchReplaysRememberedOutcomePerIdempotencyKey(t *testing.T) {
	fake := NewFake(testRegistry(t))

	first, err := fake.Dispatch(context.Background(), []byte(collectEvidenceEnvelope))
	if err != nil {
		t.Fatalf("first dispatch: %v", err)
	}
	executions := fake.ExecutionCount()
	if executions != 1 {
		t.Fatalf("executions = %d after first dispatch, want 1", executions)
	}

	second, err := fake.Dispatch(context.Background(), []byte(collectEvidenceEnvelope))
	if err != nil {
		t.Fatalf("duplicate dispatch: %v", err)
	}
	if second.Outcome != first.Outcome {
		t.Fatalf("replayed outcome = %q, want %q", second.Outcome, first.Outcome)
	}
	if got := fake.ExecutionCount(); got != executions {
		t.Fatalf("executions = %d after duplicate dispatch, want %d (no repeat)", got, executions)
	}
}

func TestNewFakeWithScenarioValidatesScenarioName(t *testing.T) {
	registry := testRegistry(t)

	for _, scenario := range []string{"", "dispatch-fails-once", "job-failure"} {
		if _, err := NewFakeWithScenario(registry, scenario); err != nil {
			t.Fatalf("scenario %q: unexpected error %v", scenario, err)
		}
	}
	if _, err := NewFakeWithScenario(registry, "nonsense"); err == nil {
		t.Fatal("unknown scenario must be rejected")
	}
}

// dispatch-fails-once: the first executed dispatch fails; the retry (a
// new idempotency key) and every later dispatch succeed.
func TestFakeScenarioDispatchFailsOnce(t *testing.T) {
	fake, err := NewFakeWithScenario(testRegistry(t), "dispatch-fails-once")
	if err != nil {
		t.Fatalf("scenario agent: %v", err)
	}

	first, err := fake.Dispatch(context.Background(), []byte(collectEvidenceEnvelope))
	if err != nil {
		t.Fatalf("first dispatch: %v", err)
	}
	if first.Outcome != OutcomeFailed {
		t.Fatalf("first outcome = %q, want failed", first.Outcome)
	}

	retryEnvelope := strings.Replace(collectEvidenceEnvelope, "attempt-1", "attempt-2", 1)
	retry, err := fake.Dispatch(context.Background(), []byte(retryEnvelope))
	if err != nil {
		t.Fatalf("retry dispatch: %v", err)
	}
	if retry.Outcome != OutcomeSucceeded {
		t.Fatalf("retry outcome = %q, want succeeded", retry.Outcome)
	}

	// The failed outcome replays for the failed key too.
	replay, err := fake.Dispatch(context.Background(), []byte(collectEvidenceEnvelope))
	if err != nil {
		t.Fatalf("replay dispatch: %v", err)
	}
	if replay.Outcome != OutcomeFailed {
		t.Fatalf("replay outcome = %q, want the remembered failure", replay.Outcome)
	}
}

// job-failure: every first execution of a key fails, retries included,
// until the retry budget is spent.
func TestFakeScenarioJobFailureFailsEveryAttempt(t *testing.T) {
	fake, err := NewFakeWithScenario(testRegistry(t), "job-failure")
	if err != nil {
		t.Fatalf("scenario agent: %v", err)
	}

	for attempt := 1; attempt <= 3; attempt++ {
		envelope := strings.Replace(collectEvidenceEnvelope, "attempt-1", fmt.Sprintf("attempt-%d", attempt), 1)
		result, err := fake.Dispatch(context.Background(), []byte(envelope))
		if err != nil {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
		if result.Outcome != OutcomeFailed {
			t.Fatalf("attempt %d outcome = %q, want failed", attempt, result.Outcome)
		}
	}
}

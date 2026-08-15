package agent

import (
	"context"
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

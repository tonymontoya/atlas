package operations

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func validEnvelope(t *testing.T) RequestEnvelope {
	t.Helper()
	return RequestEnvelope{
		WorkflowInstanceID: 347,
		JobID:              2,
		Actor:              Actor{Subject: "operator-1", DisplayName: "Operator One"},
		IdempotencyKey:     "replace-osd-347-job-2",
		AuditCorrelationID: "audit-7f3k",
		OperationType:      "CollectHostEvidence",
		Parameters:         marshalParams(t, CollectHostEvidence{Host: "storage-03", RequestType: "smart"}),
	}
}

func decode(t *testing.T, envelope RequestEnvelope) (Request, error) {
	t.Helper()
	registry := defaultTestRegistry(t)
	encoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	return DecodeRequest(registry, encoded)
}

func TestDecodeRequestRejectsUnknownOperationType(t *testing.T) {
	envelope := validEnvelope(t)
	envelope.OperationType = "RunShellCommand"
	envelope.Parameters = json.RawMessage(`{"command":"rm -rf /"}`)

	_, err := decode(t, envelope)
	if !errors.Is(err, ErrUnknownOperation) {
		t.Fatalf("error = %v, want ErrUnknownOperation", err)
	}
}

func TestDecodeRequestRejectsMalformedJSON(t *testing.T) {
	registry := defaultTestRegistry(t)

	if _, err := DecodeRequest(registry, []byte(`{"workflowInstanceId":`)); err == nil {
		t.Fatal("accepted a truncated envelope")
	}
}

func TestDecodeRequestRejectsInvalidParameters(t *testing.T) {
	registry := defaultTestRegistry(t)
	base := validEnvelope(t)

	invalid := []struct {
		name       string
		parameters string
	}{
		{"wrong field types", `{"host":42,"requestType":"smart"}`},
		{"missing required fields", `{}`},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			envelope := base
			envelope.Parameters = json.RawMessage(test.parameters)
			encoded, err := json.Marshal(envelope)
			if err != nil {
				t.Fatalf("marshal envelope: %v", err)
			}
			if _, err := DecodeRequest(registry, encoded); err == nil {
				t.Fatalf("accepted %s", test.name)
			}
		})
	}

	t.Run("absent parameters", func(t *testing.T) {
		encoded := []byte(`{
			"workflowInstanceId": 347, "jobId": 2,
			"actor": {"subject": "operator-1", "displayName": "Operator One"},
			"idempotencyKey": "replace-osd-347-job-2", "auditCorrelationId": "audit-7f3k",
			"operationType": "CollectHostEvidence"
		}`)
		if _, err := DecodeRequest(registry, encoded); err == nil {
			t.Fatal("accepted absent parameters")
		}
	})
}

func TestDecodeRequestRejectsInvalidOSDOperationParameters(t *testing.T) {
	tests := []struct {
		name      string
		operation Operation
	}{
		{"DestroyOSD without clusterFsid", DestroyOSD{OSDID: 12}},
		{"DestroyOSD with negative osdId", DestroyOSD{ClusterFSID: "00000000-0000-4000-8000-000000000101", OSDID: -1}},
		{"VerifyOSD without clusterFsid", VerifyOSD{OSDID: 12}},
		{"VerifyOSD with negative osdId", VerifyOSD{ClusterFSID: "00000000-0000-4000-8000-000000000101", OSDID: -1}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			envelope := validEnvelope(t)
			envelope.OperationType = test.operation.OperationType()
			envelope.Parameters = marshalParams(t, test.operation)

			if _, err := decode(t, envelope); err == nil {
				t.Fatalf("accepted %s", test.name)
			}
		})
	}
}

func TestDecodeRequestRejectsInvalidEnvelopes(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*RequestEnvelope)
		wantError string
	}{
		{"missing workflow instance id", func(e *RequestEnvelope) { e.WorkflowInstanceID = 0 }, "workflowInstanceId is required"},
		{"missing job id", func(e *RequestEnvelope) { e.JobID = 0 }, "jobId is required"},
		{"missing actor subject", func(e *RequestEnvelope) { e.Actor.Subject = "" }, "actor subject is required"},
		{"missing idempotency key", func(e *RequestEnvelope) { e.IdempotencyKey = "" }, "idempotencyKey is required"},
		{"missing audit correlation id", func(e *RequestEnvelope) { e.AuditCorrelationID = "" }, "auditCorrelationId is required"},
		{"approval without id", func(e *RequestEnvelope) { e.Approval = &ApprovalContext{Approver: "approver-1"} }, "approvalId is required"},
		{"approval without approver", func(e *RequestEnvelope) { e.Approval = &ApprovalContext{ApprovalID: 9} }, "approver is required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			envelope := validEnvelope(t)
			test.mutate(&envelope)

			_, err := decode(t, envelope)
			if err == nil {
				t.Fatalf("accepted %s", test.name)
			}
			if !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("error = %q, want it to mention %q", err, test.wantError)
			}
		})
	}
}

func TestRegistryRejectsPointerPrototypes(t *testing.T) {
	if _, err := NewRegistry(&CollectHostEvidence{}); err == nil {
		t.Fatal("accepted a pointer prototype")
	}
}

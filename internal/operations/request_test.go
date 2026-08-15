package operations

import (
	"encoding/json"
	"testing"
)

func TestDecodeRequestRoundTripsCollectHostEvidence(t *testing.T) {
	registry := defaultTestRegistry(t)
	envelope := RequestEnvelope{
		WorkflowInstanceID: 347,
		JobID:              2,
		Actor:              Actor{Subject: "operator-1", DisplayName: "Operator One"},
		Approval:           &ApprovalContext{ApprovalID: 9, Approver: "approver-1"},
		IdempotencyKey:     "replace-osd-347-job-2",
		AuditCorrelationID: "audit-7f3k",
		OperationType:      "CollectHostEvidence",
		Parameters:         marshalParams(t, CollectHostEvidence{Host: "storage-03", RequestType: "smart"}),
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}

	request, err := DecodeRequest(registry, encoded)
	if err != nil {
		t.Fatalf("DecodeRequest: %v", err)
	}

	if request.Envelope.WorkflowInstanceID != 347 || request.Envelope.JobID != 2 {
		t.Fatalf("ids = %d/%d, want 347/2", request.Envelope.WorkflowInstanceID, request.Envelope.JobID)
	}
	if request.Envelope.Actor != (Actor{Subject: "operator-1", DisplayName: "Operator One"}) {
		t.Fatalf("actor = %+v", request.Envelope.Actor)
	}
	if request.Envelope.Approval == nil || request.Envelope.Approval.ApprovalID != 9 || request.Envelope.Approval.Approver != "approver-1" {
		t.Fatalf("approval = %+v", request.Envelope.Approval)
	}
	if request.Envelope.IdempotencyKey != "replace-osd-347-job-2" {
		t.Fatalf("idempotencyKey = %q", request.Envelope.IdempotencyKey)
	}
	if request.Envelope.AuditCorrelationID != "audit-7f3k" {
		t.Fatalf("auditCorrelationId = %q", request.Envelope.AuditCorrelationID)
	}
	collect, ok := request.Operation.(*CollectHostEvidence)
	if !ok {
		t.Fatalf("operation = %T, want *CollectHostEvidence", request.Operation)
	}
	if collect.Host != "storage-03" || collect.RequestType != "smart" {
		t.Fatalf("operation = %+v, want host storage-03 and requestType smart", collect)
	}
}

func TestDecodeRequestRoundTripsEnvelopeWithoutApproval(t *testing.T) {
	registry := defaultTestRegistry(t)
	envelope := RequestEnvelope{
		WorkflowInstanceID: 1,
		JobID:              1,
		Actor:              Actor{Subject: "operator-1"},
		IdempotencyKey:     "key-1",
		AuditCorrelationID: "audit-1",
		OperationType:      "CollectHostEvidence",
		Parameters:         marshalParams(t, CollectHostEvidence{Host: "storage-01", RequestType: "smart"}),
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}

	request, err := DecodeRequest(registry, encoded)
	if err != nil {
		t.Fatalf("DecodeRequest: %v", err)
	}
	if request.Envelope.Approval != nil {
		t.Fatalf("approval = %+v, want nil", request.Envelope.Approval)
	}
}

func marshalParams(t *testing.T, operation Operation) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(operation)
	if err != nil {
		t.Fatalf("marshal parameters: %v", err)
	}
	return json.RawMessage(encoded)
}

package operations

import (
	"encoding/json"
	"testing"
)

func TestDecodeRequestRoundTripsOSDOperations(t *testing.T) {
	registry := defaultTestRegistry(t)

	tests := []struct {
		name         string
		operation    Operation
		approval     *ApprovalContext
		wantApproval bool
	}{
		{
			name:         "DestroyOSD with approval",
			operation:    DestroyOSD{ClusterFSID: "00000000-0000-4000-8000-000000000101", OSDID: 12},
			approval:     &ApprovalContext{ApprovalID: 9, Approver: "approver-1"},
			wantApproval: true,
		},
		{
			name:      "VerifyOSD without approval",
			operation: VerifyOSD{ClusterFSID: "00000000-0000-4000-8000-000000000101", OSDID: 0},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			envelope := RequestEnvelope{
				WorkflowInstanceID: 347,
				JobID:              5,
				Actor:              Actor{Subject: "operator-1", DisplayName: "Operator One"},
				Approval:           test.approval,
				IdempotencyKey:     "replace-osd-347-job-5",
				AuditCorrelationID: "audit-7f3k",
				OperationType:      test.operation.OperationType(),
				Parameters:         marshalParams(t, test.operation),
			}
			encoded, err := json.Marshal(envelope)
			if err != nil {
				t.Fatalf("marshal envelope: %v", err)
			}

			request, err := DecodeRequest(registry, encoded)
			if err != nil {
				t.Fatalf("DecodeRequest: %v", err)
			}

			if request.Operation.OperationType() != test.operation.OperationType() {
				t.Fatalf("operation type = %q, want %q", request.Operation.OperationType(), test.operation.OperationType())
			}
			encodedAgain, err := json.Marshal(request.Operation)
			if err != nil {
				t.Fatalf("marshal decoded operation: %v", err)
			}
			if string(encodedAgain) != string(envelope.Parameters) {
				t.Fatalf("re-encoded operation = %s, want %s", encodedAgain, envelope.Parameters)
			}
			if test.wantApproval && request.Envelope.Approval == nil {
				t.Fatal("approval context lost in round trip")
			}
			if !test.wantApproval && request.Envelope.Approval != nil {
				t.Fatalf("approval = %+v, want nil", request.Envelope.Approval)
			}
		})
	}
}

func TestDefaultRegistryResolvesReplaceOSDOperations(t *testing.T) {
	registry, err := DefaultRegistry()
	if err != nil {
		t.Fatalf("DefaultRegistry: %v", err)
	}
	for _, operationType := range []string{"CollectHostEvidence", "DestroyOSD", "VerifyOSD"} {
		if _, ok := registry.Get(operationType); !ok {
			t.Fatalf("operation %q not in default registry", operationType)
		}
	}
}

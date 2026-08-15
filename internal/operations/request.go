package operations

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
)

// Actor identifies the verified operator a request is executed for.
type Actor struct {
	Subject     string `json:"subject"`
	DisplayName string `json:"displayName"`
}

// ApprovalContext carries the Approval evidence a request executes under
// (ADR-0020). It is optional until policy requires it for an operation.
type ApprovalContext struct {
	ApprovalID int64  `json:"approvalId"`
	Approver   string `json:"approver"`
}

// RequestEnvelope is the shared wire shape for agent requests (ADR-0018):
// the execution context every operation travels with, plus the operation
// type and its raw typed parameters.
type RequestEnvelope struct {
	WorkflowInstanceID int64            `json:"workflowInstanceId"`
	JobID              int64            `json:"jobId"`
	Actor              Actor            `json:"actor"`
	Approval           *ApprovalContext `json:"approval,omitempty"`
	IdempotencyKey     string           `json:"idempotencyKey"`
	AuditCorrelationID string           `json:"auditCorrelationId"`
	OperationType      string           `json:"operationType"`
	Parameters         json.RawMessage  `json:"parameters"`
}

// Request is a decoded agent request: the envelope context plus the
// typed, validated operation.
type Request struct {
	Envelope  RequestEnvelope
	Operation Operation
}

// DecodeRequest unmarshals an agent request envelope, resolves its
// operation type through the registry, unmarshals the parameters into the
// typed operation struct, and validates both the envelope and the
// operation (ADR-0018). Unknown operation types and malformed inputs are
// rejected.
func DecodeRequest(registry *Registry, data []byte) (Request, error) {
	var envelope RequestEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return Request{}, fmt.Errorf("decode request envelope: %w", err)
	}
	if err := envelope.validate(); err != nil {
		return Request{}, err
	}
	prototype, ok := registry.Get(envelope.OperationType)
	if !ok {
		return Request{}, fmt.Errorf("%w: %q", ErrUnknownOperation, envelope.OperationType)
	}
	parameters := envelope.Parameters
	if len(parameters) == 0 {
		parameters = json.RawMessage("{}")
	}
	operation := newOperation(prototype)
	if err := json.Unmarshal(parameters, operation); err != nil {
		return Request{}, fmt.Errorf("decode %s parameters: %w", envelope.OperationType, err)
	}
	if err := operation.Validate(); err != nil {
		return Request{}, fmt.Errorf("invalid %s parameters: %w", envelope.OperationType, err)
	}
	return Request{Envelope: envelope, Operation: operation}, nil
}

// ErrUnknownOperation reports an operation type the registry does not know.
var ErrUnknownOperation = errors.New("unknown operation type")

// newOperation returns a fresh, decodable instance of the prototype's
// type. Prototypes are registered as values, so the instance is a pointer
// to a zero value of the same type.
func newOperation(prototype Operation) Operation {
	return reflect.New(reflect.TypeOf(prototype)).Interface().(Operation)
}

func (e RequestEnvelope) validate() error {
	if e.WorkflowInstanceID <= 0 {
		return errors.New("workflowInstanceId is required")
	}
	if e.JobID <= 0 {
		return errors.New("jobId is required")
	}
	if e.Actor.Subject == "" {
		return errors.New("actor subject is required")
	}
	if e.IdempotencyKey == "" {
		return errors.New("idempotencyKey is required")
	}
	if e.AuditCorrelationID == "" {
		return errors.New("auditCorrelationId is required")
	}
	if e.Approval != nil {
		if e.Approval.ApprovalID <= 0 {
			return errors.New("approvalId is required when approval is present")
		}
		if e.Approval.Approver == "" {
			return errors.New("approver is required when approval is present")
		}
	}
	return nil
}

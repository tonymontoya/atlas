package workflowdispatch

import (
	"context"

	"github.com/tonymontoya/ceph-atlas/internal/providers"
	"github.com/tonymontoya/ceph-atlas/internal/store"
	"github.com/tonymontoya/ceph-atlas/internal/workflows"
)

// LifecycleStore is the persistence seam the Workflow Instance
// choreography drives. Every method is already implemented by the
// PostgreSQL store.
type LifecycleStore interface {
	CreateWorkflowInstance(ctx context.Context, input store.CreateWorkflowInstanceInput) (store.WorkflowInstance, error)
	GetWorkflowInstance(ctx context.Context, instanceID int64) (store.WorkflowInstance, error)
	TransitionWorkflowInstance(ctx context.Context, input store.WorkflowInstanceTransitionInput) (store.WorkflowInstance, error)
	RecordApproval(ctx context.Context, input store.RecordApprovalInput) (store.ApprovalRecord, error)
	RecordTaskCompletion(ctx context.Context, input store.RecordTaskCompletionInput) (store.TaskCompletionRecord, error)
}

// Lifecycle owns the Workflow Instance choreography around dispatch:
// attach parks a fresh instance at its definition's first Approval
// Gate, approvals and task completions resume paused instances and hand
// them to the Dispatcher. A nil Dispatcher means nothing dispatches —
// instances rest running with pending Jobs (ADR-0022 disabled mode).
type Lifecycle struct {
	store    LifecycleStore
	defs     workflows.Registry
	dispatch *Dispatcher
}

// NewLifecycle builds a Lifecycle over the durable store, the Workflow
// definition registry, and an optional Dispatcher. A nil Dispatcher
// disables Job execution: resumed instances rest running.
func NewLifecycle(store LifecycleStore, defs workflows.Registry, dispatch *Dispatcher) *Lifecycle {
	return &Lifecycle{store: store, defs: defs, dispatch: dispatch}
}

// RecordResult reports a durable record write and whether this call
// advanced the instance past the step it was paused at. A replay of an
// already-recorded write returns the existing record with Advanced
// false and touches nothing.
type RecordResult[T any] struct {
	Record   T
	Advanced bool
}

// Attach attaches a Workflow to a Case: the definition is resolved in
// the code registry (ADR-0017) before any store call, and only Job
// steps become Job rows — Gates and Tasks are not Jobs (ADR-0019). The
// instance then advances from pending through running to a pause at the
// definition's first Approval Gate, with the Atlas system actor
// attributing the advancement Timeline Events; without a gate the
// instance rests pending.
func (l *Lifecycle) Attach(ctx context.Context, actor store.Actor, caseID int64, workflowID string, workflowVersion int) (store.WorkflowInstance, error) {
	definition, ok := l.defs.Get(workflowID, workflowVersion)
	if !ok {
		return store.WorkflowInstance{}, providers.ProviderError{
			Class:   providers.ErrorNotFound,
			Message: "workflow definition not found",
		}
	}

	jobs := make([]store.WorkflowJobInput, 0, len(definition.Steps))
	for _, step := range definition.Steps {
		if jobStep, ok := step.(workflows.JobStep); ok {
			jobs = append(jobs, store.WorkflowJobInput{
				StepID:        jobStep.ID,
				OperationType: jobStep.OperationType,
				MaxAttempts:   jobStep.Retry.MaxAttempts,
			})
		}
	}

	instance, err := l.store.CreateWorkflowInstance(ctx, store.CreateWorkflowInstanceInput{
		CaseID:            caseID,
		DefinitionID:      definition.ID,
		DefinitionVersion: definition.Version,
		Jobs:              jobs,
		Actor:             actor,
	})
	if err != nil {
		return store.WorkflowInstance{}, err
	}

	return l.advanceToFirstGate(ctx, instance)
}

// advanceToFirstGate moves a freshly attached instance from pending
// through running to waiting_for_approval at the definition's first
// Approval Gate. Gateless definitions leave the instance where it is.
func (l *Lifecycle) advanceToFirstGate(ctx context.Context, instance store.WorkflowInstance) (store.WorkflowInstance, error) {
	var gateID string
	definition, ok := l.defs.Get(instance.DefinitionID, instance.DefinitionVersion)
	if ok {
		for _, step := range definition.Steps {
			if gate, isGate := step.(workflows.ApprovalGate); isGate {
				gateID = gate.ID
				break
			}
		}
	}
	if gateID == "" || instance.State != workflows.InstancePending {
		return instance, nil
	}

	if _, err := l.store.TransitionWorkflowInstance(ctx, store.WorkflowInstanceTransitionInput{
		InstanceID: instance.ID,
		To:         workflows.InstanceRunning,
	}); err != nil {
		return store.WorkflowInstance{}, err
	}
	return l.store.TransitionWorkflowInstance(ctx, store.WorkflowInstanceTransitionInput{
		InstanceID: instance.ID,
		To:         workflows.InstanceWaitingForApproval,
		AtStep:     gateID,
	})
}

// ApproveGate records the durable, immutable Approval for the gate the
// instance is paused at and advances the instance past it (ADR-0020,
// ADR-0021). A second approval of an already-passed gate is an
// idempotent no-op returning the existing record without touching the
// instance.
func (l *Lifecycle) ApproveGate(ctx context.Context, actor store.Actor, instanceID int64, gateID, reason string) (RecordResult[store.ApprovalRecord], error) {
	approval, err := l.store.RecordApproval(ctx, store.RecordApprovalInput{
		InstanceID: instanceID,
		GateID:     gateID,
		Approver:   actor,
		Reason:     reason,
	})
	if err != nil {
		return RecordResult[store.ApprovalRecord]{}, err
	}

	// The record already existing means the gate was passed before; the
	// approval authorizes nothing further (ADR-0020). Otherwise this call
	// recorded the first approval, and the instance is still paused at the
	// gate: advance it.
	advanced, err := l.resumeIfPausedAt(ctx, actor, instanceID, workflows.InstanceWaitingForApproval, gateID)
	if err != nil {
		return RecordResult[store.ApprovalRecord]{}, err
	}
	return RecordResult[store.ApprovalRecord]{Record: approval, Advanced: advanced}, nil
}

// CompleteTask records the durable, immutable Task completion for the
// human Task the instance is paused at and resumes the instance past it
// (ADR-0019). A second completion of an already-passed task is an
// idempotent no-op returning the existing record without touching the
// instance.
func (l *Lifecycle) CompleteTask(ctx context.Context, actor store.Actor, instanceID int64, taskID, note string) (RecordResult[store.TaskCompletionRecord], error) {
	completion, err := l.store.RecordTaskCompletion(ctx, store.RecordTaskCompletionInput{
		InstanceID: instanceID,
		TaskID:     taskID,
		Operator:   actor,
		Note:       note,
	})
	if err != nil {
		return RecordResult[store.TaskCompletionRecord]{}, err
	}

	// The record already existing means the task was passed before; the
	// completion authorizes nothing further. Otherwise this call
	// recorded the first completion, and the instance is still paused at
	// the task: resume it.
	advanced, err := l.resumeIfPausedAt(ctx, actor, instanceID, workflows.InstanceWaitingForOperator, taskID)
	if err != nil {
		return RecordResult[store.TaskCompletionRecord]{}, err
	}
	return RecordResult[store.TaskCompletionRecord]{Record: completion, Advanced: advanced}, nil
}

// resumeIfPausedAt advances the instance past the step it is paused at
// and hands it to the Dispatcher: with the step passed, the agent loop
// (ADR-0022) drives the instance through its Jobs; without a dispatcher
// the instance rests running with pending Jobs. An instance no longer
// paused at that step — the record was a replay of a passed step —
// touches nothing and reports not advanced.
func (l *Lifecycle) resumeIfPausedAt(ctx context.Context, actor store.Actor, instanceID int64, state workflows.InstanceState, stepID string) (bool, error) {
	instance, err := l.store.GetWorkflowInstance(ctx, instanceID)
	if err != nil {
		return false, err
	}
	if instance.State != state || instance.CurrentStep == nil || *instance.CurrentStep != stepID {
		return false, nil
	}
	if _, err := l.store.TransitionWorkflowInstance(ctx, store.WorkflowInstanceTransitionInput{
		InstanceID: instanceID,
		To:         workflows.InstanceRunning,
		Actor:      &actor,
	}); err != nil {
		return false, err
	}
	if l.dispatch != nil {
		if _, err := l.dispatch.Run(ctx, instanceID); err != nil {
			return false, err
		}
	}
	return true, nil
}

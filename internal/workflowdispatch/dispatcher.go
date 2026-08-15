// Package workflowdispatch drives a running Workflow Instance through
// its definition's steps by dispatching Jobs to an Atlas Agent adapter
// (ADR-0022). The dispatcher owns no state of its own: every advance is
// a durable store transition, so a stopped process leaves the instance
// resumable from PostgreSQL (ADR-0019).
package workflowdispatch

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/tonymontoya/ceph-atlas/internal/agent"
	"github.com/tonymontoya/ceph-atlas/internal/cases"
	"github.com/tonymontoya/ceph-atlas/internal/operations"
	"github.com/tonymontoya/ceph-atlas/internal/store"
	"github.com/tonymontoya/ceph-atlas/internal/workflows"
)

// Store is the persistence seam the dispatcher drives. Every method is
// already implemented by the PostgreSQL store.
type Store interface {
	GetWorkflowInstance(ctx context.Context, instanceID int64) (store.WorkflowInstance, error)
	TransitionWorkflowInstance(ctx context.Context, input store.WorkflowInstanceTransitionInput) (store.WorkflowInstance, error)
	TransitionWorkflowJob(ctx context.Context, input store.WorkflowJobTransitionInput) (store.WorkflowJob, error)
	ListWorkflowJobs(ctx context.Context, instanceID int64) ([]store.WorkflowJob, error)
	ListWorkflowApprovals(ctx context.Context, instanceID int64) ([]store.ApprovalRecord, error)
	ListWorkflowTaskCompletions(ctx context.Context, instanceID int64) ([]store.TaskCompletionRecord, error)
	GetCase(ctx context.Context, caseID int64) (cases.Case, error)
}

// Dispatcher advances one running Workflow Instance at a time through
// its definition: pending Jobs are dispatched to the agent adapter and
// failed Jobs are retried per the definition's policy until they succeed
// or their budget is spent, human Tasks pause the instance at
// waiting_for_operator until an Operator records their completion, and
// once every Job has succeeded the instance reaches terminal succeeded
// (ADR-0019). The dispatcher owns no state of its own: every advance is
// a durable store transition, so a stopped process leaves the instance
// resumable from PostgreSQL — Run re-dispatches a Job left in flight
// under its original idempotency key and re-queues a failed Job whose
// budget has not been spent.
type Dispatcher struct {
	store   Store
	defs    workflows.Registry
	adapter agent.AgentAdapter
}

// New builds a Dispatcher over the durable store, the Workflow
// definition registry, and an agent adapter.
func New(store Store, defs workflows.Registry, adapter agent.AgentAdapter) *Dispatcher {
	return &Dispatcher{store: store, defs: defs, adapter: adapter}
}

// Run drives the instance until it pauses or reaches a terminal state
// and returns its latest durable row. Instances that are not running —
// parked at a gate or task, or terminal — are returned untouched:
// dispatch only ever continues a running instance.
func (d *Dispatcher) Run(ctx context.Context, instanceID int64) (store.WorkflowInstance, error) {
	instance, err := d.store.GetWorkflowInstance(ctx, instanceID)
	if err != nil {
		return store.WorkflowInstance{}, err
	}
	if instance.State != workflows.InstanceRunning {
		return instance, nil
	}
	definition, ok := d.defs.Get(instance.DefinitionID, instance.DefinitionVersion)
	if !ok {
		return store.WorkflowInstance{}, fmt.Errorf("workflow %s v%d is not in the registry", instance.DefinitionID, instance.DefinitionVersion)
	}
	jobs, err := d.store.ListWorkflowJobs(ctx, instanceID)
	if err != nil {
		return store.WorkflowInstance{}, err
	}
	jobsByStep := make(map[string]store.WorkflowJob, len(jobs))
	for _, job := range jobs {
		jobsByStep[job.StepID] = job
	}
	approvals, err := d.store.ListWorkflowApprovals(ctx, instanceID)
	if err != nil {
		return store.WorkflowInstance{}, err
	}
	approvedGates := make(map[string]bool, len(approvals))
	for _, approval := range approvals {
		approvedGates[approval.GateID] = true
	}
	completions, err := d.store.ListWorkflowTaskCompletions(ctx, instanceID)
	if err != nil {
		return store.WorkflowInstance{}, err
	}
	completedTasks := make(map[string]bool, len(completions))
	for _, completion := range completions {
		completedTasks[completion.TaskID] = true
	}
	target, err := d.store.GetCase(ctx, instance.CaseID)
	if err != nil {
		return store.WorkflowInstance{}, err
	}

	for _, step := range definition.Steps {
		switch step := step.(type) {
		case workflows.ApprovalGate:
			// Gates pause the instance before dispatch ever runs; a gate
			// the walk reaches without an approval record stops the run
			// instead of executing past an unauthorized mutation.
			if !approvedGates[step.ID] {
				return instance, nil
			}
		case workflows.TaskStep:
			// Human Tasks pause the instance until an Operator records
			// the completion that resumes it (ADR-0019).
			if !completedTasks[step.ID] {
				return d.store.TransitionWorkflowInstance(ctx, store.WorkflowInstanceTransitionInput{
					InstanceID: instanceID,
					To:         workflows.InstanceWaitingForOperator,
					AtStep:     step.ID,
				})
			}
		case workflows.JobStep:
			job, ok := jobsByStep[step.ID]
			if !ok {
				return store.WorkflowInstance{}, fmt.Errorf("workflow instance %d has no job row for step %q", instanceID, step.ID)
			}
			if job.State == workflows.JobSucceeded {
				continue
			}
			outcome, err := d.dispatchJob(ctx, definition, target, instance, job, approvals)
			if err != nil {
				return store.WorkflowInstance{}, err
			}
			if outcome == workflows.JobFailed {
				return d.store.TransitionWorkflowInstance(ctx, store.WorkflowInstanceTransitionInput{
					InstanceID: instanceID,
					To:         workflows.InstanceFailed,
				})
			}
		}
	}
	return d.store.TransitionWorkflowInstance(ctx, store.WorkflowInstanceTransitionInput{
		InstanceID: instanceID,
		To:         workflows.InstanceSucceeded,
	})
}

// dispatchJob drives one Job to a terminal outcome, applying the
// definition's retry policy (ADR-0019). A pending Job is dispatched and
// the adapter's answer is recorded; a dispatched Job — one a dead
// process left in flight — is re-dispatched under its original
// idempotency key, which the agent deduplicates; a failed Job whose
// budget is not spent is re-queued onto a fresh attempt. An adapter that
// rejects the envelope counts as a failed attempt: the request did not
// satisfy the typed-operation contract.
func (d *Dispatcher) dispatchJob(ctx context.Context, definition workflows.Definition, target cases.Case, instance store.WorkflowInstance, job store.WorkflowJob, approvals []store.ApprovalRecord) (workflows.JobState, error) {
	for {
		switch job.State {
		case workflows.JobFailed:
			if job.Attempt >= job.MaxAttempts {
				return workflows.JobFailed, nil
			}
			requeued, err := d.store.TransitionWorkflowJob(ctx, store.WorkflowJobTransitionInput{JobID: job.ID, To: workflows.JobPending})
			if err != nil {
				return "", err
			}
			job = requeued
			continue
		case workflows.JobPending:
			dispatched, err := d.store.TransitionWorkflowJob(ctx, store.WorkflowJobTransitionInput{JobID: job.ID, To: workflows.JobDispatched})
			if err != nil {
				return "", err
			}
			job = dispatched
		}

		envelope, err := requestEnvelope(definition, target, instance, job, approvals)
		if err != nil {
			return "", err
		}
		payload, err := json.Marshal(envelope)
		if err != nil {
			return "", err
		}
		result, err := d.adapter.Dispatch(ctx, payload)
		if err != nil || result.Outcome == agent.OutcomeFailed {
			failed, transitionErr := d.store.TransitionWorkflowJob(ctx, store.WorkflowJobTransitionInput{JobID: job.ID, To: workflows.JobFailed})
			if transitionErr != nil {
				return "", transitionErr
			}
			job = failed
			continue
		}
		succeeded, err := d.store.TransitionWorkflowJob(ctx, store.WorkflowJobTransitionInput{JobID: job.ID, To: workflows.JobSucceeded})
		if err != nil {
			return "", err
		}
		return succeeded.State, nil
	}
}

// requestEnvelope builds the typed-operation request envelope for one
// Job (ADR-0018). Execution is attributed to the operator whose gate
// Approval authorized the run; without one the Atlas system actor
// executes. The idempotency key is deterministic per Job attempt, so a
// re-dispatch after a restart carries the same key and a destructive
// step never executes twice for it.
func requestEnvelope(definition workflows.Definition, target cases.Case, instance store.WorkflowInstance, job store.WorkflowJob, approvals []store.ApprovalRecord) (operations.RequestEnvelope, error) {
	parameters, err := fakeJobParameters(definition, target, job)
	if err != nil {
		return operations.RequestEnvelope{}, err
	}
	actor := operations.Actor{Subject: "atlas", DisplayName: "Atlas"}
	var approval *operations.ApprovalContext
	// The latest Approval record authorizes this run.
	if len(approvals) > 0 {
		latest := approvals[len(approvals)-1]
		actor = operations.Actor{Subject: latest.Approver.Subject, DisplayName: latest.Approver.DisplayName}
		approval = &operations.ApprovalContext{ApprovalID: latest.ID, Approver: latest.Approver.Subject}
	}
	return operations.RequestEnvelope{
		WorkflowInstanceID: instance.ID,
		JobID:              job.ID,
		Actor:              actor,
		Approval:           approval,
		IdempotencyKey:     fmt.Sprintf("instance-%d-job-%d-attempt-%d", instance.ID, job.ID, job.Attempt),
		AuditCorrelationID: fmt.Sprintf("workflow-%d-job-%d-attempt-%d", instance.ID, job.ID, job.Attempt),
		OperationType:      job.OperationType,
		Parameters:         parameters,
	}, nil
}

// fakeJobParameters synthesizes the typed parameters for one Job. The
// fake scaffold has no parameter binding yet — definitions carry only
// operation types and attaches carry no inputs — so deterministic fake
// values stand in, seeded with the Case's cluster where the operation
// needs one. Real parameter binding replaces this seam.
func fakeJobParameters(definition workflows.Definition, target cases.Case, job store.WorkflowJob) (json.RawMessage, error) {
	switch job.OperationType {
	case operations.CollectHostEvidence{}.OperationType():
		return json.Marshal(operations.CollectHostEvidence{Host: "fake-storage-01", RequestType: definition.ID})
	case operations.DestroyOSD{}.OperationType():
		return json.Marshal(operations.DestroyOSD{ClusterFSID: target.ClusterFSID, OSDID: 0})
	case operations.VerifyOSD{}.OperationType():
		return json.Marshal(operations.VerifyOSD{ClusterFSID: target.ClusterFSID, OSDID: 0})
	default:
		return nil, fmt.Errorf("no fake parameters for operation type %q", job.OperationType)
	}
}

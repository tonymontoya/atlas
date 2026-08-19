package workflowdispatch

import (
	"context"
	"testing"

	"github.com/tonymontoya/ceph-atlas/internal/apperr"
	"github.com/tonymontoya/ceph-atlas/internal/operations"
	"github.com/tonymontoya/ceph-atlas/internal/store"
	"github.com/tonymontoya/ceph-atlas/internal/workflows"
)

// The Lifecycle choreography drives the same in-memory store the
// dispatcher tests use, extended here with the record-creating methods
// whose guarded semantics (wrong-pause conflict, idempotent replay)
// mirror the PostgreSQL store; the store-backed integration tests prove
// the real contract.
var _ LifecycleStore = (*memStore)(nil)

func (m *memStore) CreateWorkflowInstance(_ context.Context, input store.CreateWorkflowInstanceInput) (store.WorkflowInstance, error) {
	inputCopy := input
	m.createInput = &inputCopy
	m.instance = store.WorkflowInstance{
		ID: 42, CaseID: input.CaseID,
		DefinitionID: input.DefinitionID, DefinitionVersion: input.DefinitionVersion,
		State: workflows.InstancePending,
	}
	for i, job := range input.Jobs {
		m.jobs = append(m.jobs, store.WorkflowJob{
			ID: int64(101 + i), WorkflowInstanceID: m.instance.ID, Position: i + 1,
			StepID: job.StepID, OperationType: job.OperationType,
			State: workflows.JobPending, Attempt: 1, MaxAttempts: job.MaxAttempts,
		})
	}
	return m.instance, nil
}

func (m *memStore) RecordApproval(_ context.Context, input store.RecordApprovalInput) (store.ApprovalRecord, error) {
	for _, approval := range m.approvals {
		if approval.WorkflowInstanceID == input.InstanceID && approval.GateID == input.GateID {
			return approval, nil
		}
	}
	if m.instance.State != workflows.InstanceWaitingForApproval || m.instance.CurrentStep == nil || *m.instance.CurrentStep != input.GateID {
		return store.ApprovalRecord{}, apperr.Error{
			Class:   apperr.Conflict,
			Message: "workflow instance is not waiting for approval at the gate",
		}
	}
	record := store.ApprovalRecord{
		ID: int64(10 + len(m.approvals)), WorkflowInstanceID: input.InstanceID,
		GateID: input.GateID, Approver: input.Approver,
	}
	if input.Reason != "" {
		reason := input.Reason
		record.Reason = &reason
	}
	m.approvals = append(m.approvals, record)
	return record, nil
}

func (m *memStore) RecordTaskCompletion(_ context.Context, input store.RecordTaskCompletionInput) (store.TaskCompletionRecord, error) {
	for _, completion := range m.taskCompletions {
		if completion.WorkflowInstanceID == input.InstanceID && completion.TaskID == input.TaskID {
			return completion, nil
		}
	}
	if m.instance.State != workflows.InstanceWaitingForOperator || m.instance.CurrentStep == nil || *m.instance.CurrentStep != input.TaskID {
		return store.TaskCompletionRecord{}, apperr.Error{
			Class:   apperr.Conflict,
			Message: "workflow instance is not waiting for the operator at the task",
		}
	}
	record := store.TaskCompletionRecord{
		ID: int64(20 + len(m.taskCompletions)), WorkflowInstanceID: input.InstanceID,
		TaskID: input.TaskID, Operator: input.Operator,
	}
	if input.Note != "" {
		note := input.Note
		record.Note = &note
	}
	m.taskCompletions = append(m.taskCompletions, record)
	return record, nil
}

// gatelessRegistry holds a definition with no Approval Gate, for the
// rest-pending attach path.
func gatelessRegistry(t *testing.T) *workflows.CodeRegistry {
	t.Helper()
	ops, err := operations.DefaultRegistry()
	if err != nil {
		t.Fatalf("operations registry: %v", err)
	}
	registry, err := workflows.NewCodeRegistry(ops, workflows.Definition{
		ID: "gateless", Version: 1,
		Steps: []workflows.Step{
			workflows.JobStep{ID: "collect-evidence", OperationType: "CollectHostEvidence", Retry: workflows.RetryPolicy{MaxAttempts: 3}},
		},
	})
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	return registry
}

const attachCaseID = 7

func operatorActor() store.Actor {
	return store.Actor{Subject: "op-1", DisplayName: "Operator One"}
}

// parkedReplaceOSD runs Attach against a fresh memStore and returns the
// store alongside the parked instance.
func parkedReplaceOSD(t *testing.T) (*memStore, store.WorkflowInstance) {
	t.Helper()
	mem := &memStore{}
	lifecycle := NewLifecycle(mem, replaceOSDRegistry(t), nil)
	instance, err := lifecycle.Attach(context.Background(), operatorActor(), attachCaseID, "replace-osd", 1)
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	return mem, instance
}

func TestAttachParksInstanceAtFirstGate(t *testing.T) {
	mem, instance := parkedReplaceOSD(t)

	if instance.State != workflows.InstanceWaitingForApproval || instance.CurrentStep == nil || *instance.CurrentStep != "approve-destroy" {
		t.Fatalf("instance = %+v, want waiting_for_approval at approve-destroy", instance)
	}
	// Only Job steps become Job rows — the gate and the task do not.
	wantSteps := []string{"collect-evidence", "destroy-osd", "verify-osd"}
	if len(mem.jobs) != len(wantSteps) {
		t.Fatalf("jobs = %+v, want %d job rows only", mem.jobs, len(wantSteps))
	}
	for i, job := range mem.jobs {
		if job.StepID != wantSteps[i] || job.State != workflows.JobPending {
			t.Fatalf("job %d = %+v, want pending %s", i, job, wantSteps[i])
		}
	}
	if mem.createInput == nil || mem.createInput.CaseID != attachCaseID || mem.createInput.Actor.Subject != "op-1" {
		t.Fatalf("create input = %+v, want the case id and attaching actor", mem.createInput)
	}
	wantTransitions := []workflows.InstanceState{workflows.InstanceRunning, workflows.InstanceWaitingForApproval}
	if len(mem.instanceCalls) != 2 {
		t.Fatalf("instance calls = %+v, want the two advancement transitions", mem.instanceCalls)
	}
	for i, want := range wantTransitions {
		if mem.instanceCalls[i].To != want {
			t.Fatalf("instance call %d = %+v, want %s", i, mem.instanceCalls[i], want)
		}
	}
	if mem.instanceCalls[1].AtStep != "approve-destroy" {
		t.Fatalf("pause transition = %+v, want paused at the gate", mem.instanceCalls[1])
	}
}

func TestAttachGatelessDefinitionRestsPending(t *testing.T) {
	mem := &memStore{}
	lifecycle := NewLifecycle(mem, gatelessRegistry(t), nil)

	instance, err := lifecycle.Attach(context.Background(), operatorActor(), attachCaseID, "gateless", 1)
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if instance.State != workflows.InstancePending || instance.CurrentStep != nil {
		t.Fatalf("instance = %+v, want pending with no current step", instance)
	}
	if len(mem.instanceCalls) != 0 {
		t.Fatalf("instance calls = %+v, want none for a gateless attach", mem.instanceCalls)
	}
}

func TestAttachUnknownDefinitionReturnsNotFound(t *testing.T) {
	mem := &memStore{}
	lifecycle := NewLifecycle(mem, replaceOSDRegistry(t), nil)

	_, err := lifecycle.Attach(context.Background(), operatorActor(), attachCaseID, "no-such-workflow", 1)
	if err == nil {
		t.Fatal("Attach of an unknown definition must fail")
	}
	class, ok := err.(apperr.Error)
	if !ok || class.Class != apperr.NotFound {
		t.Fatalf("error = %v, want a not-found error", err)
	}
	if len(mem.instanceCalls) != 0 {
		t.Fatalf("instance calls = %+v, want none before the registry resolves", mem.instanceCalls)
	}
}

// ApproveGate with a wired Dispatcher advances the instance and drives
// its Jobs within the call, parking at the human Task.
func TestApproveGateAdvancesAndDispatches(t *testing.T) {
	mem, parked := parkedReplaceOSD(t)
	adapter := &recordingAdapter{}
	lifecycle := NewLifecycle(mem, replaceOSDRegistry(t), New(mem, replaceOSDRegistry(t), adapter))

	result, err := lifecycle.ApproveGate(context.Background(), operatorActor(), parked.ID, "approve-destroy", "authorized")
	if err != nil {
		t.Fatalf("ApproveGate: %v", err)
	}
	if !result.Advanced {
		t.Fatalf("result = %+v, want Advanced", result)
	}
	if result.Record.GateID != "approve-destroy" || result.Record.Reason == nil || *result.Record.Reason != "authorized" {
		t.Fatalf("record = %+v, want the durable approval", result.Record)
	}
	if mem.instance.State != workflows.InstanceWaitingForOperator || mem.instance.CurrentStep == nil || *mem.instance.CurrentStep != "replace-device" {
		t.Fatalf("instance = %+v, want parked at the task after dispatch", mem.instance)
	}
	if mem.jobs[0].State != workflows.JobSucceeded || mem.jobs[1].State != workflows.JobSucceeded || mem.jobs[2].State != workflows.JobPending {
		t.Fatalf("jobs = %+v, want evidence and destroy done past the gate", mem.jobs)
	}
	if len(adapter.envelopes) != 2 {
		t.Fatalf("dispatches = %d, want 2 before the task", len(adapter.envelopes))
	}
	// The gate-resume transition is attributed to the approver.
	resume := mem.instanceCalls[len(mem.instanceCalls)-2]
	if resume.To != workflows.InstanceRunning || resume.Actor == nil || resume.Actor.Subject != "op-1" {
		t.Fatalf("resume transition = %+v, want attributed to the approver", resume)
	}
}

// Without a Dispatcher the gate still passes but nothing dispatches:
// the instance rests running with pending Jobs.
func TestApproveGateWithoutDispatcherRestsRunning(t *testing.T) {
	mem, parked := parkedReplaceOSD(t)
	lifecycle := NewLifecycle(mem, replaceOSDRegistry(t), nil)

	result, err := lifecycle.ApproveGate(context.Background(), operatorActor(), parked.ID, "approve-destroy", "")
	if err != nil {
		t.Fatalf("ApproveGate: %v", err)
	}
	if !result.Advanced {
		t.Fatalf("result = %+v, want Advanced", result)
	}
	if mem.instance.State != workflows.InstanceRunning {
		t.Fatalf("state = %s, want running with nothing dispatched", mem.instance.State)
	}
	for _, job := range mem.jobs {
		if job.State != workflows.JobPending {
			t.Fatalf("job %s = %s, want pending", job.StepID, job.State)
		}
	}
}

func TestApproveGateReplayIsIdempotentNoOp(t *testing.T) {
	mem, parked := parkedReplaceOSD(t)
	lifecycle := NewLifecycle(mem, replaceOSDRegistry(t), nil)

	first, err := lifecycle.ApproveGate(context.Background(), operatorActor(), parked.ID, "approve-destroy", "")
	if err != nil {
		t.Fatalf("first ApproveGate: %v", err)
	}
	callsAfterFirst := len(mem.instanceCalls)
	stateAfterFirst := mem.instance.State

	second, err := lifecycle.ApproveGate(context.Background(), store.Actor{Subject: "op-2", DisplayName: "Operator Two"}, parked.ID, "approve-destroy", "")
	if err != nil {
		t.Fatalf("replayed ApproveGate: %v", err)
	}
	if second.Advanced {
		t.Fatalf("replay = %+v, want not Advanced", second)
	}
	if second.Record.ID != first.Record.ID || second.Record.Approver.Subject != first.Record.Approver.Subject {
		t.Fatalf("replay record = %+v, want the original %+v", second.Record, first.Record)
	}
	if mem.instance.State != stateAfterFirst || len(mem.instanceCalls) != callsAfterFirst {
		t.Fatalf("replay moved the instance: state=%s calls=%+v", mem.instance.State, mem.instanceCalls)
	}
}

func TestApproveGateRejectsGateInstanceIsNotPausedAt(t *testing.T) {
	mem, parked := parkedReplaceOSD(t)
	lifecycle := NewLifecycle(mem, replaceOSDRegistry(t), nil)

	_, err := lifecycle.ApproveGate(context.Background(), operatorActor(), parked.ID, "approve-other", "")
	if err == nil {
		t.Fatal("approving a gate the instance is not paused at must fail")
	}
	if class, ok := err.(apperr.Error); !ok || class.Class != apperr.Conflict {
		t.Fatalf("error = %v, want a conflict provider error", err)
	}
}

// CompleteTask with a wired Dispatcher resumes the instance through its
// remaining Jobs to terminal succeeded within the call.
func TestCompleteTaskResumesAndDispatches(t *testing.T) {
	mem, parked := parkedReplaceOSD(t)
	adapter := &recordingAdapter{}
	lifecycle := NewLifecycle(mem, replaceOSDRegistry(t), New(mem, replaceOSDRegistry(t), adapter))
	if _, err := lifecycle.ApproveGate(context.Background(), operatorActor(), parked.ID, "approve-destroy", ""); err != nil {
		t.Fatalf("ApproveGate: %v", err)
	}

	result, err := lifecycle.CompleteTask(context.Background(), operatorActor(), parked.ID, "replace-device", "device swapped")
	if err != nil {
		t.Fatalf("CompleteTask: %v", err)
	}
	if !result.Advanced {
		t.Fatalf("result = %+v, want Advanced", result)
	}
	if result.Record.TaskID != "replace-device" || result.Record.Note == nil || *result.Record.Note != "device swapped" {
		t.Fatalf("record = %+v, want the durable completion", result.Record)
	}
	if mem.instance.State != workflows.InstanceSucceeded {
		t.Fatalf("state = %s, want terminal succeeded", mem.instance.State)
	}
	for _, job := range mem.jobs {
		if job.State != workflows.JobSucceeded {
			t.Fatalf("job %s = %s, want succeeded", job.StepID, job.State)
		}
	}
	if len(adapter.envelopes) != 3 {
		t.Fatalf("dispatches = %d, want 3 total", len(adapter.envelopes))
	}
}

func TestCompleteTaskReplayIsIdempotentNoOp(t *testing.T) {
	mem, parked := parkedReplaceOSD(t)
	adapter := &recordingAdapter{}
	lifecycle := NewLifecycle(mem, replaceOSDRegistry(t), New(mem, replaceOSDRegistry(t), adapter))
	if _, err := lifecycle.ApproveGate(context.Background(), operatorActor(), parked.ID, "approve-destroy", ""); err != nil {
		t.Fatalf("ApproveGate: %v", err)
	}
	first, err := lifecycle.CompleteTask(context.Background(), operatorActor(), parked.ID, "replace-device", "")
	if err != nil {
		t.Fatalf("first CompleteTask: %v", err)
	}
	callsAfterFirst := len(mem.instanceCalls)

	second, err := lifecycle.CompleteTask(context.Background(), store.Actor{Subject: "op-2", DisplayName: "Operator Two"}, parked.ID, "replace-device", "")
	if err != nil {
		t.Fatalf("replayed CompleteTask: %v", err)
	}
	if second.Advanced {
		t.Fatalf("replay = %+v, want not Advanced", second)
	}
	if second.Record.ID != first.Record.ID {
		t.Fatalf("replay record id = %d, want the original %d", second.Record.ID, first.Record.ID)
	}
	if mem.instance.State != workflows.InstanceSucceeded || len(mem.instanceCalls) != callsAfterFirst {
		t.Fatalf("replay moved the instance: state=%s calls=%+v", mem.instance.State, mem.instanceCalls)
	}
}

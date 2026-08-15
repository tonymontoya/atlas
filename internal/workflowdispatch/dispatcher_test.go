package workflowdispatch

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/tonymontoya/ceph-atlas/internal/agent"
	"github.com/tonymontoya/ceph-atlas/internal/cases"
	"github.com/tonymontoya/ceph-atlas/internal/operations"
	"github.com/tonymontoya/ceph-atlas/internal/providers"
	"github.com/tonymontoya/ceph-atlas/internal/store"
	"github.com/tonymontoya/ceph-atlas/internal/workflows"
)

// memStore is an in-memory Store that applies transitions and records
// them. Retry-budget semantics (the failed -> pending attempt bump and
// its refusal once the budget is spent) mirror the PostgreSQL store so
// dispatcher retry logic is exercised against the real contract; the
// remaining edge rules are proven by the store-backed integration tests.
type memStore struct {
	instance        store.WorkflowInstance
	jobs            []store.WorkflowJob
	approvals       []store.ApprovalRecord
	taskCompletions []store.TaskCompletionRecord
	target          cases.Case
	jobCalls        []store.WorkflowJobTransitionInput
	instanceCalls   []store.WorkflowInstanceTransitionInput
	getCaseErr      error
}

func (m *memStore) GetWorkflowInstance(_ context.Context, _ int64) (store.WorkflowInstance, error) {
	return m.instance, nil
}

func (m *memStore) TransitionWorkflowInstance(_ context.Context, input store.WorkflowInstanceTransitionInput) (store.WorkflowInstance, error) {
	m.instanceCalls = append(m.instanceCalls, input)
	m.instance.State = input.To
	if input.AtStep != "" {
		step := input.AtStep
		m.instance.CurrentStep = &step
		return m.instance, nil
	}
	m.instance.CurrentStep = nil
	return m.instance, nil
}

func (m *memStore) TransitionWorkflowJob(_ context.Context, input store.WorkflowJobTransitionInput) (store.WorkflowJob, error) {
	m.jobCalls = append(m.jobCalls, input)
	for i := range m.jobs {
		if m.jobs[i].ID != input.JobID {
			continue
		}
		if m.jobs[i].State == workflows.JobFailed && input.To == workflows.JobPending {
			if m.jobs[i].Attempt >= m.jobs[i].MaxAttempts {
				return store.WorkflowJob{}, providers.ProviderError{
					Class:   providers.ErrorConflict,
					Message: "job exhausted its retry budget",
				}
			}
			m.jobs[i].Attempt++
		}
		m.jobs[i].State = input.To
		return m.jobs[i], nil
	}
	return store.WorkflowJob{}, errors.New("unknown job")
}

func (m *memStore) ListWorkflowJobs(_ context.Context, _ int64) ([]store.WorkflowJob, error) {
	return m.jobs, nil
}

func (m *memStore) ListWorkflowApprovals(_ context.Context, _ int64) ([]store.ApprovalRecord, error) {
	return m.approvals, nil
}

func (m *memStore) ListWorkflowTaskCompletions(_ context.Context, _ int64) ([]store.TaskCompletionRecord, error) {
	return m.taskCompletions, nil
}

func (m *memStore) GetCase(_ context.Context, _ int64) (cases.Case, error) {
	return m.target, m.getCaseErr
}

// recordingAdapter captures every serialized envelope and answers from
// a scripted list of results, one per dispatch.
type recordingAdapter struct {
	envelopes []operations.RequestEnvelope
	raw       [][]byte
	results   []agent.Result
	errs      []error
}

func (r *recordingAdapter) Dispatch(_ context.Context, envelope []byte) (agent.Result, error) {
	var decoded operations.RequestEnvelope
	if err := json.Unmarshal(envelope, &decoded); err != nil {
		return agent.Result{}, err
	}
	r.envelopes = append(r.envelopes, decoded)
	r.raw = append(r.raw, envelope)
	index := len(r.envelopes) - 1
	var err error
	if index < len(r.errs) {
		err = r.errs[index]
	}
	if index < len(r.results) {
		return r.results[index], err
	}
	return agent.Result{Outcome: agent.OutcomeSucceeded, Detail: "ok"}, err
}

func replaceOSDRegistry(t *testing.T) *workflows.CodeRegistry {
	t.Helper()
	ops, err := operations.DefaultRegistry()
	if err != nil {
		t.Fatalf("operations registry: %v", err)
	}
	registry, err := workflows.DefaultRegistry(ops)
	if err != nil {
		t.Fatalf("workflow registry: %v", err)
	}
	return registry
}

// runningReplaceOSD builds a memStore holding a running replace-osd
// instance whose gate was approved, with every Job pending.
func runningReplaceOSD(t *testing.T) *memStore {
	t.Helper()
	return &memStore{
		instance: store.WorkflowInstance{
			ID: 42, CaseID: 7, DefinitionID: "replace-osd", DefinitionVersion: 1,
			State: workflows.InstanceRunning,
		},
		jobs: []store.WorkflowJob{
			{ID: 101, WorkflowInstanceID: 42, Position: 1, StepID: "collect-evidence", OperationType: "CollectHostEvidence", State: workflows.JobPending, Attempt: 1, MaxAttempts: 3},
			{ID: 102, WorkflowInstanceID: 42, Position: 3, StepID: "destroy-osd", OperationType: "DestroyOSD", State: workflows.JobPending, Attempt: 1, MaxAttempts: 1},
			{ID: 103, WorkflowInstanceID: 42, Position: 5, StepID: "verify-osd", OperationType: "VerifyOSD", State: workflows.JobPending, Attempt: 1, MaxAttempts: 3},
		},
		approvals: []store.ApprovalRecord{
			{ID: 9, WorkflowInstanceID: 42, GateID: "approve-destroy", Approver: store.Actor{Subject: "op-1", DisplayName: "Operator One"}},
		},
		target: cases.Case{ID: 7, ClusterFSID: "11111111-1111-4000-8000-000000000101"},
	}
}

// completeReplaceDevice seeds the memStore with the durable Task
// completion record an Operator's resume leaves behind.
func completeReplaceDevice(mem *memStore) {
	mem.taskCompletions = append(mem.taskCompletions, store.TaskCompletionRecord{
		ID: 11, WorkflowInstanceID: 42, TaskID: "replace-device",
		Operator: store.Actor{Subject: "op-1", DisplayName: "Operator One"},
	})
}

func TestRunPausesAtTaskThenResumesToSucceeded(t *testing.T) {
	mem := runningReplaceOSD(t)
	adapter := &recordingAdapter{}
	dispatcher := New(mem, replaceOSDRegistry(t), adapter)

	paused, err := dispatcher.Run(context.Background(), 42)
	if err != nil {
		t.Fatalf("Run to task: %v", err)
	}
	if paused.State != workflows.InstanceWaitingForOperator || paused.CurrentStep == nil || *paused.CurrentStep != "replace-device" {
		t.Fatalf("instance = %+v, want waiting_for_operator at replace-device", paused)
	}
	if mem.jobs[0].State != workflows.JobSucceeded || mem.jobs[1].State != workflows.JobSucceeded {
		t.Fatalf("jobs before the task = %+v, want succeeded", mem.jobs)
	}
	if mem.jobs[2].State != workflows.JobPending {
		t.Fatalf("verify job = %s, want untouched pending past the task", mem.jobs[2].State)
	}
	if len(adapter.envelopes) != 2 {
		t.Fatalf("dispatches = %d, want 2 before the task", len(adapter.envelopes))
	}
	if len(mem.instanceCalls) != 1 || mem.instanceCalls[0].To != workflows.InstanceWaitingForOperator || mem.instanceCalls[0].AtStep != "replace-device" {
		t.Fatalf("instance calls = %+v, want the task pause", mem.instanceCalls)
	}

	// The Operator finishes the Task and resumes the instance.
	completeReplaceDevice(mem)
	if _, err := mem.TransitionWorkflowInstance(context.Background(), store.WorkflowInstanceTransitionInput{
		InstanceID: 42, To: workflows.InstanceRunning,
	}); err != nil {
		t.Fatalf("resume: %v", err)
	}

	finished, err := dispatcher.Run(context.Background(), 42)
	if err != nil {
		t.Fatalf("Run after resume: %v", err)
	}
	if finished.State != workflows.InstanceSucceeded {
		t.Fatalf("instance state = %s, want succeeded", finished.State)
	}
	for _, job := range mem.jobs {
		if job.State != workflows.JobSucceeded {
			t.Fatalf("job %s state = %s, want succeeded", job.StepID, job.State)
		}
	}
	if len(adapter.envelopes) != 3 {
		t.Fatalf("dispatches = %d, want 3 total", len(adapter.envelopes))
	}

	// Jobs advance pending -> dispatched -> succeeded, in definition order.
	wantJobs := []struct {
		id    int64
		steps []workflows.JobState
	}{
		{101, []workflows.JobState{workflows.JobDispatched, workflows.JobSucceeded}},
		{102, []workflows.JobState{workflows.JobDispatched, workflows.JobSucceeded}},
		{103, []workflows.JobState{workflows.JobDispatched, workflows.JobSucceeded}},
	}
	for i, want := range wantJobs {
		for j, wantState := range want.steps {
			call := mem.jobCalls[i*2+j]
			if call.JobID != want.id || call.To != wantState {
				t.Fatalf("job call %d = %+v, want job %d -> %s", i*2+j, call, want.id, wantState)
			}
		}
	}
}

func TestRunEnvelopeCarriesApprovalActorContextAndDeterministicKeys(t *testing.T) {
	mem := runningReplaceOSD(t)
	adapter := &recordingAdapter{}
	dispatcher := New(mem, replaceOSDRegistry(t), adapter)

	if _, err := dispatcher.Run(context.Background(), 42); err != nil {
		t.Fatalf("Run: %v", err)
	}

	destroy := adapter.envelopes[1]
	if destroy.OperationType != "DestroyOSD" {
		t.Fatalf("second dispatch operation = %s, want DestroyOSD", destroy.OperationType)
	}
	if destroy.Actor.Subject != "op-1" || destroy.Actor.DisplayName != "Operator One" {
		t.Fatalf("actor = %+v, want the gate approver", destroy.Actor)
	}
	if destroy.Approval == nil || destroy.Approval.ApprovalID != 9 || destroy.Approval.Approver != "op-1" {
		t.Fatalf("approval context = %+v, want the gate approval", destroy.Approval)
	}
	if destroy.IdempotencyKey != "instance-42-job-102-attempt-1" {
		t.Fatalf("idempotency key = %q", destroy.IdempotencyKey)
	}
	if destroy.AuditCorrelationID == "" {
		t.Fatal("audit correlation id is empty")
	}
	var params map[string]any
	if err := json.Unmarshal(destroy.Parameters, &params); err != nil {
		t.Fatalf("destroy parameters: %v", err)
	}
	if params["clusterFsid"] != "11111111-1111-4000-8000-000000000101" {
		t.Fatalf("destroy parameters = %s, want the case cluster fsid", destroy.Parameters)
	}

	evidence := adapter.envelopes[0]
	if evidence.OperationType != "CollectHostEvidence" {
		t.Fatalf("first dispatch operation = %s, want CollectHostEvidence", evidence.OperationType)
	}
	if err := json.Unmarshal(evidence.Parameters, &operations.CollectHostEvidence{}); err != nil {
		t.Fatalf("evidence parameters are not the typed shape: %v", err)
	}
	if evidence.Approval == nil {
		t.Fatal("evidence approval context missing; the run executes under the gate approval")
	}
}

func TestRunFailsInstanceWhenJobFails(t *testing.T) {
	mem := runningReplaceOSD(t)
	adapter := &recordingAdapter{results: []agent.Result{
		{Outcome: agent.OutcomeSucceeded},
		{Outcome: agent.OutcomeFailed, Detail: "osd refused"},
	}}
	dispatcher := New(mem, replaceOSDRegistry(t), adapter)

	instance, err := dispatcher.Run(context.Background(), 42)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if instance.State != workflows.InstanceFailed {
		t.Fatalf("instance state = %s, want failed", instance.State)
	}
	if mem.jobs[0].State != workflows.JobSucceeded || mem.jobs[1].State != workflows.JobFailed {
		t.Fatalf("jobs = %+v, want first succeeded second failed", mem.jobs)
	}
	// destroy-osd has a one-attempt budget: the failure exhausts it and
	// no retry may fire.
	if mem.jobs[1].Attempt != 1 {
		t.Fatalf("destroy attempt = %d, want 1 (no auto-retry for the destructive step)", mem.jobs[1].Attempt)
	}
	if mem.jobs[2].State != workflows.JobPending {
		t.Fatalf("verify job = %s, want untouched pending", mem.jobs[2].State)
	}
	if len(adapter.envelopes) != 2 {
		t.Fatalf("dispatches = %d, want 2 (no dispatch past failure)", len(adapter.envelopes))
	}
}

// A transient failure consumes one attempt and the retry policy re-pends
// and re-dispatches the Job; the retried dispatch carries a new
// idempotency key.
func TestRunRetriesFailedJobPerPolicy(t *testing.T) {
	mem := runningReplaceOSD(t)
	completeReplaceDevice(mem)
	adapter := &recordingAdapter{results: []agent.Result{
		{Outcome: agent.OutcomeFailed, Detail: "transient evidence hiccup"},
	}}
	dispatcher := New(mem, replaceOSDRegistry(t), adapter)

	finished, err := dispatcher.Run(context.Background(), 42)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if finished.State != workflows.InstanceSucceeded {
		t.Fatalf("instance state = %s, want recovered succeeded", finished.State)
	}
	if mem.jobs[0].State != workflows.JobSucceeded || mem.jobs[0].Attempt != 2 {
		t.Fatalf("collect job = %+v, want succeeded on attempt 2", mem.jobs[0])
	}

	wantCalls := []workflows.JobState{
		workflows.JobDispatched, workflows.JobFailed, workflows.JobPending,
		workflows.JobDispatched, workflows.JobSucceeded,
	}
	for i, want := range wantCalls {
		call := mem.jobCalls[i]
		if call.JobID != 101 || call.To != want {
			t.Fatalf("job call %d = %+v, want job 101 -> %s", i, call, want)
		}
	}
	if len(adapter.envelopes) != 4 {
		t.Fatalf("dispatches = %d, want 4 (retry + remaining jobs)", len(adapter.envelopes))
	}
	if adapter.envelopes[0].IdempotencyKey != "instance-42-job-101-attempt-1" {
		t.Fatalf("first attempt key = %q", adapter.envelopes[0].IdempotencyKey)
	}
	if adapter.envelopes[1].IdempotencyKey != "instance-42-job-101-attempt-2" {
		t.Fatalf("retry key = %q, want a fresh key for the new attempt", adapter.envelopes[1].IdempotencyKey)
	}
}

// When every allowed attempt fails, the retry budget is spent, the Job
// rests failed, and the instance fails terminally.
func TestRunFailsInstanceWhenRetriesExhaust(t *testing.T) {
	mem := runningReplaceOSD(t)
	adapter := &recordingAdapter{results: []agent.Result{
		{Outcome: agent.OutcomeFailed, Detail: "attempt 1"},
		{Outcome: agent.OutcomeFailed, Detail: "attempt 2"},
		{Outcome: agent.OutcomeFailed, Detail: "attempt 3"},
	}}
	dispatcher := New(mem, replaceOSDRegistry(t), adapter)

	instance, err := dispatcher.Run(context.Background(), 42)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if instance.State != workflows.InstanceFailed {
		t.Fatalf("instance state = %s, want failed", instance.State)
	}
	if mem.jobs[0].State != workflows.JobFailed || mem.jobs[0].Attempt != 3 {
		t.Fatalf("collect job = %+v, want failed after 3 attempts", mem.jobs[0])
	}
	if mem.jobs[1].State != workflows.JobPending || mem.jobs[2].State != workflows.JobPending {
		t.Fatalf("later jobs = %+v, want untouched pending", mem.jobs)
	}
	if len(adapter.envelopes) != 3 {
		t.Fatalf("dispatches = %d, want exactly the 3 allowed attempts", len(adapter.envelopes))
	}
	if len(mem.instanceCalls) != 1 || mem.instanceCalls[0].To != workflows.InstanceFailed {
		t.Fatalf("instance calls = %+v, want one terminal failed", mem.instanceCalls)
	}
}

// An adapter error is a retriable Job failure, not a dispatcher error.
func TestRunTreatsAdapterErrorAsRetriableJobFailure(t *testing.T) {
	mem := runningReplaceOSD(t)
	completeReplaceDevice(mem)
	adapter := &recordingAdapter{errs: []error{errors.New("agent unreachable")}}
	dispatcher := New(mem, replaceOSDRegistry(t), adapter)

	instance, err := dispatcher.Run(context.Background(), 42)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if instance.State != workflows.InstanceSucceeded {
		t.Fatalf("instance state = %s, want recovered after retry", instance.State)
	}
	if mem.jobs[0].State != workflows.JobSucceeded || mem.jobs[0].Attempt != 2 {
		t.Fatalf("collect job = %+v, want succeeded on attempt 2", mem.jobs[0])
	}
}

func TestRunLeavesNonRunningInstancesUntouched(t *testing.T) {
	for name, state := range map[string]workflows.InstanceState{
		"pending":   workflows.InstancePending,
		"at gate":   workflows.InstanceWaitingForApproval,
		"at task":   workflows.InstanceWaitingForOperator,
		"succeeded": workflows.InstanceSucceeded,
		"failed":    workflows.InstanceFailed,
		"cancelled": workflows.InstanceCancelled,
	} {
		t.Run(name, func(t *testing.T) {
			mem := runningReplaceOSD(t)
			mem.instance.State = state
			adapter := &recordingAdapter{}
			dispatcher := New(mem, replaceOSDRegistry(t), adapter)

			instance, err := dispatcher.Run(context.Background(), 42)
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if instance.State != state {
				t.Fatalf("state = %s, want untouched %s", instance.State, state)
			}
			if len(adapter.envelopes) != 0 || len(mem.jobCalls) != 0 || len(mem.instanceCalls) != 0 {
				t.Fatal("a non-running instance must not be dispatched")
			}
		})
	}
}

func TestRunStopsAtUnapprovedGate(t *testing.T) {
	mem := runningReplaceOSD(t)
	mem.approvals = nil
	adapter := &recordingAdapter{}
	dispatcher := New(mem, replaceOSDRegistry(t), adapter)

	instance, err := dispatcher.Run(context.Background(), 42)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// collect-evidence ran before the gate; the walk stops at the gate
	// and the instance stays running rather than executing destroy-osd.
	if instance.State != workflows.InstanceRunning {
		t.Fatalf("state = %s, want still running", instance.State)
	}
	if mem.jobs[0].State != workflows.JobSucceeded || mem.jobs[1].State != workflows.JobPending {
		t.Fatalf("jobs = %+v, want evidence done and destroy untouched", mem.jobs)
	}
	if len(mem.instanceCalls) != 0 {
		t.Fatalf("instance calls = %+v, want none", mem.instanceCalls)
	}
}

// A Job left dispatched by a dead process is re-dispatched under the
// SAME idempotency key: the agent replays the recorded outcome instead
// of executing again.
func TestRunReDispatchesInFlightJobWithSameKey(t *testing.T) {
	mem := runningReplaceOSD(t)
	completeReplaceDevice(mem)
	mem.jobs[0].State = workflows.JobDispatched
	adapter := &recordingAdapter{}
	dispatcher := New(mem, replaceOSDRegistry(t), adapter)

	finished, err := dispatcher.Run(context.Background(), 42)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if finished.State != workflows.InstanceSucceeded {
		t.Fatalf("instance state = %s, want recovered succeeded", finished.State)
	}
	if mem.jobs[0].State != workflows.JobSucceeded || mem.jobs[0].Attempt != 1 {
		t.Fatalf("collect job = %+v, want succeeded on the same attempt", mem.jobs[0])
	}
	if len(adapter.envelopes) != 3 {
		t.Fatalf("dispatches = %d, want 3", len(adapter.envelopes))
	}
	if adapter.envelopes[0].JobID != 101 || adapter.envelopes[0].IdempotencyKey != "instance-42-job-101-attempt-1" {
		t.Fatalf("re-dispatch = %+v, want job 101 under its original attempt key", adapter.envelopes[0])
	}
	// The in-flight Job needs no dispatched transition: it is already
	// dispatched; only the terminal outcome is recorded.
	if len(mem.jobCalls) != 5 {
		t.Fatalf("job calls = %d, want 5 (one outcome for job 101, full lifecycle for the rest)", len(mem.jobCalls))
	}
	if mem.jobCalls[0].JobID != 101 || mem.jobCalls[0].To != workflows.JobSucceeded {
		t.Fatalf("first job call = %+v, want job 101 -> succeeded", mem.jobCalls[0])
	}
}

// A Job left failed by a dead process with budget remaining is re-pended
// and re-dispatched under a fresh attempt key.
func TestRunRequeuesFailedJobWithRemainingBudget(t *testing.T) {
	mem := runningReplaceOSD(t)
	completeReplaceDevice(mem)
	mem.jobs[0].State = workflows.JobFailed
	mem.jobs[0].Attempt = 1
	adapter := &recordingAdapter{}
	dispatcher := New(mem, replaceOSDRegistry(t), adapter)

	finished, err := dispatcher.Run(context.Background(), 42)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if finished.State != workflows.InstanceSucceeded {
		t.Fatalf("instance state = %s, want recovered succeeded", finished.State)
	}
	if mem.jobs[0].Attempt != 2 || mem.jobs[0].State != workflows.JobSucceeded {
		t.Fatalf("collect job = %+v, want re-run on attempt 2", mem.jobs[0])
	}
	wantFirst := []workflows.JobState{workflows.JobPending, workflows.JobDispatched, workflows.JobSucceeded}
	for i, want := range wantFirst {
		if mem.jobCalls[i].JobID != 101 || mem.jobCalls[i].To != want {
			t.Fatalf("job call %d = %+v, want job 101 -> %s", i, mem.jobCalls[i], want)
		}
	}
}

// A Job left failed by a dead process with its budget already spent
// fails the instance without another dispatch.
func TestRunFailsInstanceOnJobFailedWithSpentBudget(t *testing.T) {
	mem := runningReplaceOSD(t)
	mem.jobs[0].State = workflows.JobFailed
	mem.jobs[0].Attempt = 3
	adapter := &recordingAdapter{}
	dispatcher := New(mem, replaceOSDRegistry(t), adapter)

	instance, err := dispatcher.Run(context.Background(), 42)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if instance.State != workflows.InstanceFailed {
		t.Fatalf("instance state = %s, want failed", instance.State)
	}
	if len(adapter.envelopes) != 0 {
		t.Fatalf("dispatches = %d, want none", len(adapter.envelopes))
	}
}

func TestRunResumesPastSucceededJobs(t *testing.T) {
	mem := runningReplaceOSD(t)
	completeReplaceDevice(mem)
	mem.jobs[0].State = workflows.JobSucceeded
	adapter := &recordingAdapter{}
	dispatcher := New(mem, replaceOSDRegistry(t), adapter)

	if _, err := dispatcher.Run(context.Background(), 42); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, envelope := range adapter.envelopes {
		if envelope.JobID == 101 {
			t.Fatal("a succeeded job must not re-dispatch")
		}
	}
	if len(adapter.envelopes) != 2 {
		t.Fatalf("dispatches = %d, want 2", len(adapter.envelopes))
	}
}

func TestRunUsesSystemActorWithoutApprovals(t *testing.T) {
	// A gateless definition run has no approval record: the Atlas
	// system actor executes and no approval context travels.
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
	mem := &memStore{
		instance: store.WorkflowInstance{ID: 5, CaseID: 2, DefinitionID: "gateless", DefinitionVersion: 1, State: workflows.InstanceRunning},
		jobs: []store.WorkflowJob{
			{ID: 51, WorkflowInstanceID: 5, Position: 1, StepID: "collect-evidence", OperationType: "CollectHostEvidence", State: workflows.JobPending, Attempt: 1, MaxAttempts: 3},
		},
		target: cases.Case{ID: 2},
	}
	adapter := &recordingAdapter{}
	dispatcher := New(mem, registry, adapter)

	if _, err := dispatcher.Run(context.Background(), 5); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(adapter.envelopes) != 1 {
		t.Fatalf("dispatches = %d, want 1", len(adapter.envelopes))
	}
	envelope := adapter.envelopes[0]
	if envelope.Actor.Subject != "atlas" || envelope.Actor.DisplayName != "Atlas" {
		t.Fatalf("actor = %+v, want the Atlas system actor", envelope.Actor)
	}
	if envelope.Approval != nil {
		t.Fatalf("approval = %+v, want none", envelope.Approval)
	}
}

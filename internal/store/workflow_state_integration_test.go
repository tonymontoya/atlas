package store

import (
	"context"
	"testing"

	"github.com/tonymontoya/ceph-atlas/internal/cases"
	"github.com/tonymontoya/ceph-atlas/internal/providers"
	"github.com/tonymontoya/ceph-atlas/internal/workflows"
)

func workflowStateTestDB(t *testing.T) (*PostgresStore, context.Context) {
	t.Helper()
	store, ctx := manualWritesTestDB(t)
	workflowStateCleanup(t, store)
	t.Cleanup(func() { workflowStateCleanup(t, store) })
	return store, ctx
}

func workflowStateCleanup(t *testing.T, store *PostgresStore) {
	t.Helper()
	ctx := context.Background()
	statements := []string{
		`DELETE FROM workflow_approvals`,
		`DELETE FROM workflow_jobs`,
		`DELETE FROM workflow_instances WHERE definition_id LIKE 'wf-test%'`,
		`DELETE FROM case_timeline_events WHERE case_id IN (SELECT id FROM cases WHERE title LIKE 'manual-test%')`,
		`DELETE FROM cases WHERE title LIKE 'manual-test%'`,
	}
	for _, statement := range statements {
		if _, err := store.db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("cleanup %q: %v", statement, err)
		}
	}
}

func workflowTestCaseID(t *testing.T, store *PostgresStore, ctx context.Context, status cases.CaseStatus) int64 {
	t.Helper()
	created, err := store.CreateManualCase(ctx, ManualCaseInput{
		Title:    "manual-test workflow target",
		Summary:  "manual-test summary",
		Severity: string(cases.CaseSeverityMedium),
		Actor:    manualTestActor(),
	})
	if err != nil {
		t.Fatalf("CreateManualCase: %v", err)
	}
	if status != cases.CaseStatusDetected && status != cases.CaseStatusClosed {
		t.Fatalf("unsupported fixture status %s", status)
	}
	if status == cases.CaseStatusClosed {
		closed, err := store.TransitionCase(ctx, CaseTransitionInput{CaseID: created.ID, To: cases.CaseStatusClosed, Actor: manualTestActor()})
		if err != nil {
			t.Fatalf("TransitionCase: %v", err)
		}
		return closed.ID
	}
	return created.ID
}

func validWorkflowInstanceInput(caseID int64) CreateWorkflowInstanceInput {
	return CreateWorkflowInstanceInput{
		CaseID:            caseID,
		DefinitionID:      "wf-test-replace-osd",
		DefinitionVersion: 1,
		Jobs: []WorkflowJobInput{
			{StepID: "collect-evidence", OperationType: "CollectHostEvidence", MaxAttempts: 3},
			{StepID: "destroy-osd", OperationType: "DestroyOSD", MaxAttempts: 1},
		},
		Actor: manualTestActor(),
	}
}

func TestCreateWorkflowInstanceWritesInstanceJobsAndAttachEvent(t *testing.T) {
	store, ctx := workflowStateTestDB(t)
	caseID := workflowTestCaseID(t, store, ctx, cases.CaseStatusDetected)

	instance, err := store.CreateWorkflowInstance(ctx, validWorkflowInstanceInput(caseID))
	if err != nil {
		t.Fatalf("CreateWorkflowInstance: %v", err)
	}
	if instance.ID == 0 || instance.CaseID != caseID {
		t.Fatalf("instance = %+v, want case %d", instance, caseID)
	}
	if instance.State != workflows.InstancePending {
		t.Fatalf("state = %s, want pending", instance.State)
	}
	if instance.DefinitionID != "wf-test-replace-osd" || instance.DefinitionVersion != 1 {
		t.Fatalf("definition = %s v%d", instance.DefinitionID, instance.DefinitionVersion)
	}
	if instance.CurrentStep != nil {
		t.Fatalf("current step = %v, want nil", *instance.CurrentStep)
	}

	jobs, err := store.ListWorkflowJobs(ctx, instance.ID)
	if err != nil {
		t.Fatalf("ListWorkflowJobs: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("jobs = %d, want 2", len(jobs))
	}
	wantSteps := []struct {
		stepID   string
		state    workflows.JobState
		attempt  int
		maxAtmp  int
		position int
	}{
		{"collect-evidence", workflows.JobPending, 1, 3, 1},
		{"destroy-osd", workflows.JobPending, 1, 1, 2},
	}
	for i, job := range jobs {
		want := wantSteps[i]
		if job.StepID != want.stepID || job.State != want.state || job.Attempt != want.attempt || job.MaxAttempts != want.maxAtmp || job.Position != want.position {
			t.Fatalf("job %d = %+v, want %+v", i, job, want)
		}
	}

	timeline, err := store.ListCaseTimeline(ctx, caseID)
	if err != nil {
		t.Fatalf("ListCaseTimeline: %v", err)
	}
	var attach *cases.TimelineEvent
	for i := range timeline {
		if timeline[i].Type == cases.TimelineEventWorkflowAttached {
			attach = &timeline[i]
		}
	}
	if attach == nil {
		t.Fatalf("timeline = %+v, want a workflow_attached event", timeline)
	}
	if attach.Actor.Type != cases.TimelineActorUser || attach.Actor.ID != "manual-test-operator" {
		t.Fatalf("attach actor = %+v, want the operator", attach.Actor)
	}
	if attach.Payload["workflowId"] != "wf-test-replace-osd" {
		t.Fatalf("attach payload = %+v, want workflowId", attach.Payload)
	}
}

func TestCreateWorkflowInstanceRejectsInvalidInput(t *testing.T) {
	store, ctx := workflowStateTestDB(t)
	caseID := workflowTestCaseID(t, store, ctx, cases.CaseStatusDetected)

	mutators := []struct {
		name   string
		mutate func(*CreateWorkflowInstanceInput)
	}{
		{"empty definition id", func(i *CreateWorkflowInstanceInput) { i.DefinitionID = "" }},
		{"zero definition version", func(i *CreateWorkflowInstanceInput) { i.DefinitionVersion = 0 }},
		{"no jobs", func(i *CreateWorkflowInstanceInput) { i.Jobs = nil }},
		{"job without step id", func(i *CreateWorkflowInstanceInput) { i.Jobs[0].StepID = "" }},
		{"job without operation type", func(i *CreateWorkflowInstanceInput) { i.Jobs[0].OperationType = "" }},
		{"job without max attempts", func(i *CreateWorkflowInstanceInput) { i.Jobs[0].MaxAttempts = 0 }},
		{"duplicate step ids", func(i *CreateWorkflowInstanceInput) {
			i.Jobs = append(i.Jobs, WorkflowJobInput{StepID: "collect-evidence", OperationType: "VerifyOSD", MaxAttempts: 2})
		}},
		{"actor without subject", func(i *CreateWorkflowInstanceInput) { i.Actor = Actor{Subject: "", DisplayName: "Name"} }},
	}
	for _, m := range mutators {
		t.Run(m.name, func(t *testing.T) {
			input := validWorkflowInstanceInput(caseID)
			m.mutate(&input)
			if _, err := store.CreateWorkflowInstance(ctx, input); !isInvalidInput(err) {
				t.Fatalf("error = %v, want InvalidInputError", err)
			}
		})
	}
}

func TestCreateWorkflowInstanceRejectsMissingAndClosedCases(t *testing.T) {
	store, ctx := workflowStateTestDB(t)

	if _, err := store.CreateWorkflowInstance(ctx, validWorkflowInstanceInput(999999)); !isNotFound(err) {
		t.Fatalf("missing case error = %v, want not found", err)
	}

	closedCaseID := workflowTestCaseID(t, store, ctx, cases.CaseStatusClosed)
	_, err := store.CreateWorkflowInstance(ctx, validWorkflowInstanceInput(closedCaseID))
	if err == nil {
		t.Fatal("attached a workflow to a closed case")
	}
	if providerErr, ok := err.(providers.ProviderError); !ok || providerErr.Class != providers.ErrorConflict {
		t.Fatalf("closed case error = %v, want conflict", err)
	}
}

func TestGetWorkflowInstance(t *testing.T) {
	store, ctx := workflowStateTestDB(t)
	caseID := workflowTestCaseID(t, store, ctx, cases.CaseStatusDetected)
	created, err := store.CreateWorkflowInstance(ctx, validWorkflowInstanceInput(caseID))
	if err != nil {
		t.Fatalf("CreateWorkflowInstance: %v", err)
	}

	got, err := store.GetWorkflowInstance(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetWorkflowInstance: %v", err)
	}
	if got.ID != created.ID || got.State != workflows.InstancePending {
		t.Fatalf("instance = %+v, want pending %d", got, created.ID)
	}

	if _, err := store.GetWorkflowInstance(ctx, 999999); !isNotFound(err) {
		t.Fatalf("missing instance error = %v, want not found", err)
	}
}

func attachTestInstance(t *testing.T, store *PostgresStore, ctx context.Context) WorkflowInstance {
	t.Helper()
	caseID := workflowTestCaseID(t, store, ctx, cases.CaseStatusDetected)
	instance, err := store.CreateWorkflowInstance(ctx, validWorkflowInstanceInput(caseID))
	if err != nil {
		t.Fatalf("CreateWorkflowInstance: %v", err)
	}
	return instance
}

func TestWorkflowInstanceLifecycleTransitions(t *testing.T) {
	store, ctx := workflowStateTestDB(t)
	instance := attachTestInstance(t, store, ctx)

	running, err := store.TransitionWorkflowInstance(ctx, WorkflowInstanceTransitionInput{InstanceID: instance.ID, To: workflows.InstanceRunning})
	if err != nil {
		t.Fatalf("pending -> running: %v", err)
	}
	if running.State != workflows.InstanceRunning || running.CurrentStep != nil {
		t.Fatalf("running = %+v, want running with no current step", running)
	}

	waiting, err := store.TransitionWorkflowInstance(ctx, WorkflowInstanceTransitionInput{
		InstanceID: instance.ID,
		To:         workflows.InstanceWaitingForApproval,
		AtStep:     "approve-destroy",
		Actor:      &[]Actor{manualTestActor()}[0],
	})
	if err != nil {
		t.Fatalf("running -> waiting_for_approval: %v", err)
	}
	if waiting.State != workflows.InstanceWaitingForApproval || waiting.CurrentStep == nil || *waiting.CurrentStep != "approve-destroy" {
		t.Fatalf("waiting = %+v, want waiting at approve-destroy", waiting)
	}

	resumed, err := store.TransitionWorkflowInstance(ctx, WorkflowInstanceTransitionInput{InstanceID: instance.ID, To: workflows.InstanceRunning})
	if err != nil {
		t.Fatalf("waiting_for_approval -> running: %v", err)
	}
	if resumed.CurrentStep != nil {
		t.Fatalf("resumed current step = %v, want cleared", *resumed.CurrentStep)
	}

	done, err := store.TransitionWorkflowInstance(ctx, WorkflowInstanceTransitionInput{InstanceID: instance.ID, To: workflows.InstanceSucceeded})
	if err != nil {
		t.Fatalf("running -> succeeded: %v", err)
	}
	if done.FinishedAt == nil {
		t.Fatal("terminal instance has no finished_at")
	}

	timeline, err := store.ListCaseTimeline(ctx, instance.CaseID)
	if err != nil {
		t.Fatalf("ListCaseTimeline: %v", err)
	}
	changed := 0
	var userEvent, systemEvent bool
	for _, event := range timeline {
		if event.Type != cases.TimelineEventWorkflowStateChanged {
			continue
		}
		changed++
		switch {
		case event.Actor.Type == cases.TimelineActorUser && event.Actor.ID == "manual-test-operator":
			userEvent = true
		case event.Actor.Type == cases.TimelineActorSystem && event.Actor.DisplayName == "Atlas":
			systemEvent = true
		}
	}
	if changed != 4 || !userEvent || !systemEvent {
		t.Fatalf("state-changed events = %d (user=%v system=%v), want 4 with both actor kinds", changed, userEvent, systemEvent)
	}
}

func TestWorkflowInstanceTransitionsRejectInvalidAndTerminal(t *testing.T) {
	store, ctx := workflowStateTestDB(t)
	instance := attachTestInstance(t, store, ctx)

	if _, err := store.TransitionWorkflowInstance(ctx, WorkflowInstanceTransitionInput{InstanceID: instance.ID, To: workflows.InstancePending}); !isConflict(err) {
		t.Fatalf("self-edge error = %v, want conflict", err)
	}
	if _, err := store.TransitionWorkflowInstance(ctx, WorkflowInstanceTransitionInput{InstanceID: instance.ID, To: "paused"}); !isInvalidInput(err) {
		t.Fatalf("unknown state error = %v, want InvalidInputError", err)
	}
	if _, err := store.TransitionWorkflowInstance(ctx, WorkflowInstanceTransitionInput{InstanceID: instance.ID, To: workflows.InstanceWaitingForApproval}); !isInvalidInput(err) {
		t.Fatalf("pause without step error = %v, want InvalidInputError", err)
	}
	if _, err := store.TransitionWorkflowInstance(ctx, WorkflowInstanceTransitionInput{InstanceID: 999999, To: workflows.InstanceRunning}); !isNotFound(err) {
		t.Fatalf("missing instance error = %v, want not found", err)
	}

	if _, err := store.TransitionWorkflowInstance(ctx, WorkflowInstanceTransitionInput{InstanceID: instance.ID, To: workflows.InstanceRunning}); err != nil {
		t.Fatalf("pending -> running: %v", err)
	}
	if _, err := store.TransitionWorkflowInstance(ctx, WorkflowInstanceTransitionInput{InstanceID: instance.ID, To: workflows.InstanceSucceeded}); err != nil {
		t.Fatalf("running -> succeeded: %v", err)
	}
	if _, err := store.TransitionWorkflowInstance(ctx, WorkflowInstanceTransitionInput{InstanceID: instance.ID, To: workflows.InstanceRunning}); !isConflict(err) {
		t.Fatalf("terminal error = %v, want conflict", err)
	}
}

func isConflict(err error) bool {
	providerErr, ok := err.(providers.ProviderError)
	return ok && providerErr.Class == providers.ErrorConflict
}

func firstPendingJob(t *testing.T, store *PostgresStore, ctx context.Context, instanceID int64) WorkflowJob {
	t.Helper()
	jobs, err := store.ListWorkflowJobs(ctx, instanceID)
	if err != nil {
		t.Fatalf("ListWorkflowJobs: %v", err)
	}
	return jobs[0]
}

func TestWorkflowJobLifecycleThroughSucceed(t *testing.T) {
	store, ctx := workflowStateTestDB(t)
	instance := attachTestInstance(t, store, ctx)
	job := firstPendingJob(t, store, ctx, instance.ID)

	dispatched, err := store.TransitionWorkflowJob(ctx, WorkflowJobTransitionInput{JobID: job.ID, To: workflows.JobDispatched})
	if err != nil {
		t.Fatalf("pending -> dispatched: %v", err)
	}
	if dispatched.State != workflows.JobDispatched || dispatched.FinishedAt != nil {
		t.Fatalf("dispatched = %+v", dispatched)
	}

	succeeded, err := store.TransitionWorkflowJob(ctx, WorkflowJobTransitionInput{JobID: job.ID, To: workflows.JobSucceeded})
	if err != nil {
		t.Fatalf("dispatched -> succeeded: %v", err)
	}
	if succeeded.FinishedAt == nil {
		t.Fatal("terminal job has no finished_at")
	}
	if _, err := store.TransitionWorkflowJob(ctx, WorkflowJobTransitionInput{JobID: job.ID, To: workflows.JobDispatched}); !isConflict(err) {
		t.Fatalf("terminal job error = %v, want conflict", err)
	}
}

func TestWorkflowJobRetryEnforcesAttemptBudget(t *testing.T) {
	store, ctx := workflowStateTestDB(t)
	instance := attachTestInstance(t, store, ctx)
	// First job has MaxAttempts 3.
	job := firstPendingJob(t, store, ctx, instance.ID)

	mustTransitionJob := func(to workflows.JobState) WorkflowJob {
		t.Helper()
		updated, err := store.TransitionWorkflowJob(ctx, WorkflowJobTransitionInput{JobID: job.ID, To: to})
		if err != nil {
			t.Fatalf("job -> %s: %v", to, err)
		}
		return updated
	}

	if _, err := store.TransitionWorkflowJob(ctx, WorkflowJobTransitionInput{JobID: job.ID, To: workflows.JobSucceeded}); !isConflict(err) {
		t.Fatalf("pending -> succeeded error = %v, want conflict", err)
	}

	mustTransitionJob(workflows.JobDispatched)
	retry := mustTransitionJob(workflows.JobFailed)
	if retry.Attempt != 1 || retry.FinishedAt == nil {
		t.Fatalf("failed job = %+v, want attempt 1 finished", retry)
	}
	requeued := mustTransitionJob(workflows.JobPending)
	if requeued.Attempt != 2 || requeued.FinishedAt != nil {
		t.Fatalf("requeued job = %+v, want attempt 2 unsealed", requeued)
	}

	mustTransitionJob(workflows.JobDispatched)
	mustTransitionJob(workflows.JobFailed)
	mustTransitionJob(workflows.JobPending) // attempt 3
	mustTransitionJob(workflows.JobDispatched)
	mustTransitionJob(workflows.JobFailed)

	if _, err := store.TransitionWorkflowJob(ctx, WorkflowJobTransitionInput{JobID: job.ID, To: workflows.JobPending}); !isConflict(err) {
		t.Fatalf("exhausted retry error = %v, want conflict", err)
	}
}

func TestWorkflowJobTransitionRejectsBadInput(t *testing.T) {
	store, ctx := workflowStateTestDB(t)
	instance := attachTestInstance(t, store, ctx)
	job := firstPendingJob(t, store, ctx, instance.ID)

	if _, err := store.TransitionWorkflowJob(ctx, WorkflowJobTransitionInput{JobID: job.ID, To: "running"}); !isInvalidInput(err) {
		t.Fatalf("unknown state error = %v, want InvalidInputError", err)
	}
	if _, err := store.TransitionWorkflowJob(ctx, WorkflowJobTransitionInput{JobID: job.ID, To: workflows.JobPending}); !isConflict(err) {
		t.Fatalf("self-edge error = %v, want conflict", err)
	}
	if _, err := store.TransitionWorkflowJob(ctx, WorkflowJobTransitionInput{JobID: 999999, To: workflows.JobDispatched}); !isNotFound(err) {
		t.Fatalf("missing job error = %v, want not found", err)
	}
}

func pauseAtGate(t *testing.T, store *PostgresStore, ctx context.Context, instance WorkflowInstance) {
	t.Helper()
	if _, err := store.TransitionWorkflowInstance(ctx, WorkflowInstanceTransitionInput{InstanceID: instance.ID, To: workflows.InstanceRunning}); err != nil {
		t.Fatalf("pending -> running: %v", err)
	}
	if _, err := store.TransitionWorkflowInstance(ctx, WorkflowInstanceTransitionInput{
		InstanceID: instance.ID,
		To:         workflows.InstanceWaitingForApproval,
		AtStep:     "approve-destroy",
	}); err != nil {
		t.Fatalf("running -> waiting_for_approval: %v", err)
	}
}

func TestRecordApprovalBindsGateAndIsIdempotent(t *testing.T) {
	store, ctx := workflowStateTestDB(t)
	instance := attachTestInstance(t, store, ctx)
	pauseAtGate(t, store, ctx, instance)

	approval, err := store.RecordApproval(ctx, RecordApprovalInput{
		InstanceID: instance.ID,
		GateID:     "approve-destroy",
		Approver:   manualTestActor(),
		Reason:     "replacement scheduled",
	})
	if err != nil {
		t.Fatalf("RecordApproval: %v", err)
	}
	if approval.ID == 0 || approval.WorkflowInstanceID != instance.ID || approval.GateID != "approve-destroy" {
		t.Fatalf("approval = %+v", approval)
	}
	if approval.Approver.Subject != "manual-test-operator" || approval.Approver.DisplayName != "Manual Test Operator" {
		t.Fatalf("approver = %+v, want the acting operator", approval.Approver)
	}
	if approval.Reason == nil || *approval.Reason != "replacement scheduled" {
		t.Fatalf("reason = %v, want snapshot", approval.Reason)
	}
	if approval.CreatedAt.IsZero() {
		t.Fatal("approval has no timestamp")
	}

	again, err := store.RecordApproval(ctx, RecordApprovalInput{
		InstanceID: instance.ID,
		GateID:     "approve-destroy",
		Approver:   manualTestActor(),
	})
	if err != nil {
		t.Fatalf("second RecordApproval: %v", err)
	}
	if again.ID != approval.ID {
		t.Fatalf("second approval = %d, want idempotent %d", again.ID, approval.ID)
	}
}

func TestRecordApprovalRejectsWrongGateAndNotWaiting(t *testing.T) {
	store, ctx := workflowStateTestDB(t)

	waiting := attachTestInstance(t, store, ctx)
	pauseAtGate(t, store, ctx, waiting)
	if _, err := store.RecordApproval(ctx, RecordApprovalInput{InstanceID: waiting.ID, GateID: "other-gate", Approver: manualTestActor()}); !isConflict(err) {
		t.Fatalf("wrong gate error = %v, want conflict", err)
	}

	pending := attachTestInstance(t, store, ctx)
	if _, err := store.RecordApproval(ctx, RecordApprovalInput{InstanceID: pending.ID, GateID: "approve-destroy", Approver: manualTestActor()}); !isConflict(err) {
		t.Fatalf("not waiting error = %v, want conflict", err)
	}

	if _, err := store.RecordApproval(ctx, RecordApprovalInput{InstanceID: 999999, GateID: "approve-destroy", Approver: manualTestActor()}); !isNotFound(err) {
		t.Fatalf("missing instance error = %v, want not found", err)
	}
	if _, err := store.RecordApproval(ctx, RecordApprovalInput{InstanceID: pending.ID, GateID: "", Approver: manualTestActor()}); !isInvalidInput(err) {
		t.Fatalf("empty gate error = %v, want InvalidInputError", err)
	}
	if _, err := store.RecordApproval(ctx, RecordApprovalInput{InstanceID: pending.ID, GateID: "approve-destroy", Approver: Actor{Subject: "", DisplayName: "X"}}); !isInvalidInput(err) {
		t.Fatalf("bad approver error = %v, want InvalidInputError", err)
	}
}

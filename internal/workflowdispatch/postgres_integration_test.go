package workflowdispatch

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/tonymontoya/ceph-atlas/internal/agent"
	"github.com/tonymontoya/ceph-atlas/internal/cases"
	"github.com/tonymontoya/ceph-atlas/internal/operations"
	"github.com/tonymontoya/ceph-atlas/internal/store"
	"github.com/tonymontoya/ceph-atlas/internal/workflows"
)

func testDatabaseURL(t *testing.T) string {
	t.Helper()
	databaseURL := os.Getenv("ATLAS_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set ATLAS_TEST_DATABASE_URL to run PostgreSQL integration test")
	}
	return databaseURL
}

type dispatchDB struct {
	*store.PostgresStore
	db *sql.DB
}

func dispatchTestDB(t *testing.T) (dispatchDB, context.Context) {
	t.Helper()
	ctx := context.Background()
	db, err := store.OpenPostgres(ctx, testDatabaseURL(t))
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	postgresStore := dispatchDB{PostgresStore: store.NewPostgres(db), db: db}
	dispatchCleanup(t, postgresStore.db)
	t.Cleanup(func() { dispatchCleanup(t, postgresStore.db) })
	return postgresStore, ctx
}

func dispatchCleanup(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx := context.Background()
	statements := []string{
		`DELETE FROM workflow_task_completions`,
		`DELETE FROM workflow_approvals`,
		`DELETE FROM workflow_jobs`,
		`DELETE FROM workflow_instances`,
		`DELETE FROM case_timeline_events WHERE case_id IN (SELECT id FROM cases WHERE title LIKE 'dispatch-test%')`,
		`DELETE FROM cases WHERE title LIKE 'dispatch-test%'`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("cleanup %q: %v", statement, err)
		}
	}
}

// attachAndApprove drives the store through attach, gate parking, and
// approval exactly as the API does, leaving a running instance.
func attachAndApprove(t *testing.T, postgresStore *store.PostgresStore, ctx context.Context, actor store.Actor) store.WorkflowInstance {
	t.Helper()
	target, err := postgresStore.CreateManualCase(ctx, store.ManualCaseInput{
		Title:       "dispatch-test replace target",
		Summary:     "dispatch-test summary",
		Severity:    string(cases.CaseSeverityMedium),
		ClusterFSID: "00000000-0000-4000-8000-000000000101",
		Actor:       actor,
	})
	if err != nil {
		t.Fatalf("CreateManualCase: %v", err)
	}
	instance, err := postgresStore.CreateWorkflowInstance(ctx, store.CreateWorkflowInstanceInput{
		CaseID:            target.ID,
		DefinitionID:      "replace-osd",
		DefinitionVersion: 1,
		Jobs: []store.WorkflowJobInput{
			{StepID: "collect-evidence", OperationType: "CollectHostEvidence", MaxAttempts: 3},
			{StepID: "destroy-osd", OperationType: "DestroyOSD", MaxAttempts: 1},
			{StepID: "verify-osd", OperationType: "VerifyOSD", MaxAttempts: 3},
		},
		Actor: actor,
	})
	if err != nil {
		t.Fatalf("CreateWorkflowInstance: %v", err)
	}
	for _, transition := range []store.WorkflowInstanceTransitionInput{
		{InstanceID: instance.ID, To: workflows.InstanceRunning},
		{InstanceID: instance.ID, To: workflows.InstanceWaitingForApproval, AtStep: "approve-destroy"},
	} {
		if instance, err = postgresStore.TransitionWorkflowInstance(ctx, transition); err != nil {
			t.Fatalf("advance to gate: %v", err)
		}
	}
	if _, err := postgresStore.RecordApproval(ctx, store.RecordApprovalInput{
		InstanceID: instance.ID, GateID: "approve-destroy", Approver: actor,
	}); err != nil {
		t.Fatalf("RecordApproval: %v", err)
	}
	if instance, err = postgresStore.TransitionWorkflowInstance(ctx, store.WorkflowInstanceTransitionInput{
		InstanceID: instance.ID, To: workflows.InstanceRunning,
	}); err != nil {
		t.Fatalf("resume past gate: %v", err)
	}
	return instance
}

func testDispatcher(t *testing.T, postgresStore *store.PostgresStore) *Dispatcher {
	t.Helper()
	return testDispatcherWithScenario(t, postgresStore, "")
}

// testDispatcherWithScenario builds a dispatcher over the real fake
// Atlas Agent scripted by a failure scenario, so the Postgres-backed
// retry behavior runs through the same agent the app wires.
func testDispatcherWithScenario(t *testing.T, postgresStore *store.PostgresStore, scenario string) *Dispatcher {
	t.Helper()
	ops, err := operations.DefaultRegistry()
	if err != nil {
		t.Fatalf("operations registry: %v", err)
	}
	defs, err := workflows.DefaultRegistry(ops)
	if err != nil {
		t.Fatalf("workflow registry: %v", err)
	}
	fakeAgent, err := agent.NewFakeWithScenario(ops, scenario)
	if err != nil {
		t.Fatalf("fake agent scenario %q: %v", scenario, err)
	}
	return New(postgresStore, defs, fakeAgent)
}

// resumePastTask performs the Operator's task completion and resume
// against the durable store, exactly as the API resume endpoint does.
func resumePastTask(t *testing.T, postgresStore *store.PostgresStore, ctx context.Context, instance store.WorkflowInstance, actor store.Actor) store.WorkflowInstance {
	t.Helper()
	if _, err := postgresStore.RecordTaskCompletion(ctx, store.RecordTaskCompletionInput{
		InstanceID: instance.ID, TaskID: "replace-device", Operator: actor, Note: "device swapped",
	}); err != nil {
		t.Fatalf("RecordTaskCompletion: %v", err)
	}
	resumed, err := postgresStore.TransitionWorkflowInstance(ctx, store.WorkflowInstanceTransitionInput{
		InstanceID: instance.ID, To: workflows.InstanceRunning,
		Actor: &actor,
	})
	if err != nil {
		t.Fatalf("resume past task: %v", err)
	}
	return resumed
}

func TestRunPausesAtTaskAndResumesToSucceededAgainstPostgres(t *testing.T) {
	postgresStore, ctx := dispatchTestDB(t)
	actor := store.Actor{Subject: "dispatch-operator", DisplayName: "Dispatch Operator"}
	approved := attachAndApprove(t, postgresStore.PostgresStore, ctx, actor)
	dispatcher := testDispatcher(t, postgresStore.PostgresStore)

	paused, err := dispatcher.Run(ctx, approved.ID)
	if err != nil {
		t.Fatalf("Run to task: %v", err)
	}
	if paused.State != workflows.InstanceWaitingForOperator || paused.CurrentStep == nil || *paused.CurrentStep != "replace-device" {
		t.Fatalf("instance = %+v, want waiting_for_operator at replace-device", paused)
	}
	jobs, err := postgresStore.ListWorkflowJobs(ctx, approved.ID)
	if err != nil {
		t.Fatalf("ListWorkflowJobs: %v", err)
	}
	for _, job := range jobs {
		want := workflows.JobSucceeded
		if job.StepID == "verify-osd" {
			want = workflows.JobPending
		}
		if job.State != want {
			t.Fatalf("job %s = %s, want %s at the pause", job.StepID, job.State, want)
		}
	}

	resumed := resumePastTask(t, postgresStore.PostgresStore, ctx, paused, actor)
	finished, err := dispatcher.Run(ctx, resumed.ID)
	if err != nil {
		t.Fatalf("Run after resume: %v", err)
	}
	if finished.State != workflows.InstanceSucceeded || finished.FinishedAt == nil {
		t.Fatalf("instance = %+v, want terminal succeeded", finished)
	}

	stored, err := postgresStore.ListWorkflowJobs(ctx, approved.ID)
	if err != nil {
		t.Fatalf("ListWorkflowJobs: %v", err)
	}
	if len(stored) != 3 {
		t.Fatalf("jobs = %d, want 3", len(stored))
	}
	wantSteps := []string{"collect-evidence", "destroy-osd", "verify-osd"}
	for i, job := range stored {
		if job.StepID != wantSteps[i] || job.State != workflows.JobSucceeded || job.FinishedAt == nil {
			t.Fatalf("job %d = %+v, want %s succeeded", i, job, wantSteps[i])
		}
	}

	timeline, err := postgresStore.ListCaseTimeline(ctx, approved.CaseID)
	if err != nil {
		t.Fatalf("ListCaseTimeline: %v", err)
	}
	stateChanges := 0
	var last *cases.TimelineEvent
	var operatorResume, systemPause bool
	for i := range timeline {
		if timeline[i].Type != cases.TimelineEventWorkflowStateChanged {
			continue
		}
		stateChanges++
		last = &timeline[i]
		switch {
		case timeline[i].Payload["newState"] == "waiting_for_operator" && timeline[i].Actor.Type == cases.TimelineActorSystem:
			systemPause = true
		case timeline[i].Payload["previousState"] == "waiting_for_operator" && timeline[i].Actor.Type == cases.TimelineActorUser:
			operatorResume = true
		}
	}
	// running, waiting_for_approval, running (gate resume),
	// waiting_for_operator, running (task resume), succeeded.
	if stateChanges != 6 {
		t.Fatalf("workflow_state_changed events = %d, want 6", stateChanges)
	}
	if !systemPause || !operatorResume {
		t.Fatalf("pause=%v resume=%v, want the Atlas task pause and the Operator resume on the timeline", systemPause, operatorResume)
	}
	if last == nil || last.Payload["newState"] != "succeeded" {
		t.Fatalf("last state change = %+v, want succeeded", last)
	}
	if last.Actor.Type != cases.TimelineActorSystem || last.Actor.DisplayName != "Atlas" {
		t.Fatalf("terminal actor = %+v, want the Atlas system actor", last.Actor)
	}
}

// The dispatch-fails-once scenario drives a transient failure: the retry
// policy re-queues the Job, the second attempt succeeds, and the
// instance still reaches waiting_for_operator and then terminal
// succeeded.
func TestRunRetriesTransientFailureAgainstPostgres(t *testing.T) {
	postgresStore, ctx := dispatchTestDB(t)
	actor := store.Actor{Subject: "dispatch-operator", DisplayName: "Dispatch Operator"}
	approved := attachAndApprove(t, postgresStore.PostgresStore, ctx, actor)
	dispatcher := testDispatcherWithScenario(t, postgresStore.PostgresStore, agent.ScenarioDispatchFailsOnce)

	paused, err := dispatcher.Run(ctx, approved.ID)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if paused.State != workflows.InstanceWaitingForOperator {
		t.Fatalf("instance state = %s, want paused at the task after retry recovery", paused.State)
	}
	jobs, err := postgresStore.ListWorkflowJobs(ctx, approved.ID)
	if err != nil {
		t.Fatalf("ListWorkflowJobs: %v", err)
	}
	if jobs[0].State != workflows.JobSucceeded || jobs[0].Attempt != 2 {
		t.Fatalf("collect-evidence = %+v, want succeeded on attempt 2", jobs[0])
	}

	resumed := resumePastTask(t, postgresStore.PostgresStore, ctx, paused, actor)
	finished, err := dispatcher.Run(ctx, resumed.ID)
	if err != nil {
		t.Fatalf("Run after resume: %v", err)
	}
	if finished.State != workflows.InstanceSucceeded {
		t.Fatalf("instance state = %s, want succeeded", finished.State)
	}
}

// The job-failure scenario exhausts the retry budget: every allowed
// attempt fails, the Job rests failed, and the instance fails
// terminally without dispatching later Jobs.
func TestRunFailsInstanceWhenRetriesExhaustAgainstPostgres(t *testing.T) {
	postgresStore, ctx := dispatchTestDB(t)
	actor := store.Actor{Subject: "dispatch-operator", DisplayName: "Dispatch Operator"}
	approved := attachAndApprove(t, postgresStore.PostgresStore, ctx, actor)
	dispatcher := testDispatcherWithScenario(t, postgresStore.PostgresStore, agent.ScenarioJobFailure)

	finished, err := dispatcher.Run(ctx, approved.ID)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if finished.State != workflows.InstanceFailed || finished.FinishedAt == nil {
		t.Fatalf("instance = %+v, want terminal failed", finished)
	}

	jobs, err := postgresStore.ListWorkflowJobs(ctx, approved.ID)
	if err != nil {
		t.Fatalf("ListWorkflowJobs: %v", err)
	}
	if jobs[0].State != workflows.JobFailed || jobs[0].Attempt != 3 || jobs[0].FinishedAt == nil {
		t.Fatalf("collect-evidence = %+v, want failed after the 3-attempt budget", jobs[0])
	}
	if jobs[1].State != workflows.JobPending || jobs[2].State != workflows.JobPending {
		t.Fatalf("later jobs = %+v, want untouched pending", jobs)
	}

	// A terminal instance never dispatches again.
	again, err := dispatcher.Run(ctx, approved.ID)
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if again.State != workflows.InstanceFailed || !again.UpdatedAt.Equal(finished.UpdatedAt) {
		t.Fatalf("second run = %+v, want the untouched failed instance", again)
	}
}

// Restart resilience: the durable rows a killed process leaves behind —
// collect-evidence succeeded, destroy-osd dispatched in flight, verify
// pending — are recovered by a fresh dispatcher: the succeeded Job is
// never re-executed, the in-flight Job is re-dispatched under its
// original idempotency key, and the instance still reaches its pause
// and then terminal succeeded.
func TestRunResumesFromDurableStateAfterRestart(t *testing.T) {
	postgresStore, ctx := dispatchTestDB(t)
	actor := store.Actor{Subject: "dispatch-operator", DisplayName: "Dispatch Operator"}
	approved := attachAndApprove(t, postgresStore.PostgresStore, ctx, actor)

	// Durable state a killed process leaves mid-loop: the evidence Job
	// completed, the destroy Job was marked dispatched but its outcome
	// was never recorded.
	jobs, err := postgresStore.ListWorkflowJobs(ctx, approved.ID)
	if err != nil {
		t.Fatalf("ListWorkflowJobs: %v", err)
	}
	mustTransitionJob := func(jobID int64, to workflows.JobState) store.WorkflowJob {
		t.Helper()
		updated, err := postgresStore.TransitionWorkflowJob(ctx, store.WorkflowJobTransitionInput{JobID: jobID, To: to})
		if err != nil {
			t.Fatalf("job %d -> %s: %v", jobID, to, err)
		}
		return updated
	}
	mustTransitionJob(jobs[0].ID, workflows.JobDispatched)
	mustTransitionJob(jobs[0].ID, workflows.JobSucceeded)
	mustTransitionJob(jobs[1].ID, workflows.JobDispatched)

	// A fresh process builds a fresh dispatcher and fake agent over the
	// same durable state.
	ops, err := operations.DefaultRegistry()
	if err != nil {
		t.Fatalf("operations registry: %v", err)
	}
	defs, err := workflows.DefaultRegistry(ops)
	if err != nil {
		t.Fatalf("workflow registry: %v", err)
	}
	freshAgent, err := agent.NewFakeWithScenario(ops, "")
	if err != nil {
		t.Fatalf("fake agent: %v", err)
	}
	counting := &countingAdapter{inner: freshAgent}
	fresh := New(postgresStore.PostgresStore, defs, counting)

	paused, err := fresh.Run(ctx, approved.ID)
	if err != nil {
		t.Fatalf("Run after restart: %v", err)
	}
	if paused.State != workflows.InstanceWaitingForOperator {
		t.Fatalf("instance state = %s, want paused at the task", paused.State)
	}
	if calls := counting.callsFor("CollectHostEvidence"); calls != 0 {
		t.Fatalf("collect-evidence dispatches after restart = %d, want 0 (completed Jobs are not re-executed)", calls)
	}
	if calls := counting.callsFor("DestroyOSD"); calls != 1 {
		t.Fatalf("destroy-osd dispatches after restart = %d, want 1", calls)
	}
	if key := counting.keys["DestroyOSD"]; key != fmt.Sprintf("instance-%d-job-%d-attempt-1", approved.ID, jobs[1].ID) {
		t.Fatalf("re-dispatch key = %q, want the original attempt's key", key)
	}

	resumed := resumePastTask(t, postgresStore.PostgresStore, ctx, paused, actor)
	finished, err := fresh.Run(ctx, resumed.ID)
	if err != nil {
		t.Fatalf("Run after resume: %v", err)
	}
	if finished.State != workflows.InstanceSucceeded {
		t.Fatalf("instance state = %s, want succeeded", finished.State)
	}
	if calls := counting.callsFor("VerifyOSD"); calls != 1 {
		t.Fatalf("verify-osd dispatches = %d, want 1", calls)
	}
}

// Duplicate dispatch does not repeat work: the agent remembers the
// outcome per idempotency key, so re-dispatching a Job whose outcome
// Atlas never recorded (the restart case) replays the remembered
// outcome instead of executing again. The agent survives the Atlas-side
// crash the way a real out-of-process agent would (ADR-0018).
func TestDuplicateDispatchDoesNotRepeatWorkAgainstPostgres(t *testing.T) {
	postgresStore, ctx := dispatchTestDB(t)
	actor := store.Actor{Subject: "dispatch-operator", DisplayName: "Dispatch Operator"}
	approved := attachAndApprove(t, postgresStore.PostgresStore, ctx, actor)

	ops, err := operations.DefaultRegistry()
	if err != nil {
		t.Fatalf("operations registry: %v", err)
	}
	defs, err := workflows.DefaultRegistry(ops)
	if err != nil {
		t.Fatalf("workflow registry: %v", err)
	}
	survivingAgent, err := agent.NewFakeWithScenario(ops, "")
	if err != nil {
		t.Fatalf("fake agent: %v", err)
	}

	// Durable state a killed Atlas process leaves behind:
	// collect-evidence completed, destroy-osd dispatched — and the agent
	// already executed that dispatch under its idempotency key, but the
	// outcome was never recorded.
	jobs, err := postgresStore.ListWorkflowJobs(ctx, approved.ID)
	if err != nil {
		t.Fatalf("ListWorkflowJobs: %v", err)
	}
	mustTransitionJob := func(jobID int64, to workflows.JobState) {
		t.Helper()
		if _, err := postgresStore.TransitionWorkflowJob(ctx, store.WorkflowJobTransitionInput{JobID: jobID, To: to}); err != nil {
			t.Fatalf("job %d -> %s: %v", jobID, to, err)
		}
	}
	mustTransitionJob(jobs[0].ID, workflows.JobDispatched)
	mustTransitionJob(jobs[0].ID, workflows.JobSucceeded)
	mustTransitionJob(jobs[1].ID, workflows.JobDispatched)
	parameters, err := json.Marshal(operations.DestroyOSD{ClusterFSID: "00000000-0000-4000-8000-000000000101", OSDID: 0})
	if err != nil {
		t.Fatalf("marshal parameters: %v", err)
	}
	preCrash, err := json.Marshal(operations.RequestEnvelope{
		WorkflowInstanceID: approved.ID,
		JobID:              jobs[1].ID,
		Actor:              operations.Actor{Subject: actor.Subject, DisplayName: actor.DisplayName},
		IdempotencyKey:     fmt.Sprintf("instance-%d-job-%d-attempt-1", approved.ID, jobs[1].ID),
		AuditCorrelationID: fmt.Sprintf("workflow-%d-job-%d-attempt-1", approved.ID, jobs[1].ID),
		OperationType:      jobs[1].OperationType,
		Parameters:         parameters,
	})
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	if _, err := survivingAgent.Dispatch(ctx, preCrash); err != nil {
		t.Fatalf("pre-crash agent execution: %v", err)
	}
	executed := survivingAgent.ExecutionCount()
	if executed != 1 {
		t.Fatalf("agent executions = %d, want 1 before the duplicate dispatch", executed)
	}

	// A fresh dispatcher re-dispatches the in-flight Job under the same
	// key; the agent replays the remembered outcome without executing.
	recovered := New(postgresStore.PostgresStore, defs, survivingAgent)
	paused, err := recovered.Run(ctx, approved.ID)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if paused.State != workflows.InstanceWaitingForOperator {
		t.Fatalf("instance state = %s, want paused at the task", paused.State)
	}
	if got := survivingAgent.ExecutionCount(); got != executed {
		t.Fatalf("agent executions = %d after the duplicate dispatch, want %d (no repeat)", got, executed)
	}
	stored, err := postgresStore.ListWorkflowJobs(ctx, approved.ID)
	if err != nil {
		t.Fatalf("ListWorkflowJobs: %v", err)
	}
	if stored[1].State != workflows.JobSucceeded {
		t.Fatalf("destroy-osd = %s, want re-recorded succeeded from the replayed outcome", stored[1].State)
	}
}

// countingAdapter wraps an AgentAdapter and records how often each
// operation type was dispatched and under which idempotency key.
type countingAdapter struct {
	inner agent.AgentAdapter
	calls map[string]int
	keys  map[string]string
}

func (c *countingAdapter) Dispatch(ctx context.Context, envelope []byte) (agent.Result, error) {
	var decoded operations.RequestEnvelope
	if err := json.Unmarshal(envelope, &decoded); err != nil {
		return agent.Result{}, err
	}
	if c.calls == nil {
		c.calls = make(map[string]int)
		c.keys = make(map[string]string)
	}
	c.calls[decoded.OperationType]++
	c.keys[decoded.OperationType] = decoded.IdempotencyKey
	return c.inner.Dispatch(ctx, envelope)
}

func (c *countingAdapter) callsFor(operationType string) int {
	return c.calls[operationType]
}

func TestRunIsIdempotentForTerminalInstances(t *testing.T) {
	postgresStore, ctx := dispatchTestDB(t)
	actor := store.Actor{Subject: "dispatch-operator", DisplayName: "Dispatch Operator"}
	approved := attachAndApprove(t, postgresStore.PostgresStore, ctx, actor)
	dispatcher := testDispatcher(t, postgresStore.PostgresStore)

	paused, err := dispatcher.Run(ctx, approved.ID)
	if err != nil {
		t.Fatalf("first Run: %v", err)
	}
	if paused.State != workflows.InstanceWaitingForOperator {
		t.Fatalf("first run state = %s, want paused at the task", paused.State)
	}
	resumed := resumePastTask(t, postgresStore.PostgresStore, ctx, paused, actor)
	first, err := dispatcher.Run(ctx, resumed.ID)
	if err != nil {
		t.Fatalf("Run after resume: %v", err)
	}
	if first.State != workflows.InstanceSucceeded {
		t.Fatalf("run state = %s, want succeeded", first.State)
	}

	again, err := dispatcher.Run(ctx, approved.ID)
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if again.State != workflows.InstanceSucceeded {
		t.Fatalf("second run = %+v, want untouched succeeded", again)
	}
	jobs, err := postgresStore.ListWorkflowJobs(ctx, approved.ID)
	if err != nil {
		t.Fatalf("ListWorkflowJobs: %v", err)
	}
	for _, job := range jobs {
		if job.UpdatedAt.After(first.UpdatedAt) {
			t.Fatalf("job %s was touched after terminal state", job.StepID)
		}
	}
}

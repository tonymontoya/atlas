package workflowdispatch

import (
	"context"
	"database/sql"
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
	ops, err := operations.DefaultRegistry()
	if err != nil {
		t.Fatalf("operations registry: %v", err)
	}
	defs, err := workflows.DefaultRegistry(ops)
	if err != nil {
		t.Fatalf("workflow registry: %v", err)
	}
	return New(postgresStore, defs, agent.NewFake(ops))
}

func TestRunDrivesApprovedInstanceToSucceededAgainstPostgres(t *testing.T) {
	postgresStore, ctx := dispatchTestDB(t)
	actor := store.Actor{Subject: "dispatch-operator", DisplayName: "Dispatch Operator"}
	approved := attachAndApprove(t, postgresStore.PostgresStore, ctx, actor)
	dispatcher := testDispatcher(t, postgresStore.PostgresStore)

	// replace-osd also carries verify-osd after the human Task; the fake
	// loop passes Tasks through until the resilience slice adds the
	// waiting_for_operator pause.
	finished, err := dispatcher.Run(ctx, approved.ID)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if finished.State != workflows.InstanceSucceeded {
		t.Fatalf("instance state = %s, want succeeded", finished.State)
	}
	if finished.FinishedAt == nil {
		t.Fatal("succeeded instance has no finished_at")
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
	for i := range timeline {
		if timeline[i].Type == cases.TimelineEventWorkflowStateChanged {
			stateChanges++
			last = &timeline[i]
		}
	}
	// running, waiting_for_approval, running (resume), succeeded.
	if stateChanges != 4 {
		t.Fatalf("workflow_state_changed events = %d, want 4", stateChanges)
	}
	if last == nil || last.Payload["newState"] != "succeeded" {
		t.Fatalf("last state change = %+v, want succeeded", last)
	}
	if last.Actor.Type != cases.TimelineActorSystem || last.Actor.DisplayName != "Atlas" {
		t.Fatalf("terminal actor = %+v, want the Atlas system actor", last.Actor)
	}
}

func TestRunIsIdempotentForTerminalInstances(t *testing.T) {
	postgresStore, ctx := dispatchTestDB(t)
	actor := store.Actor{Subject: "dispatch-operator", DisplayName: "Dispatch Operator"}
	approved := attachAndApprove(t, postgresStore.PostgresStore, ctx, actor)
	dispatcher := testDispatcher(t, postgresStore.PostgresStore)

	first, err := dispatcher.Run(ctx, approved.ID)
	if err != nil {
		t.Fatalf("first Run: %v", err)
	}
	if first.State != workflows.InstanceSucceeded {
		t.Fatalf("first run state = %s, want succeeded", first.State)
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

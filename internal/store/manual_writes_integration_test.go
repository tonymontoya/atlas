package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/tonymontoya/ceph-atlas/internal/cases"
	"github.com/tonymontoya/ceph-atlas/internal/providers"
	"github.com/tonymontoya/ceph-atlas/internal/testdb"
)

func manualWritesTestDB(t *testing.T) (*PostgresStore, context.Context) {
	t.Helper()
	db, _ := testdb.Open(t)

	cleanupManualWriteRows(t, db)
	t.Cleanup(func() { cleanupManualWriteRows(t, db) })
	return NewPostgres(db), context.Background()
}

func cleanupManualWriteRows(t *testing.T, db *sql.DB) {
	t.Helper()
	testdb.DeleteCases(t, db, "title LIKE 'manual-test%'")
}

func manualTestActor() Actor {
	return Actor{Subject: "manual-test-operator", DisplayName: "Manual Test Operator"}
}

func TestCreateManualCaseWritesCaseAndDetectionEvent(t *testing.T) {
	store, ctx := manualWritesTestDB(t)

	created, err := store.CreateManualCase(ctx, ManualCaseInput{
		Title:    "manual-test capacity review",
		Summary:  "manual-test summary",
		Severity: string(cases.CaseSeverityMedium),
		Actor:    manualTestActor(),
	})
	if err != nil {
		t.Fatalf("CreateManualCase: %v", err)
	}
	if created.ID == 0 || created.Status != cases.CaseStatusDetected || created.Source != cases.CaseSourceManual {
		t.Fatalf("created case = %+v, want detected manual case", created)
	}
	if created.Severity != cases.CaseSeverityMedium {
		t.Fatalf("severity = %q, want medium", created.Severity)
	}

	timeline, err := store.ListCaseTimeline(ctx, created.ID)
	if err != nil {
		t.Fatalf("list timeline: %v", err)
	}
	if len(timeline) != 1 || timeline[0].Type != cases.TimelineEventCaseDetected {
		t.Fatalf("timeline = %+v, want one case_detected event", timeline)
	}
	event := timeline[0]
	if event.Actor.Type != cases.TimelineActorUser || event.Actor.ID != "manual-test-operator" || event.Actor.DisplayName != "Manual Test Operator" {
		t.Fatalf("actor = %+v, want the acting operator", event.Actor)
	}
	if event.Payload["source"] != "manual" {
		t.Fatalf("payload = %+v, want source manual", event.Payload)
	}
}

func TestCreateManualCaseRejectsInvalidInput(t *testing.T) {
	store, ctx := manualWritesTestDB(t)

	inputs := []struct {
		name  string
		input ManualCaseInput
	}{
		{"empty title", ManualCaseInput{Title: "", Summary: "s", Severity: "medium", Actor: manualTestActor()}},
		{"empty summary", ManualCaseInput{Title: "t", Summary: "", Severity: "medium", Actor: manualTestActor()}},
		{"unknown severity", ManualCaseInput{Title: "t", Summary: "s", Severity: "severe", Actor: manualTestActor()}},
		{"invalid cluster fsid", ManualCaseInput{Title: "t", Summary: "s", Severity: "medium", ClusterFSID: "not-a-uuid", Actor: manualTestActor()}},
		{"actor without subject", ManualCaseInput{Title: "t", Summary: "s", Severity: "medium", Actor: Actor{Subject: "", DisplayName: "Name"}}},
		{"actor without display name", ManualCaseInput{Title: "t", Summary: "s", Severity: "medium", Actor: Actor{Subject: "subj", DisplayName: ""}}},
	}
	for _, input := range inputs {
		t.Run(input.name, func(t *testing.T) {
			_, err := store.CreateManualCase(ctx, input.input)
			if !isInvalidInput(err) {
				t.Fatalf("error = %v, want InvalidInputError", err)
			}
		})
	}
}

func TestCaseWritesClassifyInvalidInput(t *testing.T) {
	store, ctx := manualWritesTestDB(t)

	if _, err := store.TransitionCase(ctx, CaseTransitionInput{CaseID: 1, To: cases.CaseStatus("bogus"), Actor: manualTestActor()}); !isInvalidInput(err) {
		t.Fatalf("transition error = %v, want InvalidInputError", err)
	}
	if _, err := store.AssignCase(ctx, CaseAssignmentInput{CaseID: 1, Assignee: "subj", AssigneeDisplayName: "", Actor: manualTestActor()}); !isInvalidInput(err) {
		t.Fatalf("assign error = %v, want InvalidInputError", err)
	}
	if _, err := store.AddCaseNote(ctx, CaseNoteInput{CaseID: 1, Body: "", Actor: manualTestActor()}); !isInvalidInput(err) {
		t.Fatalf("note error = %v, want InvalidInputError", err)
	}
}

func TestTransitionCaseMovesThroughLifecycle(t *testing.T) {
	store, ctx := manualWritesTestDB(t)
	created, err := store.CreateManualCase(ctx, ManualCaseInput{
		Title: "manual-test lifecycle", Summary: "manual-test summary", Severity: "low", Actor: manualTestActor(),
	})
	if err != nil {
		t.Fatalf("CreateManualCase: %v", err)
	}

	triaged, err := store.TransitionCase(ctx, CaseTransitionInput{CaseID: created.ID, To: cases.CaseStatusTriaged, Actor: manualTestActor()})
	if err != nil {
		t.Fatalf("transition to triaged: %v", err)
	}
	if triaged.Status != cases.CaseStatusTriaged || !triaged.UpdatedAt.After(created.CreatedAt) {
		t.Fatalf("triaged case = %+v, want updated triaged case", triaged)
	}

	closed, err := store.TransitionCase(ctx, CaseTransitionInput{CaseID: created.ID, To: cases.CaseStatusClosed, Actor: manualTestActor()})
	if err != nil {
		t.Fatalf("transition to closed: %v", err)
	}
	if closed.Status != cases.CaseStatusClosed || closed.ClosedAt == nil {
		t.Fatalf("closed case = %+v, want closed_at set", closed)
	}

	timeline, err := store.ListCaseTimeline(ctx, created.ID)
	if err != nil {
		t.Fatalf("list timeline: %v", err)
	}
	if len(timeline) != 3 {
		t.Fatalf("timeline length = %d, want 3; timeline=%+v", len(timeline), timeline)
	}
	if timeline[1].Type != cases.TimelineEventCaseTriaged {
		t.Fatalf("second event = %q, want case_triaged", timeline[1].Type)
	}
	if timeline[2].Type != cases.TimelineEventCaseStatusChanged {
		t.Fatalf("third event = %q, want case_status_changed", timeline[2].Type)
	}
	if timeline[1].Payload["previousStatus"] != "detected" || timeline[1].Payload["newStatus"] != "triaged" {
		t.Fatalf("triaged payload = %+v", timeline[1].Payload)
	}
	if timeline[1].Actor.ID != "manual-test-operator" {
		t.Fatalf("actor = %+v, want acting operator", timeline[1].Actor)
	}
}

func TestTransitionCaseRejectsClosedReopen(t *testing.T) {
	store, ctx := manualWritesTestDB(t)
	created, err := store.CreateManualCase(ctx, ManualCaseInput{
		Title: "manual-test reopen", Summary: "manual-test summary", Severity: "low", Actor: manualTestActor(),
	})
	if err != nil {
		t.Fatalf("CreateManualCase: %v", err)
	}
	if _, err := store.TransitionCase(ctx, CaseTransitionInput{CaseID: created.ID, To: cases.CaseStatusClosed, Actor: manualTestActor()}); err != nil {
		t.Fatalf("close: %v", err)
	}

	_, err = store.TransitionCase(ctx, CaseTransitionInput{CaseID: created.ID, To: cases.CaseStatusTriaged, Actor: manualTestActor()})
	if err == nil {
		t.Fatal("accepted reopening a closed case")
	}
	var conflictErr providers.ProviderError
	if !asProviderError(err, &conflictErr) || conflictErr.Class != providers.ErrorConflict {
		t.Fatalf("error = %v, want Conflict class", err)
	}
}

func TestTransitionCaseMissingCaseIsNotFound(t *testing.T) {
	store, ctx := manualWritesTestDB(t)

	_, err := store.TransitionCase(ctx, CaseTransitionInput{CaseID: 999999, To: cases.CaseStatusTriaged, Actor: manualTestActor()})
	if !isNotFound(err) {
		t.Fatalf("error = %v, want not found", err)
	}
}

func TestAssignCaseRecordsAssignmentEvent(t *testing.T) {
	store, ctx := manualWritesTestDB(t)
	created, err := store.CreateManualCase(ctx, ManualCaseInput{
		Title: "manual-test assignment", Summary: "manual-test summary", Severity: "low", Actor: manualTestActor(),
	})
	if err != nil {
		t.Fatalf("CreateManualCase: %v", err)
	}

	assigned, err := store.AssignCase(ctx, CaseAssignmentInput{
		CaseID: created.ID, Assignee: "assignee-subject", AssigneeDisplayName: "Assignee Name", Actor: manualTestActor(),
	})
	if err != nil {
		t.Fatalf("AssignCase: %v", err)
	}
	if assigned.Assignee != "assignee-subject" || assigned.AssigneeDisplayName != "Assignee Name" {
		t.Fatalf("assigned case = %+v, want assignee snapshot", assigned)
	}

	unassigned, err := store.AssignCase(ctx, CaseAssignmentInput{CaseID: created.ID, Actor: manualTestActor()})
	if err != nil {
		t.Fatalf("unassign: %v", err)
	}
	if unassigned.Assignee != "" || unassigned.AssigneeDisplayName != "" {
		t.Fatalf("unassigned case = %+v, want cleared assignee", unassigned)
	}

	timeline, err := store.ListCaseTimeline(ctx, created.ID)
	if err != nil {
		t.Fatalf("list timeline: %v", err)
	}
	if len(timeline) != 3 {
		t.Fatalf("timeline length = %d, want 3; timeline=%+v", len(timeline), timeline)
	}
	if timeline[1].Type != cases.TimelineEventCaseAssigned || timeline[2].Type != cases.TimelineEventCaseAssigned {
		t.Fatalf("assignment events = %q, %q; want case_assigned twice", timeline[1].Type, timeline[2].Type)
	}
	if timeline[1].Payload["previousAssignee"] != nil || timeline[1].Payload["newAssignee"] != "assignee-subject" {
		t.Fatalf("assign payload = %+v", timeline[1].Payload)
	}
	if timeline[2].Payload["previousAssignee"] != "assignee-subject" || timeline[2].Payload["newAssignee"] != nil {
		t.Fatalf("unassign payload = %+v", timeline[2].Payload)
	}
}

func TestAssignCaseSameAssigneeIsIdempotent(t *testing.T) {
	store, ctx := manualWritesTestDB(t)
	created, err := store.CreateManualCase(ctx, ManualCaseInput{
		Title: "manual-test idempotent", Summary: "manual-test summary", Severity: "low", Actor: manualTestActor(),
	})
	if err != nil {
		t.Fatalf("CreateManualCase: %v", err)
	}
	input := CaseAssignmentInput{CaseID: created.ID, Assignee: "same-subject", AssigneeDisplayName: "Same", Actor: manualTestActor()}
	if _, err := store.AssignCase(ctx, input); err != nil {
		t.Fatalf("first assign: %v", err)
	}
	if _, err := store.AssignCase(ctx, input); err != nil {
		t.Fatalf("second assign: %v", err)
	}

	timeline, err := store.ListCaseTimeline(ctx, created.ID)
	if err != nil {
		t.Fatalf("list timeline: %v", err)
	}
	assignedEvents := 0
	for _, event := range timeline {
		if event.Type == cases.TimelineEventCaseAssigned {
			assignedEvents++
		}
	}
	if assignedEvents != 1 {
		t.Fatalf("assignment event count = %d, want 1 for identical reassignment", assignedEvents)
	}
}

func TestAddCaseNoteWritesNoteAndEvent(t *testing.T) {
	store, ctx := manualWritesTestDB(t)
	created, err := store.CreateManualCase(ctx, ManualCaseInput{
		Title: "manual-test notes", Summary: "manual-test summary", Severity: "low", Actor: manualTestActor(),
	})
	if err != nil {
		t.Fatalf("CreateManualCase: %v", err)
	}

	note, err := store.AddCaseNote(ctx, CaseNoteInput{CaseID: created.ID, Body: "manual-test investigated; replacement scheduled.", Actor: manualTestActor()})
	if err != nil {
		t.Fatalf("AddCaseNote: %v", err)
	}
	if note.ID == 0 || note.CaseID != created.ID || note.AuthorID != "manual-test-operator" || note.AuthorDisplayName != "Manual Test Operator" {
		t.Fatalf("note = %+v, want persisted author context", note)
	}

	notes, err := store.ListCaseNotes(ctx, created.ID)
	if err != nil {
		t.Fatalf("ListCaseNotes: %v", err)
	}
	if len(notes) != 1 || notes[0].Body != "manual-test investigated; replacement scheduled." {
		t.Fatalf("notes = %+v, want the created note", notes)
	}

	timeline, err := store.ListCaseTimeline(ctx, created.ID)
	if err != nil {
		t.Fatalf("list timeline: %v", err)
	}
	if len(timeline) != 2 || timeline[1].Type != cases.TimelineEventCaseNoteAdded {
		t.Fatalf("timeline = %+v, want a case_note_added event", timeline)
	}
	if timeline[1].Payload["noteId"] != float64(note.ID) {
		t.Fatalf("payload = %+v, want noteId %d", timeline[1].Payload, note.ID)
	}
}

func TestListCasesSurfacesAssignee(t *testing.T) {
	store, ctx := manualWritesTestDB(t)
	created, err := store.CreateManualCase(ctx, ManualCaseInput{
		Title: "manual-test surfaced", Summary: "manual-test summary", Severity: "low", Actor: manualTestActor(),
	})
	if err != nil {
		t.Fatalf("CreateManualCase: %v", err)
	}
	if _, err := store.AssignCase(ctx, CaseAssignmentInput{
		CaseID: created.ID, Assignee: "surfaced-subject", AssigneeDisplayName: "Surfaced Name", Actor: manualTestActor(),
	}); err != nil {
		t.Fatalf("AssignCase: %v", err)
	}

	listed, err := store.ListCases(ctx, 100)
	if err != nil {
		t.Fatalf("ListCases: %v", err)
	}
	var found *cases.Case
	for i := range listed {
		if listed[i].ID == created.ID {
			found = &listed[i]
		}
	}
	if found == nil {
		t.Fatal("created case not listed")
	}
	if found.Assignee != "surfaced-subject" || found.AssigneeDisplayName != "Surfaced Name" {
		t.Fatalf("listed case assignee = %q/%q, want snapshot", found.Assignee, found.AssigneeDisplayName)
	}
}

func asProviderError(err error, target *providers.ProviderError) bool {
	providerErr, ok := err.(providers.ProviderError)
	if ok {
		*target = providerErr
	}
	return ok
}

func isNotFound(err error) bool {
	providerErr, ok := err.(providers.ProviderError)
	return ok && providerErr.Class == providers.ErrorNotFound
}

func isInvalidInput(err error) bool {
	var invalid InvalidInputError
	return errors.As(err, &invalid)
}

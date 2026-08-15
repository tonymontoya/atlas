package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tonymontoya/ceph-atlas/internal/cases"
	"github.com/tonymontoya/ceph-atlas/internal/providers"
)

const caseColumns = "id, title, summary, status, severity, source, cluster_fsid::text, assignee, assignee_display_name, created_at, updated_at, closed_at"

// Actor identifies the verified operator responsible for a manual write.
type Actor struct {
	Subject     string
	DisplayName string
}

type ManualCaseInput struct {
	Title       string
	Summary     string
	Severity    string
	ClusterFSID string
	Actor       Actor
}

type CaseTransitionInput struct {
	CaseID int64
	To     cases.CaseStatus
	Actor  Actor
}

type CaseAssignmentInput struct {
	CaseID              int64
	Assignee            string
	AssigneeDisplayName string
	Actor               Actor
}

type CaseNoteInput struct {
	CaseID int64
	Body   string
	Actor  Actor
}

func validateActor(actor Actor) error {
	if actor.Subject == "" {
		return inputError("actor subject is required")
	}
	if actor.DisplayName == "" {
		return inputError("actor display name is required")
	}
	return nil
}

func inputError(message string) error {
	return InvalidInputError{Message: message}
}

func (s *PostgresStore) CreateManualCase(ctx context.Context, input ManualCaseInput) (cases.Case, error) {
	if input.Title == "" {
		return cases.Case{}, inputError("title is required")
	}
	if input.Summary == "" {
		return cases.Case{}, inputError("summary is required")
	}
	severity, err := cases.ParseCaseSeverity(input.Severity)
	if err != nil {
		return cases.Case{}, inputError(err.Error())
	}
	if err := validateActor(input.Actor); err != nil {
		return cases.Case{}, err
	}
	if input.ClusterFSID != "" && !IsUUIDShape(input.ClusterFSID) {
		return cases.Case{}, inputError("clusterFsid must be a UUID")
	}

	occurredAt := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return cases.Case{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var clusterFSID sql.NullString
	if input.ClusterFSID != "" {
		clusterFSID = sql.NullString{String: strings.ToLower(input.ClusterFSID), Valid: true}
	}

	row := tx.QueryRowContext(ctx, `
		INSERT INTO cases (title, summary, status, severity, source, cluster_fsid, created_at, updated_at)
		VALUES ($1, $2, 'detected', $3, 'manual', $4::uuid, $5, $5)
		RETURNING `+caseColumns,
		input.Title, input.Summary, severity, clusterFSID, occurredAt)
	created, err := scanCase(row)
	if err != nil {
		return cases.Case{}, err
	}

	payload, err := json.Marshal(struct {
		Source      string  `json:"source"`
		ClusterFSID *string `json:"clusterFsid,omitempty"`
	}{Source: string(cases.CaseSourceManual), ClusterFSID: nullableString(clusterFSID)})
	if err != nil {
		return cases.Case{}, err
	}
	if err := insertTimelineEvent(ctx, tx, created.ID, cases.TimelineEventCaseDetected, "Case created manually.", occurredAt, input.Actor, payload); err != nil {
		return cases.Case{}, err
	}
	if err := tx.Commit(); err != nil {
		return cases.Case{}, err
	}
	return created, nil
}

func (s *PostgresStore) TransitionCase(ctx context.Context, input CaseTransitionInput) (cases.Case, error) {
	if err := validateActor(input.Actor); err != nil {
		return cases.Case{}, err
	}
	target, err := cases.ParseCaseStatus(string(input.To))
	if err != nil {
		return cases.Case{}, inputError(err.Error())
	}

	occurredAt := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return cases.Case{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	current, err := lockCaseForUpdate(ctx, tx, input.CaseID)
	if err != nil {
		return cases.Case{}, err
	}
	if err := cases.CanTransition(current.Status, target); err != nil {
		return cases.Case{}, providers.ProviderError{Class: providers.ErrorConflict, Message: err.Error()}
	}

	var closedAt sql.NullTime
	if target == cases.CaseStatusClosed {
		closedAt = sql.NullTime{Time: occurredAt, Valid: true}
	}
	row := tx.QueryRowContext(ctx, `
		UPDATE cases
		SET status = $2, closed_at = $3, updated_at = $4
		WHERE id = $1
		RETURNING `+caseColumns,
		input.CaseID, target, closedAt, occurredAt)
	updated, err := scanCase(row)
	if err != nil {
		return cases.Case{}, err
	}

	eventType := cases.TimelineEventCaseStatusChanged
	message := fmt.Sprintf("Case status changed to %s.", target)
	if target == cases.CaseStatusTriaged {
		eventType = cases.TimelineEventCaseTriaged
		message = "Case triaged."
	}
	payload, err := json.Marshal(struct {
		PreviousStatus cases.CaseStatus `json:"previousStatus"`
		NewStatus      cases.CaseStatus `json:"newStatus"`
	}{PreviousStatus: current.Status, NewStatus: target})
	if err != nil {
		return cases.Case{}, err
	}
	if err := insertTimelineEvent(ctx, tx, input.CaseID, eventType, message, occurredAt, input.Actor, payload); err != nil {
		return cases.Case{}, err
	}

	if err := tx.Commit(); err != nil {
		return cases.Case{}, err
	}
	return updated, nil
}

func (s *PostgresStore) AssignCase(ctx context.Context, input CaseAssignmentInput) (cases.Case, error) {
	if err := validateActor(input.Actor); err != nil {
		return cases.Case{}, err
	}
	if input.Assignee != "" && input.AssigneeDisplayName == "" {
		return cases.Case{}, inputError("assignee display name is required when assigning")
	}
	if input.Assignee == "" && input.AssigneeDisplayName != "" {
		return cases.Case{}, inputError("assignee display name must be empty when unassigning")
	}

	occurredAt := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return cases.Case{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	current, err := lockCaseForUpdate(ctx, tx, input.CaseID)
	if err != nil {
		return cases.Case{}, err
	}
	if current.Status == cases.CaseStatusClosed {
		return cases.Case{}, providers.ProviderError{Class: providers.ErrorConflict, Message: "case is closed"}
	}
	if current.Assignee == input.Assignee {
		if err := tx.Commit(); err != nil {
			return cases.Case{}, err
		}
		return current, nil
	}

	var assignee sql.NullString
	var assigneeDisplayName sql.NullString
	if input.Assignee != "" {
		assignee = sql.NullString{String: input.Assignee, Valid: true}
		assigneeDisplayName = sql.NullString{String: input.AssigneeDisplayName, Valid: true}
	}
	row := tx.QueryRowContext(ctx, `
		UPDATE cases
		SET assignee = $2, assignee_display_name = $3, updated_at = $4
		WHERE id = $1
		RETURNING `+caseColumns,
		input.CaseID, assignee, assigneeDisplayName, occurredAt)
	updated, err := scanCase(row)
	if err != nil {
		return cases.Case{}, err
	}

	message := "Case unassigned."
	if input.Assignee != "" {
		message = fmt.Sprintf("Case assigned to %s.", input.AssigneeDisplayName)
		if current.Assignee != "" {
			message = fmt.Sprintf("Case reassigned to %s.", input.AssigneeDisplayName)
		}
	}
	previousAssignee := nullableString(sql.NullString{String: current.Assignee, Valid: current.Assignee != ""})
	payload, err := json.Marshal(struct {
		PreviousAssignee *string `json:"previousAssignee"`
		NewAssignee      *string `json:"newAssignee"`
	}{PreviousAssignee: previousAssignee, NewAssignee: nullableString(assignee)})
	if err != nil {
		return cases.Case{}, err
	}
	if err := insertTimelineEvent(ctx, tx, input.CaseID, cases.TimelineEventCaseAssigned, message, occurredAt, input.Actor, payload); err != nil {
		return cases.Case{}, err
	}

	if err := tx.Commit(); err != nil {
		return cases.Case{}, err
	}
	return updated, nil
}

func (s *PostgresStore) AddCaseNote(ctx context.Context, input CaseNoteInput) (cases.CaseNote, error) {
	if err := validateActor(input.Actor); err != nil {
		return cases.CaseNote{}, err
	}
	if input.Body == "" {
		return cases.CaseNote{}, inputError("note body is required")
	}

	occurredAt := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return cases.CaseNote{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var note cases.CaseNote
	err = tx.QueryRowContext(ctx, `
		INSERT INTO case_notes (case_id, author_id, author_display_name, body, created_at)
		SELECT c.id, $2, $3, $4, $5
		FROM cases AS c
		WHERE c.id = $1
		RETURNING id, case_id, author_id, author_display_name, body, created_at
	`, input.CaseID, input.Actor.Subject, input.Actor.DisplayName, input.Body, occurredAt).
		Scan(&note.ID, &note.CaseID, &note.AuthorID, &note.AuthorDisplayName, &note.Body, &note.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return cases.CaseNote{}, notFound("case not found")
	}
	if err != nil {
		return cases.CaseNote{}, err
	}

	payload, err := json.Marshal(struct {
		NoteID int64 `json:"noteId"`
	}{NoteID: note.ID})
	if err != nil {
		return cases.CaseNote{}, err
	}
	if err := insertTimelineEvent(ctx, tx, input.CaseID, cases.TimelineEventCaseNoteAdded, "Note added to case.", occurredAt, input.Actor, payload); err != nil {
		return cases.CaseNote{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE cases SET updated_at = $2 WHERE id = $1
	`, input.CaseID, occurredAt); err != nil {
		return cases.CaseNote{}, err
	}

	if err := tx.Commit(); err != nil {
		return cases.CaseNote{}, err
	}
	return note, nil
}

func (s *PostgresStore) ListCaseNotes(ctx context.Context, caseID int64) ([]cases.CaseNote, error) {
	if caseID <= 0 {
		return nil, notFound("case not found")
	}

	var exists bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM cases WHERE id = $1)
	`, caseID).Scan(&exists)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, notFound("case not found")
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, case_id, author_id, author_display_name, body, created_at
		FROM case_notes
		WHERE case_id = $1
		ORDER BY created_at ASC, id ASC
	`, caseID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	notes := make([]cases.CaseNote, 0)
	for rows.Next() {
		var note cases.CaseNote
		if err := rows.Scan(&note.ID, &note.CaseID, &note.AuthorID, &note.AuthorDisplayName, &note.Body, &note.CreatedAt); err != nil {
			return nil, err
		}
		notes = append(notes, note)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return notes, nil
}

func insertTimelineEvent(ctx context.Context, tx *sql.Tx, caseID int64, eventType cases.TimelineEventType, message string, occurredAt time.Time, actor Actor, payload []byte) error {
	return insertTimelineEventWithActorType(ctx, tx, caseID, eventType, message, occurredAt, cases.TimelineActorUser, actor.Subject, actor.DisplayName, payload)
}

func insertTimelineEventWithActorType(ctx context.Context, tx *sql.Tx, caseID int64, eventType cases.TimelineEventType, message string, occurredAt time.Time, actorType cases.TimelineActorType, actorID, actorDisplayName string, payload []byte) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO case_timeline_events (case_id, event_type, message, occurred_at, actor_type, actor_id, actor_display_name, payload)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), $7, $8::jsonb)
	`, caseID, eventType, message, occurredAt, actorType, actorID, actorDisplayName, payload)
	return err
}

func lockCaseForUpdate(ctx context.Context, tx *sql.Tx, caseID int64) (cases.Case, error) {
	if caseID <= 0 {
		return cases.Case{}, notFound("case not found")
	}
	row := tx.QueryRowContext(ctx, `
		SELECT `+caseColumns+`
		FROM cases
		WHERE id = $1
		FOR UPDATE
	`, caseID)
	current, err := scanCase(row)
	if errors.Is(err, sql.ErrNoRows) {
		return cases.Case{}, notFound("case not found")
	}
	if err != nil {
		return cases.Case{}, err
	}
	return current, nil
}

func nullableString(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

// IsUUIDShape reports whether value looks like a canonical 36-character UUID.
func IsUUIDShape(value string) bool {
	if len(value) != 36 {
		return false
	}
	for i, char := range value {
		switch i {
		case 8, 13, 18, 23:
			if char != '-' {
				return false
			}
		default:
			isHex := (char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')
			if !isHex {
				return false
			}
		}
	}
	return true
}

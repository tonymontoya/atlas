package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/tonymontoya/ceph-atlas/internal/actor"
	"github.com/tonymontoya/ceph-atlas/internal/apperr"
	"github.com/tonymontoya/ceph-atlas/internal/cases"
)

// timelineEvent describes one Timeline Event to record on a Case. A nil
// Actor attributes the event to the Atlas system actor; a set Actor
// attributes it to the operating user. Payload is marshaled as part of
// the write.
type timelineEvent struct {
	Type    cases.TimelineEventType
	Message string
	Actor   *actor.Actor
	Payload any
}

// writeTimelineEvent records one Timeline Event on a Case inside the
// caller's open transaction, sharing the caller's occurredAt timestamp.
func writeTimelineEvent(ctx context.Context, tx *sql.Tx, caseID int64, occurredAt time.Time, event timelineEvent) error {
	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return err
	}
	actorType := cases.TimelineActorSystem
	actorID := ""
	actorDisplayName := "Atlas"
	if event.Actor != nil {
		actorType = cases.TimelineActorUser
		actorID = event.Actor.Subject
		actorDisplayName = event.Actor.DisplayName
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO case_timeline_events (case_id, event_type, message, occurred_at, actor_type, actor_id, actor_display_name, payload)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), $7, $8::jsonb)
	`, caseID, event.Type, event.Message, occurredAt, actorType, actorID, actorDisplayName, payload)
	return err
}

// runTransition executes one guarded transition on locked state: it
// begins the transaction, locks the target rows, evaluates the guard
// against the locked state, applies the writes, and commits. Writes are
// therefore always preceded by the lock, and a guard failure is always
// a Conflict on the current state. Pre-transaction validation
// (InvalidRequest) and NotFound mapping stay with the call site and the
// lock function. Idempotent no-ops short-circuit inside apply; the
// transaction still commits.
func runTransition[L, R any](ctx context.Context, db *sql.DB,
	lock func(context.Context, *sql.Tx) (L, error),
	guard func(L) error,
	apply func(context.Context, *sql.Tx, L) (R, error),
) (R, error) {
	var zero R
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return zero, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	locked, err := lock(ctx, tx)
	if err != nil {
		return zero, err
	}
	if err := guard(locked); err != nil {
		return zero, apperr.Error{Class: apperr.Conflict, Message: err.Error()}
	}

	result, err := apply(ctx, tx, locked)
	if err != nil {
		return zero, err
	}
	if err := tx.Commit(); err != nil {
		return zero, err
	}
	return result, nil
}

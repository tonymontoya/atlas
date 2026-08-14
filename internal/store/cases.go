package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/tonymontoya/ceph-atlas/internal/cases"
	"github.com/tonymontoya/ceph-atlas/internal/providers"
)

func (s *PostgresStore) ListCases(ctx context.Context, limit int) ([]cases.Case, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, title, summary, status, severity, source, cluster_fsid::text, created_at, updated_at, closed_at
		FROM cases
		ORDER BY updated_at DESC, id DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	result := make([]cases.Case, 0)
	for rows.Next() {
		item, err := scanCase(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *PostgresStore) GetCase(ctx context.Context, id int64) (cases.Case, error) {
	if id <= 0 {
		return cases.Case{}, notFound("case not found")
	}

	row := s.db.QueryRowContext(ctx, `
		SELECT id, title, summary, status, severity, source, cluster_fsid::text, created_at, updated_at, closed_at
		FROM cases
		WHERE id = $1
	`, id)
	item, err := scanCase(row)
	if errors.Is(err, sql.ErrNoRows) {
		return cases.Case{}, notFound("case not found")
	}
	if err != nil {
		return cases.Case{}, err
	}
	return item, nil
}

func (s *PostgresStore) ListCaseTimeline(ctx context.Context, caseID int64) ([]cases.TimelineEvent, error) {
	if caseID <= 0 {
		return nil, notFound("case not found")
	}

	var exists bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM cases
			WHERE id = $1
		)
	`, caseID).Scan(&exists)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, notFound("case not found")
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, case_id, event_type, message, occurred_at, actor_type, actor_id, actor_display_name, payload
		FROM case_timeline_events
		WHERE case_id = $1
		ORDER BY occurred_at ASC, id ASC
	`, caseID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	result := make([]cases.TimelineEvent, 0)
	for rows.Next() {
		item, err := scanTimelineEvent(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func scanCase(scanner rowScanner) (cases.Case, error) {
	var item cases.Case
	var clusterFSID sql.NullString
	var closedAt sql.NullTime
	if err := scanner.Scan(
		&item.ID,
		&item.Title,
		&item.Summary,
		&item.Status,
		&item.Severity,
		&item.Source,
		&clusterFSID,
		&item.CreatedAt,
		&item.UpdatedAt,
		&closedAt,
	); err != nil {
		return cases.Case{}, err
	}
	if clusterFSID.Valid {
		item.ClusterFSID = clusterFSID.String
	}
	if closedAt.Valid {
		item.ClosedAt = &closedAt.Time
	}
	return item, nil
}

func scanTimelineEvent(scanner rowScanner) (cases.TimelineEvent, error) {
	var item cases.TimelineEvent
	var actorID sql.NullString
	var payloadJSON []byte
	if err := scanner.Scan(
		&item.ID,
		&item.CaseID,
		&item.Type,
		&item.Message,
		&item.OccurredAt,
		&item.Actor.Type,
		&actorID,
		&item.Actor.DisplayName,
		&payloadJSON,
	); err != nil {
		return cases.TimelineEvent{}, err
	}
	if actorID.Valid {
		item.Actor.ID = actorID.String
	}
	if len(payloadJSON) == 0 {
		item.Payload = map[string]any{}
		return item, nil
	}
	if err := json.Unmarshal(payloadJSON, &item.Payload); err != nil {
		return cases.TimelineEvent{}, providers.ProviderError{
			Class:   providers.ErrorMalformedResponse,
			Message: "parse case timeline payload: " + err.Error(),
		}
	}
	if item.Payload == nil {
		item.Payload = map[string]any{}
	}
	return item, nil
}

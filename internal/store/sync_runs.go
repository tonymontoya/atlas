package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type BeginSyncRun struct {
	Provider string
	Scenario string
}

type SyncRunFailure struct {
	RunID        int64
	ErrorClass   string
	ErrorMessage string
}

type SyncRunResult struct {
	RunID      int64
	SnapshotID int64
}

type InventorySyncRun struct {
	ID           int64      `json:"id"`
	Provider     string     `json:"provider"`
	Scenario     string     `json:"scenario,omitempty"`
	Status       string     `json:"status"`
	StartedAt    time.Time  `json:"startedAt"`
	FinishedAt   *time.Time `json:"finishedAt,omitempty"`
	SnapshotID   *int64     `json:"snapshotId,omitempty"`
	ErrorClass   string     `json:"errorClass,omitempty"`
	ErrorMessage string     `json:"errorMessage,omitempty"`
}

func (s *PostgresStore) BeginInventorySyncRun(ctx context.Context, run BeginSyncRun) (int64, error) {
	if run.Provider == "" {
		return 0, errors.New("provider is required")
	}
	var runID int64
	scenario := sql.NullString{String: run.Scenario, Valid: run.Scenario != ""}
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO inventory_sync_runs (provider, scenario, status)
		VALUES ($1, $2, 'running')
		RETURNING id
	`, run.Provider, scenario).Scan(&runID)
	return runID, err
}

func (s *PostgresStore) SucceedInventorySyncRun(ctx context.Context, result SyncRunResult) error {
	commandTag, err := s.db.ExecContext(ctx, `
		UPDATE inventory_sync_runs
		SET status = 'succeeded',
			finished_at = now(),
			snapshot_id = $2,
			error_class = NULL,
			error_message = NULL
		WHERE id = $1 AND status = 'running'
	`, result.RunID, result.SnapshotID)
	if err != nil {
		return err
	}
	rows, err := commandTag.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return errors.New("running sync run not found")
	}
	return nil
}

func (s *PostgresStore) FailInventorySyncRun(ctx context.Context, failure SyncRunFailure) error {
	if failure.ErrorClass == "" {
		return errors.New("error class is required")
	}
	if failure.ErrorMessage == "" {
		return errors.New("error message is required")
	}
	commandTag, err := s.db.ExecContext(ctx, `
		UPDATE inventory_sync_runs
		SET status = 'failed',
			finished_at = now(),
			error_class = $2,
			error_message = $3
		WHERE id = $1 AND status = 'running'
	`, failure.RunID, failure.ErrorClass, failure.ErrorMessage)
	if err != nil {
		return err
	}
	rows, err := commandTag.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return errors.New("running sync run not found")
	}
	return nil
}

func (s *PostgresStore) ListInventorySyncRuns(ctx context.Context, limit int) ([]InventorySyncRun, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, provider, scenario, status, started_at, finished_at, snapshot_id, error_class, error_message
		FROM inventory_sync_runs
		ORDER BY started_at DESC, id DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	runs := make([]InventorySyncRun, 0)
	for rows.Next() {
		var run InventorySyncRun
		var scenario sql.NullString
		var finishedAt sql.NullTime
		var snapshotID sql.NullInt64
		var errorClass sql.NullString
		var errorMessage sql.NullString
		if err := rows.Scan(
			&run.ID,
			&run.Provider,
			&scenario,
			&run.Status,
			&run.StartedAt,
			&finishedAt,
			&snapshotID,
			&errorClass,
			&errorMessage,
		); err != nil {
			return nil, err
		}
		if scenario.Valid {
			run.Scenario = scenario.String
		}
		if finishedAt.Valid {
			run.FinishedAt = &finishedAt.Time
		}
		if snapshotID.Valid {
			run.SnapshotID = &snapshotID.Int64
		}
		if errorClass.Valid {
			run.ErrorClass = errorClass.String
		}
		if errorMessage.Valid {
			run.ErrorMessage = errorMessage.String
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return runs, nil
}

package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

type BeginSyncRun struct {
	Provider string
	Scenario string
	// ClusterID attributes the run to a cluster known before it
	// starts. Agent pushes resolve the cluster from the client
	// certificate up front; pulls leave it nil and learn the cluster
	// from the saved snapshot when the run succeeds.
	ClusterID *int64
}

type SyncRunFailure struct {
	RunID        int64
	ErrorClass   string
	ErrorMessage string
}

type SyncRunResult struct {
	RunID      int64
	SnapshotID int64
	// ClusterID attributes the succeeded run to the cluster the save
	// resolved; SucceedInventorySyncRun stamps it unless the run
	// already carried an attribution from begin.
	ClusterID int64
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
	// ClusterFSID and ClusterName identify the cluster the run touched:
	// present when the run is attributed (agent pushes from the start,
	// succeeded runs from their snapshot), absent for failed pulls that
	// never reached a cluster.
	ClusterFSID *string `json:"clusterFsid,omitempty"`
	ClusterName *string `json:"clusterName,omitempty"`
}

func (s *PostgresStore) BeginInventorySyncRun(ctx context.Context, run BeginSyncRun) (int64, error) {
	if run.Provider == "" {
		return 0, errors.New("provider is required")
	}
	var runID int64
	scenario := sql.NullString{String: run.Scenario, Valid: run.Scenario != ""}
	var clusterID sql.NullInt64
	if run.ClusterID != nil {
		clusterID = sql.NullInt64{Int64: *run.ClusterID, Valid: true}
	}
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO inventory_sync_runs (provider, scenario, cluster_id, status)
		VALUES ($1, $2, $3, 'running')
		RETURNING id
	`, run.Provider, scenario, clusterID).Scan(&runID)
	return runID, err
}

func (s *PostgresStore) SucceedInventorySyncRun(ctx context.Context, result SyncRunResult) error {
	commandTag, err := s.db.ExecContext(ctx, `
		UPDATE inventory_sync_runs
		SET status = 'succeeded',
			finished_at = now(),
			snapshot_id = $2,
			cluster_id = COALESCE(cluster_id, $3),
			error_class = NULL,
			error_message = NULL
		WHERE id = $1 AND status = 'running'
	`, result.RunID, result.SnapshotID, sql.NullInt64{Int64: result.ClusterID, Valid: result.ClusterID > 0})
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

// clusterFSID lists runs across clusters; a set one narrows to that
// cluster's runs — the same vocabulary ListCases uses for its cluster
// filter.
func (s *PostgresStore) ListInventorySyncRuns(ctx context.Context, limit int, clusterFSID string) ([]InventorySyncRun, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	if clusterFSID != "" && !IsUUIDShape(clusterFSID) {
		return nil, inputError("cluster filter must be a UUID")
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT runs.id, runs.provider, runs.scenario, runs.status, runs.started_at,
			runs.finished_at, runs.snapshot_id, runs.error_class, runs.error_message,
			clusters.fsid::text, clusters.name
		FROM inventory_sync_runs AS runs
		LEFT JOIN atlas_clusters AS clusters ON clusters.id = runs.cluster_id
		WHERE ($2::text = '' OR clusters.fsid = $2::uuid)
		ORDER BY runs.started_at DESC, runs.id DESC
		LIMIT $1
	`, limit, strings.ToLower(clusterFSID))
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
		var clusterFSID sql.NullString
		var clusterName sql.NullString
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
			&clusterFSID,
			&clusterName,
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
		if clusterFSID.Valid && clusterName.Valid {
			fsid := clusterFSID.String
			run.ClusterFSID = &fsid
			name := clusterName.String
			run.ClusterName = &name
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return runs, nil
}

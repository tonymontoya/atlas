package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"
)

type AlertCandidate struct {
	Fingerprint  string
	Name         string
	Title        string
	Summary      string
	Severity     string
	Source       string
	Signal       string
	ClusterLabel string
	OSDLabel     string
	State        string
	StartedAt    time.Time
}

type AlertDetection struct {
	Provider    string
	Scenario    string
	EvaluatedAt time.Time
	Candidates  []AlertCandidate
}

type DetectionResult struct {
	AlertsEvaluated int
	CasesCreated    int
}

type BeginEvaluationRun struct {
	Provider string
	Scenario string
}

type EvaluationRunResult struct {
	RunID           int64
	AlertsEvaluated int
	CasesCreated    int
}

type EvaluationRunFailure struct {
	RunID        int64
	ErrorClass   string
	ErrorMessage string
}

type AlertEvaluationRun struct {
	ID              int64      `json:"id"`
	Provider        string     `json:"provider"`
	Scenario        string     `json:"scenario,omitempty"`
	Status          string     `json:"status"`
	StartedAt       time.Time  `json:"startedAt"`
	FinishedAt      *time.Time `json:"finishedAt,omitempty"`
	ErrorClass      string     `json:"errorClass,omitempty"`
	ErrorMessage    string     `json:"errorMessage,omitempty"`
	AlertsEvaluated *int       `json:"alertsEvaluated,omitempty"`
	CasesCreated    *int       `json:"casesCreated,omitempty"`
}

func (s *PostgresStore) BeginAlertEvaluationRun(ctx context.Context, run BeginEvaluationRun) (int64, error) {
	if run.Provider == "" {
		return 0, errors.New("provider is required")
	}
	var runID int64
	scenario := sql.NullString{String: run.Scenario, Valid: run.Scenario != ""}
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO alert_evaluation_runs (provider, scenario, status)
		VALUES ($1, $2, 'running')
		RETURNING id
	`, run.Provider, scenario).Scan(&runID)
	return runID, err
}

func (s *PostgresStore) SucceedAlertEvaluationRun(ctx context.Context, result EvaluationRunResult) error {
	commandTag, err := s.db.ExecContext(ctx, `
		UPDATE alert_evaluation_runs
		SET status = 'succeeded',
			finished_at = now(),
			alerts_evaluated = $2,
			cases_created = $3,
			error_class = NULL,
			error_message = NULL
		WHERE id = $1 AND status = 'running'
	`, result.RunID, result.AlertsEvaluated, result.CasesCreated)
	if err != nil {
		return err
	}
	rows, err := commandTag.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return errors.New("running alert evaluation run not found")
	}
	return nil
}

func (s *PostgresStore) FailAlertEvaluationRun(ctx context.Context, failure EvaluationRunFailure) error {
	if failure.ErrorClass == "" {
		return errors.New("error class is required")
	}
	if failure.ErrorMessage == "" {
		return errors.New("error message is required")
	}
	commandTag, err := s.db.ExecContext(ctx, `
		UPDATE alert_evaluation_runs
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
		return errors.New("running alert evaluation run not found")
	}
	return nil
}

func (s *PostgresStore) ListAlertEvaluationRuns(ctx context.Context, limit int) ([]AlertEvaluationRun, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, provider, scenario, status, started_at, finished_at, error_class, error_message, alerts_evaluated, cases_created
		FROM alert_evaluation_runs
		ORDER BY started_at DESC, id DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	runs := make([]AlertEvaluationRun, 0)
	for rows.Next() {
		var run AlertEvaluationRun
		var scenario sql.NullString
		var finishedAt sql.NullTime
		var errorClass sql.NullString
		var errorMessage sql.NullString
		var alertsEvaluated sql.NullInt64
		var casesCreated sql.NullInt64
		if err := rows.Scan(
			&run.ID,
			&run.Provider,
			&scenario,
			&run.Status,
			&run.StartedAt,
			&finishedAt,
			&errorClass,
			&errorMessage,
			&alertsEvaluated,
			&casesCreated,
		); err != nil {
			return nil, err
		}
		if scenario.Valid {
			run.Scenario = scenario.String
		}
		if finishedAt.Valid {
			run.FinishedAt = &finishedAt.Time
		}
		if errorClass.Valid {
			run.ErrorClass = errorClass.String
		}
		if errorMessage.Valid {
			run.ErrorMessage = errorMessage.String
		}
		if alertsEvaluated.Valid {
			value := int(alertsEvaluated.Int64)
			run.AlertsEvaluated = &value
		}
		if casesCreated.Valid {
			value := int(casesCreated.Int64)
			run.CasesCreated = &value
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return runs, nil
}

func (s *PostgresStore) DetectFromAlerts(ctx context.Context, detection AlertDetection) (DetectionResult, error) {
	detection, err := normalizeAlertDetection(detection)
	if err != nil {
		return DetectionResult{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return DetectionResult{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext('atlas:alert-detection')::bigint)`); err != nil {
		return DetectionResult{}, err
	}

	result := DetectionResult{AlertsEvaluated: len(detection.Candidates)}
	for _, candidate := range detection.Candidates {
		created, err := detectCandidate(ctx, tx, candidate, detection.EvaluatedAt)
		if err != nil {
			return DetectionResult{}, err
		}
		if created {
			result.CasesCreated++
		}
	}

	if err := tx.Commit(); err != nil {
		return DetectionResult{}, err
	}
	return result, nil
}

func normalizeAlertDetection(detection AlertDetection) (AlertDetection, error) {
	if detection.Provider == "" {
		return AlertDetection{}, errors.New("provider is required")
	}
	if detection.EvaluatedAt.IsZero() {
		return AlertDetection{}, errors.New("evaluated_at is required")
	}
	for _, candidate := range detection.Candidates {
		if candidate.Fingerprint == "" {
			return AlertDetection{}, errors.New("candidate fingerprint is required")
		}
		if candidate.Name == "" {
			return AlertDetection{}, errors.New("candidate name is required")
		}
		if candidate.Title == "" {
			return AlertDetection{}, errors.New("candidate title is required")
		}
		if candidate.Summary == "" {
			return AlertDetection{}, errors.New("candidate summary is required")
		}
		if candidate.Severity == "" {
			return AlertDetection{}, errors.New("candidate severity is required")
		}
		if candidate.Source == "" {
			return AlertDetection{}, errors.New("candidate source is required")
		}
		switch candidate.State {
		case "firing", "pending", "resolved":
		default:
			return AlertDetection{}, fmt.Errorf("candidate %q has unknown state %q", candidate.Name, candidate.State)
		}
	}
	return detection, nil
}

func detectCandidate(ctx context.Context, tx *sql.Tx, candidate AlertCandidate, evaluatedAt time.Time) (bool, error) {
	var linkedCaseID int64
	var linkedCaseStatus string
	err := tx.QueryRowContext(ctx, `
		SELECT dedup.case_id, cases.status
		FROM case_alert_dedup AS dedup
		JOIN cases ON cases.id = dedup.case_id
		WHERE dedup.fingerprint = $1
	`, candidate.Fingerprint).Scan(&linkedCaseID, &linkedCaseStatus)

	if candidate.State == "firing" {
		if errors.Is(err, sql.ErrNoRows) || (err == nil && linkedCaseStatus == "closed") {
			_, err := createDetectedCase(ctx, tx, candidate, evaluatedAt)
			if err != nil {
				return false, err
			}
			return true, nil
		}
		if err != nil {
			return false, err
		}
		if err := touchDedupRow(ctx, tx, candidate, evaluatedAt, sql.NullString{String: "open", Valid: true}); err != nil {
			return false, err
		}
		return false, nil
	}

	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	newState := sql.NullString{}
	if candidate.State == "resolved" {
		newState = sql.NullString{String: "resolved", Valid: true}
	}
	if err := touchDedupRow(ctx, tx, candidate, evaluatedAt, newState); err != nil {
		return false, err
	}
	return false, nil
}

func touchDedupRow(ctx context.Context, tx *sql.Tx, candidate AlertCandidate, seenAt time.Time, state sql.NullString) error {
	commandTag, err := tx.ExecContext(ctx, `
		UPDATE case_alert_dedup
		SET last_seen_at = $2,
			alert_name = $3,
			cluster_label = $4,
			state = COALESCE($5, state)
		WHERE fingerprint = $1
	`, candidate.Fingerprint, seenAt, candidate.Name, candidate.ClusterLabel, state)
	if err != nil {
		return err
	}
	rows, err := commandTag.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return errors.New("dedup row disappeared during detection")
	}
	return nil
}

func createDetectedCase(ctx context.Context, tx *sql.Tx, candidate AlertCandidate, evaluatedAt time.Time) (int64, error) {
	clusterFSID, err := resolveClusterLabel(ctx, tx, candidate.ClusterLabel)
	if err != nil {
		return 0, err
	}
	osdContext, err := resolveOSDContext(ctx, tx, clusterFSID, candidate.OSDLabel)
	if err != nil {
		return 0, err
	}

	var caseID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO cases (title, summary, status, severity, source, cluster_fsid, created_at, updated_at)
		VALUES ($1, $2, 'detected', $3, $4, $5::uuid, $6, $6)
		RETURNING id
	`, candidate.Title, candidate.Summary, candidate.Severity, candidate.Source, clusterFSID, evaluatedAt).Scan(&caseID)
	if err != nil {
		return 0, err
	}

	payload, err := detectedPayload(candidate, clusterFSID, osdContext)
	if err != nil {
		return 0, err
	}
	message := fmt.Sprintf("%s detected from %s alert context.", candidate.Title, candidate.Source)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO case_timeline_events (case_id, event_type, message, occurred_at, actor_type, actor_display_name, payload)
		VALUES ($1, 'case_detected', $2, $3, 'system', 'Atlas', $4::jsonb)
	`, caseID, message, evaluatedAt, payload); err != nil {
		return 0, err
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO case_alert_dedup (fingerprint, case_id, state, alert_name, cluster_label, first_seen_at, last_seen_at)
		VALUES ($1, $2, 'open', $3, $4, $5, $5)
		ON CONFLICT (fingerprint) DO UPDATE SET
			case_id = EXCLUDED.case_id,
			state = 'open',
			alert_name = EXCLUDED.alert_name,
			cluster_label = EXCLUDED.cluster_label,
			last_seen_at = EXCLUDED.last_seen_at
	`, candidate.Fingerprint, caseID, candidate.Name, candidate.ClusterLabel, evaluatedAt); err != nil {
		return 0, err
	}
	return caseID, nil
}

func resolveClusterLabel(ctx context.Context, tx *sql.Tx, label string) (sql.NullString, error) {
	if label == "" {
		return sql.NullString{}, nil
	}
	var fsid string
	err := tx.QueryRowContext(ctx, `
		SELECT fsid::text
		FROM atlas_clusters
		WHERE name = $1 OR fsid::text = $1
		ORDER BY (name = $1) DESC, name
		LIMIT 1
	`, label).Scan(&fsid)
	if errors.Is(err, sql.ErrNoRows) {
		return sql.NullString{}, nil
	}
	if err != nil {
		return sql.NullString{}, err
	}
	return sql.NullString{String: fsid, Valid: true}, nil
}

type osdContext struct {
	OSD  int
	Host string
}

func resolveOSDContext(ctx context.Context, tx *sql.Tx, clusterFSID sql.NullString, osdLabel string) (osdContext, error) {
	if !clusterFSID.Valid || osdLabel == "" {
		return osdContext{}, nil
	}
	osdID, err := strconv.Atoi(osdLabel)
	if err != nil {
		return osdContext{}, nil
	}
	var host string
	err = tx.QueryRowContext(ctx, `
		SELECT host
		FROM cluster_current_osds
		WHERE fsid = $1 AND osd_id = $2
		LIMIT 1
	`, clusterFSID.String, osdID).Scan(&host)
	if errors.Is(err, sql.ErrNoRows) {
		return osdContext{}, nil
	}
	if err != nil {
		return osdContext{}, err
	}
	return osdContext{OSD: osdID, Host: host}, nil
}

func detectedPayload(candidate AlertCandidate, clusterFSID sql.NullString, osdContext osdContext) (string, error) {
	payload := struct {
		Source      string  `json:"source"`
		ClusterFSID *string `json:"clusterFsid,omitempty"`
		Signal      string  `json:"signal"`
		OSD         *int    `json:"osd,omitempty"`
		Host        string  `json:"host,omitempty"`
	}{
		Source: candidate.Source,
		Signal: candidate.Signal,
	}
	if clusterFSID.Valid {
		fsid := clusterFSID.String
		payload.ClusterFSID = &fsid
	}
	if osdContext.Host != "" {
		osd := osdContext.OSD
		payload.OSD = &osd
		payload.Host = osdContext.Host
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/tonymontoya/ceph-atlas/internal/fleet"
	"github.com/tonymontoya/ceph-atlas/internal/inventory"
	"github.com/tonymontoya/ceph-atlas/internal/testdb"
)

func detectionTestDB(t *testing.T) (*PostgresStore, context.Context) {
	t.Helper()
	db, _ := testdb.Open(t)

	cleanupDetectionRows(t, db)
	t.Cleanup(func() { cleanupDetectionRows(t, db) })
	return NewPostgres(db), context.Background()
}

func cleanupDetectionRows(t *testing.T, db *sql.DB) {
	t.Helper()
	testdb.DeleteCases(t, db, "title LIKE 'detection-test%'")
	testdb.DeleteAlertRuns(t, db, "provider = 'fake'")
	testdb.DeleteClusters(t, db, "fsid = '00000000-0000-4000-8000-000000000906'")
}

func detectionCandidate(fingerprint, clusterLabel, state string) AlertCandidate {
	return AlertCandidate{
		Fingerprint:  fingerprint,
		Name:         "CephOSDDown",
		Title:        "detection-test CephOSDDown on osd=1",
		Summary:      "detection-test OSD 1 is down",
		Severity:     "high",
		Source:       "prometheus",
		Signal:       "CEPH_OSD_DOWN",
		ClusterLabel: clusterLabel,
		OSDLabel:     "1",
		State:        state,
		StartedAt:    time.Date(2026, 8, 14, 9, 15, 0, 0, time.UTC),
	}
}

func syncDetectionTestCluster(t *testing.T, store *PostgresStore, ctx context.Context) {
	t.Helper()
	_, err := store.SaveInventoryObservation(ctx, InventoryObservation{
		Provider:   "fake",
		ObservedAt: time.Date(2026, 8, 14, 9, 0, 0, 0, time.UTC),
		Cluster: fleet.ClusterIdentity{
			FSID:        "00000000-0000-4000-8000-000000000906",
			Name:        "detection-test-cluster",
			CephVersion: "18.2.x",
			Type:        "bare-metal",
		},
		Health: inventory.Health{Status: inventory.HealthOK, Summary: "test cluster"},
		OSDs: []inventory.OSD{
			{ID: 1, Host: "detection-test-host.example.invalid", Up: false, In: true},
		},
	})
	if err != nil {
		t.Fatalf("save inventory observation: %v", err)
	}
}

func TestDetectFromAlertsCreatesCaseTimelineAndDedupRow(t *testing.T) {
	store, ctx := detectionTestDB(t)
	syncDetectionTestCluster(t, store, ctx)

	evaluatedAt := time.Date(2026, 8, 14, 9, 20, 0, 0, time.UTC)
	result, err := store.DetectFromAlerts(ctx, AlertDetection{
		Provider:    "detection-test",
		EvaluatedAt: evaluatedAt,
		Candidates: []AlertCandidate{
			detectionCandidate("detection-test-1", "detection-test-cluster", "firing"),
		},
	})
	if err != nil {
		t.Fatalf("DetectFromAlerts: %v", err)
	}
	if result.AlertsEvaluated != 1 || result.CasesCreated != 1 {
		t.Fatalf("result = %+v, want 1 evaluated and 1 created", result)
	}

	var caseID int64
	var status, severity, source, clusterFSID string
	var createdAt time.Time
	if err := store.db.QueryRowContext(ctx, `
		SELECT id, status, severity, source, cluster_fsid::text, created_at
		FROM cases
		WHERE title = 'detection-test CephOSDDown on osd=1'
	`).Scan(&caseID, &status, &severity, &source, &clusterFSID, &createdAt); err != nil {
		t.Fatalf("query detected case: %v", err)
	}
	if status != "detected" || severity != "high" || source != "prometheus" {
		t.Fatalf("case status/severity/source = %q/%q/%q", status, severity, source)
	}
	if clusterFSID != "00000000-0000-4000-8000-000000000906" {
		t.Fatalf("case cluster fsid = %q, want the label-resolved cluster", clusterFSID)
	}
	if !createdAt.Equal(evaluatedAt) {
		t.Fatalf("case created_at = %v, want %v", createdAt, evaluatedAt)
	}

	var eventType, message string
	var payloadBytes []byte
	if err := store.db.QueryRowContext(ctx, `
		SELECT event_type, message, payload
		FROM case_timeline_events
		WHERE case_id = $1
	`, caseID).Scan(&eventType, &message, &payloadBytes); err != nil {
		t.Fatalf("query timeline event: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		t.Fatalf("parse timeline payload: %v", err)
	}
	if eventType != "case_detected" {
		t.Fatalf("timeline event type = %q, want case_detected", eventType)
	}
	if message == "" {
		t.Fatal("timeline event message is empty")
	}
	if payload["source"] != "prometheus" || payload["signal"] != "CEPH_OSD_DOWN" {
		t.Fatalf("timeline payload = %+v, want source and signal", payload)
	}
	if payload["clusterFsid"] != "00000000-0000-4000-8000-000000000906" {
		t.Fatalf("timeline payload = %+v, want clusterFsid", payload)
	}
	if payload["osd"] != float64(1) || payload["host"] != "detection-test-host.example.invalid" {
		t.Fatalf("timeline payload = %+v, want osd and host context from the current read model", payload)
	}

	var dedupState, dedupName string
	var linkedID int64
	if err := store.db.QueryRowContext(ctx, `
		SELECT case_id, state, alert_name
		FROM case_alert_dedup
		WHERE fingerprint = 'detection-test-1'
	`).Scan(&linkedID, &dedupState, &dedupName); err != nil {
		t.Fatalf("query dedup row: %v", err)
	}
	if linkedID != caseID || dedupState != "open" || dedupName != "CephOSDDown" {
		t.Fatalf("dedup row = %d/%q/%q, want case %d open CephOSDDown", linkedID, dedupState, dedupName, caseID)
	}

	detailed, err := store.GetCase(ctx, caseID)
	if err != nil {
		t.Fatalf("GetCase for detected case: %v", err)
	}
	if detailed.DetectedBy == nil {
		t.Fatal("detected case has no detection link")
	}
	if detailed.DetectedBy.Source != "prometheus" || detailed.DetectedBy.AlertName != "CephOSDDown" {
		t.Fatalf("detection link = %+v, want prometheus CephOSDDown", detailed.DetectedBy)
	}
	if detailed.DetectedBy.Signal != "CEPH_OSD_DOWN" {
		t.Fatalf("detection link signal = %q, want CEPH_OSD_DOWN", detailed.DetectedBy.Signal)
	}
	if !detailed.DetectedBy.FirstSeenAt.Equal(evaluatedAt) || !detailed.DetectedBy.LastSeenAt.Equal(evaluatedAt) {
		t.Fatalf("detection link seen at = %v..%v, want %v", detailed.DetectedBy.FirstSeenAt, detailed.DetectedBy.LastSeenAt, evaluatedAt)
	}
}

func TestDetectFromAlertsIsIdempotentForFiringAlerts(t *testing.T) {
	store, ctx := detectionTestDB(t)
	syncDetectionTestCluster(t, store, ctx)

	candidate := detectionCandidate("detection-test-2", "detection-test-cluster", "firing")
	detection := AlertDetection{
		Provider:    "detection-test",
		EvaluatedAt: time.Date(2026, 8, 14, 9, 20, 0, 0, time.UTC),
		Candidates:  []AlertCandidate{candidate},
	}
	if _, err := store.DetectFromAlerts(ctx, detection); err != nil {
		t.Fatalf("first DetectFromAlerts: %v", err)
	}
	detection.EvaluatedAt = time.Date(2026, 8, 14, 9, 25, 0, 0, time.UTC)
	second, err := store.DetectFromAlerts(ctx, detection)
	if err != nil {
		t.Fatalf("second DetectFromAlerts: %v", err)
	}
	if second.CasesCreated != 0 {
		t.Fatalf("second evaluation created %d cases, want 0", second.CasesCreated)
	}

	var caseCount int
	if err := store.db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM cases
		JOIN case_alert_dedup ON case_alert_dedup.case_id = cases.id
		WHERE case_alert_dedup.fingerprint = 'detection-test-2'
	`).Scan(&caseCount); err != nil {
		t.Fatalf("count cases for fingerprint: %v", err)
	}
	if caseCount != 1 {
		t.Fatalf("case count for fingerprint = %d, want 1", caseCount)
	}

	var lastSeen time.Time
	if err := store.db.QueryRowContext(ctx, `
		SELECT last_seen_at FROM case_alert_dedup WHERE fingerprint = 'detection-test-2'
	`).Scan(&lastSeen); err != nil {
		t.Fatalf("query last_seen_at: %v", err)
	}
	if !lastSeen.Equal(detection.EvaluatedAt) {
		t.Fatalf("last_seen_at = %v, want %v", lastSeen, detection.EvaluatedAt)
	}
}

func TestDetectFromAlertsConcurrentEvaluationsCreateOneCase(t *testing.T) {
	store, ctx := detectionTestDB(t)
	syncDetectionTestCluster(t, store, ctx)

	const workers = 4
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func() {
			_, err := store.DetectFromAlerts(context.Background(), AlertDetection{
				Provider:    "detection-test",
				EvaluatedAt: time.Date(2026, 8, 14, 9, 20, 0, 0, time.UTC),
				Candidates: []AlertCandidate{
					detectionCandidate("detection-test-3", "detection-test-cluster", "firing"),
				},
			})
			errs <- err
		}()
	}
	for i := 0; i < workers; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent DetectFromAlerts: %v", err)
		}
	}

	var caseCount int
	if err := store.db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM cases
		JOIN case_alert_dedup ON case_alert_dedup.case_id = cases.id
		WHERE case_alert_dedup.fingerprint = 'detection-test-3'
	`).Scan(&caseCount); err != nil {
		t.Fatalf("count cases for fingerprint: %v", err)
	}
	if caseCount != 1 {
		t.Fatalf("case count after concurrent evaluations = %d, want 1", caseCount)
	}
}

func TestDetectFromAlertsReopensAfterClose(t *testing.T) {
	store, ctx := detectionTestDB(t)
	syncDetectionTestCluster(t, store, ctx)

	candidate := detectionCandidate("detection-test-4", "detection-test-cluster", "firing")
	detection := AlertDetection{
		Provider:    "detection-test",
		EvaluatedAt: time.Date(2026, 8, 14, 9, 20, 0, 0, time.UTC),
		Candidates:  []AlertCandidate{candidate},
	}
	if _, err := store.DetectFromAlerts(ctx, detection); err != nil {
		t.Fatalf("first DetectFromAlerts: %v", err)
	}

	if _, err := store.db.ExecContext(ctx, `
		UPDATE cases
		SET status = 'closed', closed_at = now(), updated_at = now()
		WHERE id = (SELECT case_id FROM case_alert_dedup WHERE fingerprint = 'detection-test-4')
	`); err != nil {
		t.Fatalf("close detected case: %v", err)
	}

	detection.EvaluatedAt = time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	result, err := store.DetectFromAlerts(ctx, detection)
	if err != nil {
		t.Fatalf("second DetectFromAlerts: %v", err)
	}
	if result.CasesCreated != 1 {
		t.Fatalf("reopen created %d cases, want 1", result.CasesCreated)
	}

	var openCount int
	if err := store.db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM cases
		JOIN case_alert_dedup ON case_alert_dedup.case_id = cases.id
		WHERE case_alert_dedup.fingerprint = 'detection-test-4'
			AND cases.status <> 'closed'
	`).Scan(&openCount); err != nil {
		t.Fatalf("count open cases: %v", err)
	}
	if openCount != 1 {
		t.Fatalf("open case count = %d, want 1", openCount)
	}
}

func TestDetectFromAlertsRecordsResolutionWithoutClosing(t *testing.T) {
	store, ctx := detectionTestDB(t)
	syncDetectionTestCluster(t, store, ctx)

	firing := AlertDetection{
		Provider:    "detection-test",
		EvaluatedAt: time.Date(2026, 8, 14, 9, 20, 0, 0, time.UTC),
		Candidates: []AlertCandidate{
			detectionCandidate("detection-test-5", "detection-test-cluster", "firing"),
		},
	}
	if _, err := store.DetectFromAlerts(ctx, firing); err != nil {
		t.Fatalf("firing DetectFromAlerts: %v", err)
	}

	resolved := AlertDetection{
		Provider:    "detection-test",
		EvaluatedAt: time.Date(2026, 8, 14, 9, 30, 0, 0, time.UTC),
		Candidates: []AlertCandidate{
			detectionCandidate("detection-test-5", "detection-test-cluster", "resolved"),
		},
	}
	if _, err := store.DetectFromAlerts(ctx, resolved); err != nil {
		t.Fatalf("resolved DetectFromAlerts: %v", err)
	}

	var dedupState, caseStatus string
	if err := store.db.QueryRowContext(ctx, `
		SELECT dedup.state, cases.status
		FROM case_alert_dedup AS dedup
		JOIN cases ON cases.id = dedup.case_id
		WHERE dedup.fingerprint = 'detection-test-5'
	`).Scan(&dedupState, &caseStatus); err != nil {
		t.Fatalf("query dedup and case state: %v", err)
	}
	if dedupState != "resolved" {
		t.Fatalf("dedup state = %q, want resolved", dedupState)
	}
	if caseStatus != "detected" {
		t.Fatalf("case status = %q, want still detected (no auto-close)", caseStatus)
	}
}

func TestDetectFromAlertsWithUnresolvedClusterLabelCreatesCaseWithoutCluster(t *testing.T) {
	store, ctx := detectionTestDB(t)

	_, err := store.DetectFromAlerts(ctx, AlertDetection{
		Provider:    "detection-test",
		EvaluatedAt: time.Date(2026, 8, 14, 9, 20, 0, 0, time.UTC),
		Candidates: []AlertCandidate{
			detectionCandidate("detection-test-6", "no-such-cluster-label", "firing"),
		},
	})
	if err != nil {
		t.Fatalf("DetectFromAlerts: %v", err)
	}

	var payload map[string]any
	var payloadBytes []byte
	if err := store.db.QueryRowContext(ctx, `
		SELECT payload
		FROM cases
		JOIN case_timeline_events ON case_timeline_events.case_id = cases.id
		WHERE cases.title = 'detection-test CephOSDDown on osd=1'
			AND cases.cluster_fsid IS NULL
	`).Scan(&payloadBytes); err != nil {
		t.Fatalf("query unresolved-label case: %v", err)
	}
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		t.Fatalf("parse timeline payload: %v", err)
	}
	if payload["clusterFsid"] != nil {
		t.Fatalf("timeline payload = %+v, want no clusterFsid", payload)
	}
	if payload["osd"] != nil || payload["host"] != nil {
		t.Fatalf("timeline payload = %+v, want no osd or host context without a resolved cluster", payload)
	}
}

func TestAlertEvaluationRunLifecycle(t *testing.T) {
	store, ctx := detectionTestDB(t)

	runID, err := store.BeginAlertEvaluationRun(ctx, BeginEvaluationRun{Provider: "fake", Scenario: "osd-down-alert"})
	if err != nil {
		t.Fatalf("BeginAlertEvaluationRun: %v", err)
	}
	if err := store.SucceedAlertEvaluationRun(ctx, EvaluationRunResult{RunID: runID, AlertsEvaluated: 3, CasesCreated: 1}); err != nil {
		t.Fatalf("SucceedAlertEvaluationRun: %v", err)
	}

	failID, err := store.BeginAlertEvaluationRun(ctx, BeginEvaluationRun{Provider: "fake"})
	if err != nil {
		t.Fatalf("BeginAlertEvaluationRun (failure): %v", err)
	}
	if err := store.FailAlertEvaluationRun(ctx, EvaluationRunFailure{RunID: failID, ErrorClass: "Unavailable", ErrorMessage: "simulated"}); err != nil {
		t.Fatalf("FailAlertEvaluationRun: %v", err)
	}

	runs, err := store.ListAlertEvaluationRuns(ctx, 10)
	if err != nil {
		t.Fatalf("ListAlertEvaluationRuns: %v", err)
	}
	var succeeded, failed *AlertEvaluationRun
	for i, run := range runs {
		switch run.ID {
		case runID:
			succeeded = &runs[i]
		case failID:
			failed = &runs[i]
		}
	}
	if succeeded == nil || succeeded.Status != "succeeded" {
		t.Fatalf("succeeded run = %+v, want status succeeded", succeeded)
	}
	if succeeded.AlertsEvaluated == nil || *succeeded.AlertsEvaluated != 3 {
		t.Fatalf("succeeded alerts evaluated = %+v, want 3", succeeded.AlertsEvaluated)
	}
	if succeeded.CasesCreated == nil || *succeeded.CasesCreated != 1 {
		t.Fatalf("succeeded cases created = %+v, want 1", succeeded.CasesCreated)
	}
	if failed == nil || failed.Status != "failed" || failed.ErrorClass != "Unavailable" {
		t.Fatalf("failed run = %+v, want failed Unavailable", failed)
	}

	if err := store.SucceedAlertEvaluationRun(ctx, EvaluationRunResult{RunID: runID}); err == nil {
		t.Fatal("expected error succeeding an already-finished run")
	}
}

func TestDetectFromAlertsRejectsInvalidDetections(t *testing.T) {
	store, ctx := detectionTestDB(t)

	if _, err := store.DetectFromAlerts(ctx, AlertDetection{EvaluatedAt: time.Now()}); err == nil {
		t.Fatal("expected error for missing provider")
	}
	if _, err := store.DetectFromAlerts(ctx, AlertDetection{Provider: "detection-test"}); err == nil {
		t.Fatal("expected error for missing evaluated_at")
	}
	missingName := detectionCandidate("detection-test-7", "", "firing")
	missingName.Name = ""
	if _, err := store.DetectFromAlerts(ctx, AlertDetection{
		Provider:    "detection-test",
		EvaluatedAt: time.Now(),
		Candidates:  []AlertCandidate{missingName},
	}); err == nil {
		t.Fatal("expected error for missing candidate name")
	}
	if _, err := store.DetectFromAlerts(ctx, AlertDetection{
		Provider:    "detection-test",
		EvaluatedAt: time.Now(),
		Candidates: []AlertCandidate{
			detectionCandidate("detection-test-8", "", "exploded"),
		},
	}); err == nil {
		t.Fatal("expected error for unknown candidate state")
	}
}

package casedetection

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tonymontoya/ceph-atlas/internal/fleet"
	"github.com/tonymontoya/ceph-atlas/internal/inventory"
	"github.com/tonymontoya/ceph-atlas/internal/observability"
	"github.com/tonymontoya/ceph-atlas/internal/store"
	"github.com/tonymontoya/ceph-atlas/internal/testdb"
)

func TestRunFakeOnceDetectsCaseFromAlerts(t *testing.T) {
	ctx := context.Background()
	db, _ := testdb.Open(t)

	const fsid = "00000000-0000-4000-8000-000000000102"
	fingerprint := string(fixtureFingerprint(t))
	cleanupDetection(t, db, fsid)
	t.Cleanup(func() { cleanupDetection(t, db, fsid) })

	writer := store.NewPostgres(db)
	if _, err := writer.SaveInventoryObservation(ctx, store.InventoryObservation{
		Provider:   "fake",
		Scenario:   "reef-osd-down-baremetal",
		ObservedAt: time.Date(2026, 8, 14, 9, 0, 0, 0, time.UTC),
		Cluster:    fixtureClusterIdentity(t),
		Health:     fixtureHealth(t),
		OSDs:       fixtureOSDs(t),
	}); err != nil {
		t.Fatalf("save inventory observation: %v", err)
	}

	result, err := RunFakeOnce(ctx, writer, Options{
		Scenario:    "osd-down-alert",
		EvaluatedAt: time.Date(2026, 8, 14, 9, 20, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("RunFakeOnce returned error: %v", err)
	}
	if result.AlertsEvaluated != 1 || result.CasesCreated != 1 {
		t.Fatalf("result = %+v, want 1 evaluated and 1 case created", result)
	}

	var title, status, severity, clusterFSID string
	var detectedCount int
	if err := db.QueryRowContext(ctx, `
		SELECT cases.title, cases.status, cases.severity, cases.cluster_fsid::text, count(*)
		FROM cases
		JOIN case_alert_dedup ON case_alert_dedup.case_id = cases.id
		WHERE case_alert_dedup.fingerprint = $1
		GROUP BY cases.id, cases.title, cases.status, cases.severity, cases.cluster_fsid
	`, fingerprint).Scan(&title, &status, &severity, &clusterFSID, &detectedCount); err != nil {
		t.Fatalf("query detected case: %v", err)
	}
	if detectedCount != 1 {
		t.Fatalf("detected case count = %d, want 1", detectedCount)
	}
	if title != "CephOSDDown on osd=1" || status != "detected" || severity != "high" {
		t.Fatalf("detected case title/status/severity = %q/%q/%q", title, status, severity)
	}
	if clusterFSID != fsid {
		t.Fatalf("detected case cluster fsid = %q, want %q", clusterFSID, fsid)
	}

	var eventType string
	var payloadBytes []byte
	if err := db.QueryRowContext(ctx, `
		SELECT event_type, payload
		FROM case_timeline_events
		WHERE case_id = (SELECT case_id FROM case_alert_dedup WHERE fingerprint = $1)
	`, fingerprint).Scan(&eventType, &payloadBytes); err != nil {
		t.Fatalf("query timeline event: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		t.Fatalf("parse timeline payload: %v", err)
	}
	if eventType != "case_detected" || payload["signal"] != "CEPH_OSD_DOWN" {
		t.Fatalf("timeline event = %s %+v, want case_detected with signal", eventType, payload)
	}
	if payload["osd"] != float64(1) || payload["host"] != "host-b.example.invalid" {
		t.Fatalf("timeline payload = %+v, want osd and host context from the synced read model", payload)
	}

	var runStatus string
	var alertsEvaluated, casesCreated int
	if err := db.QueryRowContext(ctx, `
		SELECT status, alerts_evaluated, cases_created
		FROM alert_evaluation_runs
		WHERE provider = 'fake' AND status = 'succeeded'
		ORDER BY id DESC
		LIMIT 1
	`).Scan(&runStatus, &alertsEvaluated, &casesCreated); err != nil {
		t.Fatalf("query evaluation run: %v", err)
	}
	if runStatus != "succeeded" || alertsEvaluated != 1 || casesCreated != 1 {
		t.Fatalf("evaluation run = %s %d/%d, want succeeded 1/1", runStatus, alertsEvaluated, casesCreated)
	}

	second, err := RunFakeOnce(ctx, writer, Options{
		Scenario:    "osd-down-alert",
		EvaluatedAt: time.Date(2026, 8, 14, 9, 25, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("second RunFakeOnce returned error: %v", err)
	}
	if second.CasesCreated != 0 {
		t.Fatalf("second run created %d cases, want 0 (idempotent)", second.CasesCreated)
	}
}

func TestRunFakeOnceRecordsProviderErrorClassOnFailure(t *testing.T) {
	ctx := context.Background()
	db, _ := testdb.Open(t)
	cleanupDetectionRuns(t, db)
	t.Cleanup(func() { cleanupDetectionRuns(t, db) })

	_, err := RunFakeOnce(ctx, store.NewPostgres(db), Options{Scenario: "provider-unauthorized"})
	if err == nil {
		t.Fatal("expected provider error from unauthorized scenario")
	}

	var errorClass string
	if err := db.QueryRowContext(ctx, `
		SELECT error_class
		FROM alert_evaluation_runs
		WHERE provider = 'fake' AND status = 'failed'
		ORDER BY id DESC
		LIMIT 1
	`).Scan(&errorClass); err != nil {
		t.Fatalf("query failed evaluation run: %v", err)
	}
	if errorClass != "Unauthorized" {
		t.Fatalf("failed run error class = %q, want Unauthorized", errorClass)
	}
}

func fixtureFingerprint(t *testing.T) observability.Fingerprint {
	t.Helper()
	var alerts []observability.Alert
	loadFixture(t, "prometheus", "osd-down-alert", "alerts.json", &alerts)
	if len(alerts) != 1 {
		t.Fatalf("alerts fixture holds %d alerts, want 1", len(alerts))
	}
	return observability.DeriveFingerprint(alerts[0])
}

func fixtureClusterIdentity(t *testing.T) fleet.ClusterIdentity {
	t.Helper()
	var identity fleet.ClusterIdentity
	loadFixture(t, "ceph", "reef-osd-down-baremetal", "cluster_identity.json", &identity)
	return identity
}

func fixtureHealth(t *testing.T) inventory.Health {
	t.Helper()
	var health inventory.Health
	loadFixture(t, "ceph", "reef-osd-down-baremetal", "health.json", &health)
	return health
}

func fixtureOSDs(t *testing.T) []inventory.OSD {
	t.Helper()
	var osds []inventory.OSD
	loadFixture(t, "ceph", "reef-osd-down-baremetal", "osds.json", &osds)
	return osds
}

func loadFixture(t *testing.T, family, scenario, name string, target any) {
	t.Helper()
	path := filepath.Join("..", "..", "dev", "fixtures", family, scenario, name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("parse fixture %s: %v", path, err)
	}
}

func cleanupDetection(t *testing.T, db *sql.DB, fsid string) {
	t.Helper()
	testdb.DeleteCases(t, db, "title = 'CephOSDDown on osd=1'")
	testdb.DeleteAlertRuns(t, db, "provider = 'fake'")
	testdb.DeleteSyncRuns(t, db, "provider = 'fake'")
	testdb.DeleteClusters(t, db, "fsid = $1", fsid)
}

func cleanupDetectionRuns(t *testing.T, db *sql.DB) {
	t.Helper()
	testdb.DeleteAlertRuns(t, db, "provider = 'fake'")
}

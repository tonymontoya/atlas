package inventorysync

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/tonymontoya/ceph-atlas/internal/store"
)

func TestRunOncePersistsFakeProviderObservationToPostgres(t *testing.T) {
	databaseURL := os.Getenv("ATLAS_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set ATLAS_TEST_DATABASE_URL to run PostgreSQL integration test")
	}

	ctx := context.Background()
	db, err := store.OpenPostgres(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer db.Close()

	const fsid = "00000000-0000-4000-8000-000000000102"
	if _, err := db.ExecContext(ctx, `DELETE FROM inventory_sync_runs WHERE provider = 'fake'`); err != nil {
		t.Fatalf("delete existing test sync runs: %v", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM atlas_clusters WHERE fsid = $1`, fsid); err != nil {
		t.Fatalf("delete existing test cluster: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM inventory_sync_runs WHERE provider = 'fake'`)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM atlas_clusters WHERE fsid = $1`, fsid)
	})

	writer := store.NewPostgres(db)
	result, err := RunFakeOnce(ctx, writer, Options{
		Scenario:   "reef-osd-down-baremetal",
		ObservedAt: time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if result.ClusterID == 0 || result.SnapshotID == 0 {
		t.Fatalf("result = %+v, want non-zero ids", result)
	}

	var healthStatus string
	var checkCount int
	if err := db.QueryRowContext(ctx, `
		SELECT status, jsonb_array_length(checks)
		FROM cluster_current_health
		WHERE fsid = $1
	`, fsid).Scan(&healthStatus, &checkCount); err != nil {
		t.Fatalf("query current health: %v", err)
	}
	if healthStatus != "HEALTH_WARN" {
		t.Fatalf("health status = %q, want HEALTH_WARN", healthStatus)
	}
	if checkCount != 1 {
		t.Fatalf("health check count = %d, want 1", checkCount)
	}

	var osdCount int
	var hasDownOSD bool
	if err := db.QueryRowContext(ctx, `
		SELECT count(*), bool_or(NOT osd_up)
		FROM cluster_current_osds
		WHERE fsid = $1
	`, fsid).Scan(&osdCount, &hasDownOSD); err != nil {
		t.Fatalf("query current osds: %v", err)
	}
	if osdCount != 2 {
		t.Fatalf("OSD count = %d, want 2", osdCount)
	}
	if !hasDownOSD {
		t.Fatal("expected one down OSD")
	}

	var runStatus string
	var runSnapshotID int64
	if err := db.QueryRowContext(ctx, `
		SELECT status, snapshot_id
		FROM inventory_sync_runs
		WHERE provider = 'fake' AND scenario = 'reef-osd-down-baremetal'
		ORDER BY started_at DESC, id DESC
		LIMIT 1
	`).Scan(&runStatus, &runSnapshotID); err != nil {
		t.Fatalf("query sync run: %v", err)
	}
	if runStatus != "succeeded" {
		t.Fatalf("sync run status = %q, want succeeded", runStatus)
	}
	if runSnapshotID != result.SnapshotID {
		t.Fatalf("sync run snapshot = %d, want %d", runSnapshotID, result.SnapshotID)
	}
}

func TestRunOnceRecordsFailedSyncRun(t *testing.T) {
	databaseURL := os.Getenv("ATLAS_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set ATLAS_TEST_DATABASE_URL to run PostgreSQL integration test")
	}

	ctx := context.Background()
	db, err := store.OpenPostgres(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer db.Close()

	if _, err := db.ExecContext(ctx, `DELETE FROM inventory_sync_runs WHERE provider = 'fake' AND scenario = 'missing'`); err != nil {
		t.Fatalf("delete existing failed sync runs: %v", err)
	}

	writer := store.NewPostgres(db)
	_, err = RunFakeOnce(ctx, writer, Options{
		Scenario:   "missing",
		ObservedAt: time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC),
	})
	if err == nil {
		t.Fatal("expected error")
	}

	var status string
	var errorClass string
	var errorMessage string
	if err := db.QueryRowContext(ctx, `
		SELECT status, error_class, error_message
		FROM inventory_sync_runs
		WHERE provider = 'fake' AND scenario = 'missing'
		ORDER BY started_at DESC, id DESC
		LIMIT 1
	`).Scan(&status, &errorClass, &errorMessage); err != nil {
		t.Fatalf("query failed sync run: %v", err)
	}
	if status != "failed" {
		t.Fatalf("sync run status = %q, want failed", status)
	}
	if errorClass != "Unavailable" {
		t.Fatalf("error class = %q, want Unavailable", errorClass)
	}
	if errorMessage == "" {
		t.Fatal("expected sync run error message")
	}
}

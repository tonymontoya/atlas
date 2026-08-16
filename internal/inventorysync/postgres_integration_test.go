package inventorysync

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/tonymontoya/ceph-atlas/internal/fleet"
	"github.com/tonymontoya/ceph-atlas/internal/inventory"
	"github.com/tonymontoya/ceph-atlas/internal/providers/ceph"
	"github.com/tonymontoya/ceph-atlas/internal/providers/ceph/dashtest"
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
	defer func() { _ = db.Close() }()

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

	var hostCount int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM cluster_current_hosts
		WHERE fsid = $1
	`, fsid).Scan(&hostCount); err != nil {
		t.Fatalf("query current hosts: %v", err)
	}
	if hostCount != 3 {
		t.Fatalf("Host count = %d, want 3", hostCount)
	}

	var deviceCount int
	var hasErrorDevice bool
	if err := db.QueryRowContext(ctx, `
		SELECT count(*), bool_or(device_health = 'error')
		FROM cluster_current_storage_devices
		WHERE fsid = $1
	`, fsid).Scan(&deviceCount, &hasErrorDevice); err != nil {
		t.Fatalf("query current storage devices: %v", err)
	}
	if deviceCount != 3 {
		t.Fatalf("Storage Device count = %d, want 3", deviceCount)
	}
	if !hasErrorDevice {
		t.Fatal("expected one Storage Device with error health")
	}

	var daemonCount int
	var stoppedDaemons int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*), count(*) FILTER (WHERE status = 'stopped')
		FROM cluster_current_daemons
		WHERE fsid = $1
	`, fsid).Scan(&daemonCount, &stoppedDaemons); err != nil {
		t.Fatalf("query current daemons: %v", err)
	}
	if daemonCount != 7 {
		t.Fatalf("Ceph Daemon count = %d, want 7", daemonCount)
	}
	if stoppedDaemons != 1 {
		t.Fatalf("stopped Ceph Daemon count = %d, want 1", stoppedDaemons)
	}

	var poolCount int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM cluster_current_pools
		WHERE fsid = $1
	`, fsid).Scan(&poolCount); err != nil {
		t.Fatalf("query current pools: %v", err)
	}
	if poolCount != 2 {
		t.Fatalf("Pool count = %d, want 2", poolCount)
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

func TestRunOncePersistsCephProviderObservationToPostgres(t *testing.T) {
	databaseURL := os.Getenv("ATLAS_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set ATLAS_TEST_DATABASE_URL to run PostgreSQL integration test")
	}

	dashboard := dashtest.New(t, dashtest.ModeSuccess)
	provider, err := ceph.New(ceph.Config{
		BaseURL:  dashboard.URL(),
		Username: dashtest.Username,
		Password: dashtest.Password,
	})
	if err != nil {
		t.Fatalf("ceph.New returned error: %v", err)
	}

	ctx := context.Background()
	db, err := store.OpenPostgres(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer func() { _ = db.Close() }()

	if _, err := db.ExecContext(ctx, `DELETE FROM inventory_sync_runs WHERE provider = 'ceph'`); err != nil {
		t.Fatalf("delete existing test sync runs: %v", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM atlas_clusters WHERE fsid = $1`, dashtest.FSID); err != nil {
		t.Fatalf("delete existing test cluster: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM inventory_sync_runs WHERE provider = 'ceph'`)
		_, _ = db.ExecContext(context.Background(), `DELETE FROM atlas_clusters WHERE fsid = $1`, dashtest.FSID)
	})

	writer := store.NewPostgres(db)
	result, err := RunOnce(ctx, writer, provider, Options{
		ProviderName: "ceph",
		ObservedAt:   time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC),
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
	`, dashtest.FSID).Scan(&healthStatus, &checkCount); err != nil {
		t.Fatalf("query current health: %v", err)
	}
	if healthStatus != "HEALTH_OK" {
		t.Fatalf("health status = %q, want HEALTH_OK", healthStatus)
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
	`, dashtest.FSID).Scan(&osdCount, &hasDownOSD); err != nil {
		t.Fatalf("query current osds: %v", err)
	}
	if osdCount != 3 {
		t.Fatalf("OSD count = %d, want 3", osdCount)
	}
	if !hasDownOSD {
		t.Fatal("expected one down OSD")
	}

	var deviceCount int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM cluster_current_storage_devices
		WHERE fsid = $1
	`, dashtest.FSID).Scan(&deviceCount); err != nil {
		t.Fatalf("query current storage devices: %v", err)
	}
	if deviceCount != 3 {
		t.Fatalf("Storage Device count = %d, want 3", deviceCount)
	}

	var daemonCount int
	var stoppedDaemons int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*), count(*) FILTER (WHERE status = 'stopped')
		FROM cluster_current_daemons
		WHERE fsid = $1
	`, dashtest.FSID).Scan(&daemonCount, &stoppedDaemons); err != nil {
		t.Fatalf("query current daemons: %v", err)
	}
	if daemonCount != 5 {
		t.Fatalf("Ceph Daemon count = %d, want 5", daemonCount)
	}
	if stoppedDaemons != 1 {
		t.Fatalf("stopped Ceph Daemon count = %d, want 1", stoppedDaemons)
	}

	var poolCount int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM cluster_current_pools
		WHERE fsid = $1
	`, dashtest.FSID).Scan(&poolCount); err != nil {
		t.Fatalf("query current pools: %v", err)
	}
	if poolCount != 2 {
		t.Fatalf("Pool count = %d, want 2", poolCount)
	}

	var runStatus string
	var runSnapshotID int64
	if err := db.QueryRowContext(ctx, `
		SELECT status, snapshot_id
		FROM inventory_sync_runs
		WHERE provider = 'ceph' AND scenario IS NULL
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

func TestSaveInventoryObservationPreservesDeviceOSDHistory(t *testing.T) {
	databaseURL := os.Getenv("ATLAS_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set ATLAS_TEST_DATABASE_URL to run PostgreSQL integration test")
	}

	ctx := context.Background()
	db, err := store.OpenPostgres(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer func() { _ = db.Close() }()

	const fsid = "00000000-0000-4000-8000-000000000201"
	if _, err := db.ExecContext(ctx, `DELETE FROM atlas_clusters WHERE fsid = $1`, fsid); err != nil {
		t.Fatalf("delete existing test cluster: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM atlas_clusters WHERE fsid = $1`, fsid)
	})

	cluster := fleet.ClusterIdentity{
		FSID:        fsid,
		Name:        "device-history-test",
		CephVersion: "18.2.x",
		Type:        fleet.ClusterTypeBareMetal,
	}
	base := store.InventoryObservation{
		Provider: "fake",
		Scenario: "device-history-test",
		Cluster:  cluster,
		Health:   inventory.Health{Status: inventory.HealthOK},
		Hosts:    []inventory.Host{{Name: "host-h.example.invalid"}},
		Daemons:  []inventory.Daemon{{Type: "mon", Name: "mon.a", Host: "host-h.example.invalid", Status: "running"}},
		Pools:    []inventory.Pool{{ID: 1, Name: ".mgr", Type: "replicated"}},
	}

	firstOSD := 3
	secondOSD := 7
	first := base
	first.ObservedAt = time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	first.OSDs = []inventory.OSD{{ID: firstOSD, Host: "host-h.example.invalid", Up: true, In: true}}
	first.Devices = []inventory.StorageDevice{{Host: "host-h.example.invalid", Serial: "nvme-serial-h", OSDID: &firstOSD}}
	if _, err := store.NewPostgres(db).SaveInventoryObservation(ctx, first); err != nil {
		t.Fatalf("save first observation: %v", err)
	}

	second := base
	second.ObservedAt = time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	second.OSDs = []inventory.OSD{{ID: secondOSD, Host: "host-h.example.invalid", Up: true, In: true}}
	second.Devices = []inventory.StorageDevice{{Host: "host-h.example.invalid", Serial: "nvme-serial-h", OSDID: &secondOSD}}
	if _, err := store.NewPostgres(db).SaveInventoryObservation(ctx, second); err != nil {
		t.Fatalf("save second observation: %v", err)
	}

	rows, err := db.QueryContext(ctx, `
		SELECT osd_id, first_observed_at, last_observed_at
		FROM storage_device_osd_history
		WHERE fsid = $1 AND serial = 'nvme-serial-h'
		ORDER BY osd_id
	`, fsid)
	if err != nil {
		t.Fatalf("query device OSD history: %v", err)
	}
	defer func() { _ = rows.Close() }()

	type link struct {
		osdID         int
		firstObserved time.Time
		lastObserved  time.Time
	}
	var links []link
	for rows.Next() {
		var l link
		if err := rows.Scan(&l.osdID, &l.firstObserved, &l.lastObserved); err != nil {
			t.Fatalf("scan device OSD history: %v", err)
		}
		links = append(links, l)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate device OSD history: %v", err)
	}
	if len(links) != 2 {
		t.Fatalf("history link count = %d, want 2 (OSD 3 then OSD 7 on one device)", len(links))
	}
	if links[0].osdID != firstOSD || links[1].osdID != secondOSD {
		t.Fatalf("history OSD IDs = [%d, %d], want [%d, %d]", links[0].osdID, links[1].osdID, firstOSD, secondOSD)
	}
	if !links[0].firstObserved.Equal(first.ObservedAt) || !links[0].lastObserved.Equal(first.ObservedAt) {
		t.Fatalf("first link observed range = [%s, %s], want [%s, %s]", links[0].firstObserved, links[0].lastObserved, first.ObservedAt, first.ObservedAt)
	}
	if !links[1].firstObserved.Equal(second.ObservedAt) || !links[1].lastObserved.Equal(second.ObservedAt) {
		t.Fatalf("second link observed range = [%s, %s], want [%s, %s]", links[1].firstObserved, links[1].lastObserved, second.ObservedAt, second.ObservedAt)
	}
}

func TestCurrentViewsReflectOnlyLatestSnapshot(t *testing.T) {
	databaseURL := os.Getenv("ATLAS_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set ATLAS_TEST_DATABASE_URL to run PostgreSQL integration test")
	}

	ctx := context.Background()
	db, err := store.OpenPostgres(ctx, databaseURL)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer func() { _ = db.Close() }()

	const fsid = "00000000-0000-4000-8000-000000000202"
	if _, err := db.ExecContext(ctx, `DELETE FROM atlas_clusters WHERE fsid = $1`, fsid); err != nil {
		t.Fatalf("delete existing test cluster: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM atlas_clusters WHERE fsid = $1`, fsid)
	})

	cluster := fleet.ClusterIdentity{
		FSID:        fsid,
		Name:        "removal-test",
		CephVersion: "18.2.x",
		Type:        fleet.ClusterTypeBareMetal,
	}
	base := store.InventoryObservation{
		Provider: "fake",
		Scenario: "removal-test",
		Cluster:  cluster,
		Health:   inventory.Health{Status: inventory.HealthOK},
	}

	hostA := "host-a.example.invalid"
	hostB := "host-b.example.invalid"
	first := base
	first.ObservedAt = time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	first.Hosts = []inventory.Host{{Name: hostA}, {Name: hostB}}
	first.Devices = []inventory.StorageDevice{
		{Host: hostA, Serial: "serial-a"},
		{Host: hostB, Serial: "serial-b"},
	}
	first.Daemons = []inventory.Daemon{
		{Type: "mon", Name: "mon.a", Host: hostA, Status: "running"},
		{Type: "mon", Name: "mon.b", Host: hostB, Status: "running"},
	}
	first.Pools = []inventory.Pool{{ID: 1, Name: "pool-one", Type: "replicated"}}
	first.OSDs = []inventory.OSD{{ID: 0, Host: hostA, Up: true, In: true}}
	if _, err := store.NewPostgres(db).SaveInventoryObservation(ctx, first); err != nil {
		t.Fatalf("save first observation: %v", err)
	}

	// Second snapshot: hostB, its device, mon.b, the pool, and osd.0 are gone.
	second := base
	second.ObservedAt = time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	second.Hosts = []inventory.Host{{Name: hostA}}
	second.Devices = []inventory.StorageDevice{{Host: hostA, Serial: "serial-a"}}
	second.Daemons = []inventory.Daemon{
		{Type: "mon", Name: "mon.a", Host: hostA, Status: "running"},
	}
	second.Pools = nil
	second.OSDs = nil
	if _, err := store.NewPostgres(db).SaveInventoryObservation(ctx, second); err != nil {
		t.Fatalf("save second observation: %v", err)
	}

	for _, check := range []struct {
		view  string
		query string
	}{
		{"cluster_current_hosts", `SELECT count(*) FROM cluster_current_hosts WHERE fsid = $1 AND host_name = '` + hostB + `'`},
		{"cluster_current_storage_devices", `SELECT count(*) FROM cluster_current_storage_devices WHERE fsid = $1 AND host_name = '` + hostB + `'`},
		{"cluster_current_daemons", `SELECT count(*) FROM cluster_current_daemons WHERE fsid = $1 AND daemon_name = 'mon.b'`},
		{"cluster_current_pools", `SELECT count(*) FROM cluster_current_pools WHERE fsid = $1`},
		{"cluster_current_osds", `SELECT count(*) FROM cluster_current_osds WHERE fsid = $1`},
	} {
		var remaining int
		if err := db.QueryRowContext(ctx, check.query, fsid).Scan(&remaining); err != nil {
			t.Fatalf("query %s: %v", check.view, err)
		}
		if remaining != 0 {
			t.Fatalf("%s still lists %d row(s) absent from the latest snapshot; current views must reflect only the latest snapshot", check.view, remaining)
		}
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
	defer func() { _ = db.Close() }()

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

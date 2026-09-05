package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tonymontoya/ceph-atlas/internal/apperr"
	"github.com/tonymontoya/ceph-atlas/internal/fleet"
	"github.com/tonymontoya/ceph-atlas/internal/inventory"
	"github.com/tonymontoya/ceph-atlas/internal/testdb"
)

const deregisteredLivenessFSID = "00000000-0000-4000-8000-000000000a01"

func cleanupDeregisteredLivenessRows(t *testing.T, db *sql.DB) {
	t.Helper()
	testdb.DeleteCases(t, db, "cluster_fsid = $1::uuid", deregisteredLivenessFSID)
	// The live holder matches by fsid; after a transfer the dead row
	// carries fsid NULL, so both row names are swept as well.
	testdb.DeleteClusters(t, db, "fsid = $1::uuid", deregisteredLivenessFSID)
	testdb.DeleteClusters(t, db, "name IN ('deregistered-liveness', 'deregistered-liveness-fresh')")
}

// seedDeregisteredCluster persists one fully-populated cluster and then
// deregisters it, leaving its snapshots, cases, and sync runs behind —
// the state a retired registration keeps (ADR-0026 amendment 2026-09-05).
func seedDeregisteredCluster(t *testing.T, store *PostgresStore, db *sql.DB) int64 {
	t.Helper()
	ctx := context.Background()
	obs := InventoryObservation{
		Provider:   "fake",
		ObservedAt: time.Date(2026, 9, 5, 9, 0, 0, 0, time.UTC),
		Cluster: fleet.ClusterIdentity{
			FSID:        deregisteredLivenessFSID,
			Name:        "deregistered-liveness",
			CephVersion: "18.2.x",
			Type:        fleet.ClusterTypeBareMetal,
		},
		Health: inventory.Health{Status: inventory.HealthOK, Summary: "healthy"},
		OSDs:   []inventory.OSD{{ID: 30, Host: "host-dl.example.invalid", Up: true, In: true}},
		Hosts:  []inventory.Host{{Name: "host-dl.example.invalid"}},
		Devices: []inventory.StorageDevice{
			{Host: "host-dl.example.invalid", Serial: "serial-dl", OSDID: ptrInt(30)},
		},
		Daemons: []inventory.Daemon{{Type: inventory.DaemonMon, Name: "mon.a", Host: "host-dl.example.invalid", Status: inventory.DaemonRunning}},
		Pools:   []inventory.Pool{{ID: 3, Name: "pool-dl", Type: "replicated"}},
	}
	saved, err := store.SaveInventoryObservation(ctx, obs)
	if err != nil {
		t.Fatalf("seed observation: %v", err)
	}
	if _, err := store.CreateManualCase(ctx, ManualCaseInput{
		Title:       "deregistered-liveness case",
		Summary:     "Case history that must stay queryable.",
		Severity:    "low",
		ClusterFSID: deregisteredLivenessFSID,
		Actor:       registrationActor(),
	}); err != nil {
		t.Fatalf("seed case: %v", err)
	}
	runID, err := store.BeginInventorySyncRun(ctx, BeginSyncRun{Provider: "fake", ClusterID: &saved.ClusterID})
	if err != nil {
		t.Fatalf("begin sync run: %v", err)
	}
	if err := store.SucceedInventorySyncRun(ctx, SyncRunResult{RunID: runID, SnapshotID: saved.SnapshotID, ClusterID: saved.ClusterID}); err != nil {
		t.Fatalf("succeed sync run: %v", err)
	}
	if _, err := store.DeregisterCluster(ctx, DeregisterClusterInput{ClusterID: saved.ClusterID, Actor: registrationActor()}); err != nil {
		t.Fatalf("deregister: %v", err)
	}
	return saved.ClusterID
}

func ptrInt(v int) *int { return &v }

func wantClusterNotFound(t *testing.T, name string, err error) {
	t.Helper()
	var appErr apperr.Error
	if !errors.As(err, &appErr) || appErr.Class != apperr.NotFound || !strings.Contains(appErr.Message, "cluster not found") {
		t.Fatalf("%s: err = %v, want NotFound 'cluster not found'", name, err)
	}
}

func TestScopedReadsServeNothingForDeregisteredCluster(t *testing.T) {
	db, _ := testdb.Open(t)
	store := NewPostgres(db)
	cleanupDeregisteredLivenessRows(t, db)
	t.Cleanup(func() { cleanupDeregisteredLivenessRows(t, db) })
	seedDeregisteredCluster(t, store, db)
	ctx := context.Background()

	if _, err := store.ClusterHealth(ctx, deregisteredLivenessFSID); err == nil {
		t.Fatal("ClusterHealth must not serve a deregistered cluster")
	} else {
		wantClusterNotFound(t, "ClusterHealth", err)
	}
	if _, err := store.ClusterOSDs(ctx, deregisteredLivenessFSID); err == nil {
		t.Fatal("ClusterOSDs must not serve a deregistered cluster")
	} else {
		wantClusterNotFound(t, "ClusterOSDs", err)
	}
	if _, err := store.ClusterHosts(ctx, deregisteredLivenessFSID); err == nil {
		t.Fatal("ClusterHosts must not serve a deregistered cluster")
	} else {
		wantClusterNotFound(t, "ClusterHosts", err)
	}
	if _, err := store.ClusterStorageDevices(ctx, deregisteredLivenessFSID); err == nil {
		t.Fatal("ClusterStorageDevices must not serve a deregistered cluster")
	} else {
		wantClusterNotFound(t, "ClusterStorageDevices", err)
	}
	if _, err := store.ClusterDaemons(ctx, deregisteredLivenessFSID); err == nil {
		t.Fatal("ClusterDaemons must not serve a deregistered cluster")
	} else {
		wantClusterNotFound(t, "ClusterDaemons", err)
	}
	if _, err := store.ClusterPools(ctx, deregisteredLivenessFSID); err == nil {
		t.Fatal("ClusterPools must not serve a deregistered cluster")
	} else {
		wantClusterNotFound(t, "ClusterPools", err)
	}

	index, err := store.ListClusterSummaries(ctx, ListClustersQuery{})
	if err != nil {
		t.Fatalf("ListClusterSummaries: %v", err)
	}
	for _, summary := range index.Clusters {
		if summary.FSID != nil && *summary.FSID == deregisteredLivenessFSID {
			t.Fatal("index must not list a deregistered cluster")
		}
	}
}

func TestRecordFiltersStayQueryableForDeregisteredCluster(t *testing.T) {
	db, _ := testdb.Open(t)
	store := NewPostgres(db)
	cleanupDeregisteredLivenessRows(t, db)
	t.Cleanup(func() { cleanupDeregisteredLivenessRows(t, db) })
	seedDeregisteredCluster(t, store, db)
	ctx := context.Background()

	filteredCases, err := store.ListCases(ctx, 50, deregisteredLivenessFSID)
	if err != nil {
		t.Fatalf("ListCases: %v", err)
	}
	if len(filteredCases) != 1 || filteredCases[0].Title != "deregistered-liveness case" {
		t.Fatalf("filtered cases = %+v, want the deregistered cluster's case to stay queryable", filteredCases)
	}

	filteredRuns, err := store.ListInventorySyncRuns(ctx, 50, deregisteredLivenessFSID)
	if err != nil {
		t.Fatalf("ListInventorySyncRuns: %v", err)
	}
	if len(filteredRuns) != 1 || filteredRuns[0].ClusterFSID == nil || *filteredRuns[0].ClusterFSID != deregisteredLivenessFSID {
		t.Fatalf("filtered runs = %+v, want the deregistered cluster's run to stay queryable", filteredRuns)
	}
}

func TestSaveInventoryObservationRejectsDeregisteredHolder(t *testing.T) {
	db, _ := testdb.Open(t)
	store := NewPostgres(db)
	cleanupDeregisteredLivenessRows(t, db)
	t.Cleanup(func() { cleanupDeregisteredLivenessRows(t, db) })
	seedDeregisteredCluster(t, store, db)
	ctx := context.Background()

	obs := InventoryObservation{
		Provider:   "fake",
		ObservedAt: time.Date(2026, 9, 5, 10, 0, 0, 0, time.UTC),
		Cluster: fleet.ClusterIdentity{
			FSID:        deregisteredLivenessFSID,
			Name:        "deregistered-liveness-refreshed",
			CephVersion: "18.2.y",
			Type:        fleet.ClusterTypeBareMetal,
		},
		Health: inventory.Health{Status: inventory.HealthOK, Summary: "still healthy"},
	}
	_, err := store.SaveInventoryObservation(ctx, obs)
	var appErr apperr.Error
	if !errors.As(err, &appErr) || appErr.Class != apperr.Conflict || !strings.Contains(appErr.Message, "cluster is deregistered") {
		t.Fatalf("save err = %v, want Conflict 'cluster is deregistered'", err)
	}

	var name string
	if err := db.QueryRowContext(ctx, `
		SELECT name FROM atlas_clusters WHERE fsid = $1::uuid
	`, deregisteredLivenessFSID).Scan(&name); err != nil {
		t.Fatalf("query deregistered row: %v", err)
	}
	if name != "deregistered-liveness" {
		t.Fatalf("rejected save must not refresh the dead row's data; name = %q", name)
	}
}

func TestSaveInventoryObservationWorksAfterFreshEnrollmentTransfer(t *testing.T) {
	db, _ := testdb.Open(t)
	store := NewPostgres(db)
	cleanupDeregisteredLivenessRows(t, db)
	t.Cleanup(func() { cleanupDeregisteredLivenessRows(t, db) })
	seedDeregisteredCluster(t, store, db)
	ctx := context.Background()

	// Renewal is re-enrollment (#44): the fresh registration releases the
	// stale FSID claim and binds it, after which the save path works again.
	registration, _ := createTestRegistration(t, store, "deregistered-liveness-fresh")
	if _, err := store.BindClusterFSID(ctx, registration.ID, deregisteredLivenessFSID); err != nil {
		t.Fatalf("bind transferred fsid: %v", err)
	}

	obs := InventoryObservation{
		Provider:   "agent",
		ObservedAt: time.Date(2026, 9, 5, 11, 0, 0, 0, time.UTC),
		Cluster: fleet.ClusterIdentity{
			FSID:        deregisteredLivenessFSID,
			Name:        "deregistered-liveness-fresh",
			CephVersion: "18.2.z",
			Type:        fleet.ClusterTypeBareMetal,
		},
		Health: inventory.Health{Status: inventory.HealthOK, Summary: "healthy again"},
	}
	saved, err := store.SaveInventoryObservation(ctx, obs)
	if err != nil {
		t.Fatalf("save after transfer: %v", err)
	}

	var liveClusterID int64
	if err := db.QueryRowContext(ctx, `
		SELECT id FROM atlas_clusters
		WHERE fsid = $1::uuid AND deregistered_at IS NULL
	`, deregisteredLivenessFSID).Scan(&liveClusterID); err != nil {
		t.Fatalf("query live holder: %v", err)
	}
	if saved.ClusterID != liveClusterID {
		t.Fatalf("save attached to cluster %d, want the fresh live holder %d", saved.ClusterID, liveClusterID)
	}
	if _, err := store.ClusterHealth(ctx, deregisteredLivenessFSID); err != nil {
		t.Fatalf("scoped reads must serve the fresh live holder: %v", err)
	}
}

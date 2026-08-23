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

const (
	clusterReadsFSIDA = "00000000-0000-4000-8000-000000000901"
	clusterReadsFSIDB = "00000000-0000-4000-8000-000000000902"
)

func cleanupClusterReadsRows(t *testing.T, db *sql.DB) {
	t.Helper()
	testdb.DeleteClusters(t, db, "fsid::text IN ($1, $2)", clusterReadsFSIDA, clusterReadsFSIDB)
	testdb.DeleteClusters(t, db, "name LIKE 'store-clusterreads-nofsid%'")
}

// seedClusterReads persists two clusters with distinct entity sets in
// the same database: A is a fake-provider sync (osd 10, one host), B is
// an agent push (osd 20/21, two hosts, a down OSD, its own pools).
func seedClusterReads(t *testing.T, store *PostgresStore) {
	t.Helper()
	ctx := context.Background()
	observedAt := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	base := func(fsid, name string, provider string) InventoryObservation {
		return InventoryObservation{
			Provider:   provider,
			ObservedAt: observedAt,
			Cluster: fleet.ClusterIdentity{
				FSID:        fsid,
				Name:        name,
				CephVersion: "18.2.x",
				Type:        fleet.ClusterTypeBareMetal,
			},
			Health: inventory.Health{Status: inventory.HealthOK, Summary: "healthy"},
		}
	}

	a := base(clusterReadsFSIDA, "clusterreads-a", "fake")
	a.OSDs = []inventory.OSD{{ID: 10, Host: "host-ra.example.invalid", Up: true, In: true}}
	a.Hosts = []inventory.Host{{Name: "host-ra.example.invalid"}}
	a.Daemons = []inventory.Daemon{{Type: inventory.DaemonMon, Name: "mon.a", Host: "host-ra.example.invalid", Status: inventory.DaemonRunning}}
	a.Pools = []inventory.Pool{{ID: 1, Name: "pool-a", Type: "replicated"}}
	if _, err := store.SaveInventoryObservation(ctx, a); err != nil {
		t.Fatalf("seed cluster A: %v", err)
	}

	b := base(clusterReadsFSIDB, "clusterreads-b", "agent")
	b.Health = inventory.Health{
		Status:  inventory.HealthErr,
		Summary: "one OSD down",
		Checks:  []inventory.HealthCheck{{Name: "OSD", Severity: "error", Summary: "osd.21 down"}},
	}
	b.OSDs = []inventory.OSD{
		{ID: 20, Host: "host-rb1.example.invalid", Up: true, In: true},
		{ID: 21, Host: "host-rb2.example.invalid", Up: false, In: true},
	}
	b.Hosts = []inventory.Host{{Name: "host-rb1.example.invalid"}, {Name: "host-rb2.example.invalid"}}
	b.Devices = []inventory.StorageDevice{{Host: "host-rb1.example.invalid", Serial: "serial-rb1"}}
	b.Daemons = []inventory.Daemon{{Type: inventory.DaemonOsd, Name: "osd.21", Host: "host-rb2.example.invalid", Status: inventory.DaemonStopped}}
	b.Pools = []inventory.Pool{{ID: 2, Name: "pool-b", Type: "replicated"}}
	if _, err := store.SaveInventoryObservation(ctx, b); err != nil {
		t.Fatalf("seed cluster B: %v", err)
	}
}

func TestClusterScopedReadsIsolateClusters(t *testing.T) {
	db, _ := testdb.Open(t)
	store := NewPostgres(db)
	cleanupClusterReadsRows(t, db)
	t.Cleanup(func() { cleanupClusterReadsRows(t, db) })
	seedClusterReads(t, store)
	ctx := context.Background()

	healthB, err := store.ClusterHealth(ctx, clusterReadsFSIDB)
	if err != nil {
		t.Fatalf("ClusterHealth B: %v", err)
	}
	if healthB.Status != inventory.HealthErr || len(healthB.Checks) != 1 {
		t.Fatalf("health B = %+v, want HEALTH_ERR with one check", healthB)
	}
	healthA, err := store.ClusterHealth(ctx, clusterReadsFSIDA)
	if err != nil {
		t.Fatalf("ClusterHealth A: %v", err)
	}
	if healthA.Status != inventory.HealthOK || len(healthA.Checks) != 0 {
		t.Fatalf("health A = %+v, want HEALTH_OK with no checks", healthA)
	}

	osdsA, err := store.ClusterOSDs(ctx, clusterReadsFSIDA)
	if err != nil {
		t.Fatalf("ClusterOSDs A: %v", err)
	}
	osdsB, err := store.ClusterOSDs(ctx, clusterReadsFSIDB)
	if err != nil {
		t.Fatalf("ClusterOSDs B: %v", err)
	}
	if len(osdsA) != 1 || osdsA[0].ID != 10 {
		t.Fatalf("OSDs A = %+v, want exactly osd 10", osdsA)
	}
	if len(osdsB) != 2 || osdsB[0].ID != 20 || osdsB[1].Up {
		t.Fatalf("OSDs B = %+v, want osds 20 and a down 21", osdsB)
	}

	hostsB, err := store.ClusterHosts(ctx, clusterReadsFSIDB)
	if err != nil {
		t.Fatalf("ClusterHosts B: %v", err)
	}
	if len(hostsB) != 2 {
		t.Fatalf("hosts B = %+v, want 2", hostsB)
	}

	devicesB, err := store.ClusterStorageDevices(ctx, clusterReadsFSIDB)
	if err != nil {
		t.Fatalf("ClusterStorageDevices B: %v", err)
	}
	if len(devicesB) != 1 || devicesB[0].Serial != "serial-rb1" {
		t.Fatalf("devices B = %+v, want one serial-rb1 device", devicesB)
	}

	daemonsB, err := store.ClusterDaemons(ctx, clusterReadsFSIDB)
	if err != nil {
		t.Fatalf("ClusterDaemons B: %v", err)
	}
	if len(daemonsB) != 1 || daemonsB[0].Status != inventory.DaemonStopped {
		t.Fatalf("daemons B = %+v, want one stopped osd", daemonsB)
	}

	poolsA, err := store.ClusterPools(ctx, clusterReadsFSIDA)
	if err != nil {
		t.Fatalf("ClusterPools A: %v", err)
	}
	if len(poolsA) != 1 || poolsA[0].Name != "pool-a" {
		t.Fatalf("pools A = %+v, want pool-a only", poolsA)
	}
}

func TestClusterScopedReadsReportNotFound(t *testing.T) {
	db, _ := testdb.Open(t)
	store := NewPostgres(db)
	cleanupClusterReadsRows(t, db)
	t.Cleanup(func() { cleanupClusterReadsRows(t, db) })
	seedClusterReads(t, store)
	ctx := context.Background()

	reads := []struct {
		name string
		call func() error
	}{
		{"health", func() error { _, err := store.ClusterHealth(ctx, "00000000-0000-4000-8000-0000000009ff"); return err }},
		{"osds", func() error { _, err := store.ClusterOSDs(ctx, "00000000-0000-4000-8000-0000000009ff"); return err }},
		{"hosts", func() error { _, err := store.ClusterHosts(ctx, "00000000-0000-4000-8000-0000000009ff"); return err }},
		{"devices", func() error {
			_, err := store.ClusterStorageDevices(ctx, "00000000-0000-4000-8000-0000000009ff")
			return err
		}},
		{"daemons", func() error { _, err := store.ClusterDaemons(ctx, "00000000-0000-4000-8000-0000000009ff"); return err }},
		{"pools", func() error { _, err := store.ClusterPools(ctx, "00000000-0000-4000-8000-0000000009ff"); return err }},
		{"malformed fsid", func() error { _, err := store.ClusterPools(ctx, "not-a-uuid"); return err }},
	}
	for _, read := range reads {
		t.Run(read.name, func(t *testing.T) {
			err := read.call()
			var appErr apperr.Error
			if !errors.As(err, &appErr) || appErr.Class != apperr.NotFound {
				t.Fatalf("error = %v, want NotFound", err)
			}
			if appErr.Message != "cluster not found" {
				t.Fatalf("message = %q, want the unknown-cluster message", appErr.Message)
			}
		})
	}

	// Cluster A observed no devices: known cluster, missing entity.
	if _, err := store.ClusterStorageDevices(ctx, clusterReadsFSIDA); err == nil {
		t.Fatal("expected NotFound for a known cluster without device observations")
	} else {
		var appErr apperr.Error
		if !errors.As(err, &appErr) || appErr.Class != apperr.NotFound {
			t.Fatalf("error = %v, want NotFound", err)
		}
		if appErr.Message == "cluster not found" {
			t.Fatalf("message must distinguish a known cluster: %q", appErr.Message)
		}
	}
}

func TestListClusterSummariesIndexesHealthAndAgentLastSeen(t *testing.T) {
	db, _ := testdb.Open(t)
	store := NewPostgres(db)
	cleanupClusterReadsRows(t, db)
	t.Cleanup(func() { cleanupClusterReadsRows(t, db) })
	seedClusterReads(t, store)

	index, err := store.ListClusterSummaries(context.Background(), ListClustersQuery{})
	if err != nil {
		t.Fatalf("ListClusterSummaries: %v", err)
	}
	byName := map[string]ClusterSummary{}
	for _, summary := range index.Clusters {
		if summary.Name == "clusterreads-a" || summary.Name == "clusterreads-b" {
			byName[summary.Name] = summary
		}
	}
	if len(byName) != 2 {
		t.Fatalf("index misses the seeded clusters: %+v", index.Clusters)
	}

	a := byName["clusterreads-a"]
	if a.FSID == nil || *a.FSID != clusterReadsFSIDA {
		t.Fatalf("summary A fsid = %v, want %s", a.FSID, clusterReadsFSIDA)
	}
	if a.HealthStatus == nil || *a.HealthStatus != "HEALTH_OK" {
		t.Fatalf("summary A health = %v, want HEALTH_OK", a.HealthStatus)
	}
	if a.AgentLastSeen != nil {
		t.Fatalf("summary A agent last-seen = %v, want none (fake provider batch)", a.AgentLastSeen)
	}

	b := byName["clusterreads-b"]
	if b.HealthStatus == nil || *b.HealthStatus != "HEALTH_ERR" {
		t.Fatalf("summary B health = %v, want HEALTH_ERR", b.HealthStatus)
	}
	if b.AgentLastSeen == nil {
		t.Fatal("summary B agent last-seen = nil, want the agent push time")
	}
}

func TestListClusterSummariesSearchAndPagination(t *testing.T) {
	db, _ := testdb.Open(t)
	store := NewPostgres(db)
	cleanupClusterReadsRows(t, db)
	t.Cleanup(func() { cleanupClusterReadsRows(t, db) })
	seedClusterReads(t, store)
	ctx := context.Background()

	byName, err := store.ListClusterSummaries(ctx, ListClustersQuery{Search: "CLUSTERREADS-A"})
	if err != nil {
		t.Fatalf("search by name: %v", err)
	}
	if byName.Total != 1 || len(byName.Clusters) != 1 || byName.Clusters[0].Name != "clusterreads-a" {
		t.Fatalf("name search = %+v, want only clusterreads-a", byName)
	}

	byFSID, err := store.ListClusterSummaries(ctx, ListClustersQuery{Search: clusterReadsFSIDB})
	if err != nil {
		t.Fatalf("search by fsid: %v", err)
	}
	if byFSID.Total != 1 || byFSID.Clusters[0].Name != "clusterreads-b" {
		t.Fatalf("fsid search = %+v, want only clusterreads-b", byFSID)
	}

	pageOne, err := store.ListClusterSummaries(ctx, ListClustersQuery{Limit: 1})
	if err != nil {
		t.Fatalf("page one: %v", err)
	}
	if len(pageOne.Clusters) != 1 || pageOne.Total < 2 {
		t.Fatalf("page one = %+v, want one row of a larger total", pageOne)
	}
	if pageOne.Limit != 1 || pageOne.Offset != 0 {
		t.Fatalf("page one echo = %+v", pageOne)
	}
	pageTwo, err := store.ListClusterSummaries(ctx, ListClustersQuery{Limit: 1, Offset: 1})
	if err != nil {
		t.Fatalf("page two: %v", err)
	}
	if len(pageTwo.Clusters) != 1 || pageTwo.Clusters[0].ID == pageOne.Clusters[0].ID {
		t.Fatalf("page two = %+v, want the next distinct row", pageTwo)
	}
}

func TestListCasesFiltersByCluster(t *testing.T) {
	db, _ := testdb.Open(t)
	store := NewPostgres(db)
	cleanupClusterReadsRows(t, db)
	cleanupCases := func() {
		testdb.DeleteCases(t, db, "title LIKE 'store-clusterreads-case%'")
	}
	cleanupCases()
	t.Cleanup(cleanupCases)
	ctx := context.Background()

	for _, tc := range []struct {
		title string
		fsid  string
	}{
		{"store-clusterreads-case-a", clusterReadsFSIDA},
		{"store-clusterreads-case-b", clusterReadsFSIDB},
		{"store-clusterreads-case-none", ""},
	} {
		if _, err := store.CreateManualCase(ctx, ManualCaseInput{
			Title:       tc.title,
			Summary:     "cluster filter test",
			Severity:    "medium",
			ClusterFSID: tc.fsid,
			Actor:       registrationActor(),
		}); err != nil {
			t.Fatalf("create case %q: %v", tc.title, err)
		}
	}

	onlyA, err := store.ListCases(ctx, 100, clusterReadsFSIDA)
	if err != nil {
		t.Fatalf("ListCases for cluster A: %v", err)
	}
	if len(onlyA) != 1 || onlyA[0].Title != "store-clusterreads-case-a" {
		t.Fatalf("filtered cases = %+v, want only cluster A's case", onlyA)
	}

	all, err := store.ListCases(ctx, 100, "")
	if err != nil {
		t.Fatalf("ListCases unfiltered: %v", err)
	}
	mine := 0
	for _, item := range all {
		if strings.HasPrefix(item.Title, "store-clusterreads-case") {
			mine++
		}
	}
	if mine != 3 {
		t.Fatalf("unfiltered matched %d of this test's cases, want 3 (shared database)", mine)
	}

	if _, err := store.ListCases(ctx, 100, "not-a-uuid"); err == nil {
		t.Fatal("expected error for malformed cluster filter")
	} else {
		var appErr apperr.Error
		if !errors.As(err, &appErr) || appErr.Class != apperr.InvalidRequest {
			t.Fatalf("error = %v, want InvalidRequest", err)
		}
	}
}

func TestListClusterSummariesIncludesRegisteredUnobservedCluster(t *testing.T) {
	db, _ := testdb.Open(t)
	store := NewPostgres(db)
	cleanupClusterReadsRows(t, db)
	t.Cleanup(func() { cleanupClusterReadsRows(t, db) })

	registration, _ := createTestRegistration(t, store, "store-clusterreads-nofsid")

	index, err := store.ListClusterSummaries(context.Background(), ListClustersQuery{})
	if err != nil {
		t.Fatalf("ListClusterSummaries: %v", err)
	}
	var found *ClusterSummary
	for i, summary := range index.Clusters {
		if summary.ID == registration.ID {
			found = &index.Clusters[i]
		}
	}
	if found == nil {
		t.Fatalf("index misses the registered-but-unobserved cluster: %+v", index.Clusters)
	}
	if found.FSID != nil || found.HealthStatus != nil || found.AgentLastSeen != nil {
		t.Fatalf("unobserved summary = %+v, want nil fsid/health/last-seen", found)
	}
}

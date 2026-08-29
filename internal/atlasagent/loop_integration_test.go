package atlasagent

import (
	"context"
	"crypto/tls"
	"database/sql"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tonymontoya/ceph-atlas/internal/actor"
	atlasapi "github.com/tonymontoya/ceph-atlas/internal/api"
	"github.com/tonymontoya/ceph-atlas/internal/app"
	"github.com/tonymontoya/ceph-atlas/internal/ca/catest"
	"github.com/tonymontoya/ceph-atlas/internal/config"
	"github.com/tonymontoya/ceph-atlas/internal/providers/ceph"
	"github.com/tonymontoya/ceph-atlas/internal/providers/ceph/dashtest"
	"github.com/tonymontoya/ceph-atlas/internal/store"
	"github.com/tonymontoya/ceph-atlas/internal/testdb"
)

// loopHarness stands up the full enrolled loop against the real Atlas
// API server: PostgreSQL persistence, the enrollment CA through its
// real file path, a TLS listener with client-certificate verification,
// and the in-process fake Dashboard for collection. No real Ceph, no
// real certificates (ADR-0025, ADR-0026).
type loopHarness struct {
	db         *sql.DB
	apiServer  *httptest.Server
	authority  *catest.TestCA
	caCertPath string
	credential string
	stateDir   string
}

func newLoopHarness(t *testing.T) *loopHarness {
	t.Helper()
	ctx := context.Background()

	db, databaseURL := testdb.Open(t)
	cleanupLoopRows(t, db)
	t.Cleanup(func() { cleanupLoopRows(t, db) })

	authority := catest.New(t)
	certPath, keyPath := authority.WriteFiles(t)

	application, err := app.NewFromConfig(ctx, config.Config{
		DatabaseURL:          databaseURL,
		ReadSource:           config.ReadSourcePostgres,
		AgentMode:            config.AgentModeDisabled,
		EnrollmentCACertPath: certPath,
		EnrollmentCAKeyPath:  keyPath,
	})
	if err != nil {
		t.Fatalf("new app with enrollment CA: %v", err)
	}
	t.Cleanup(func() { _ = application.Close() })

	// The registration's one-time credential, minted through the same
	// store the Operator API uses.
	_, credential, err := application.ClusterRegistrations.CreateClusterRegistration(ctx, store.ClusterRegistrationInput{
		Name:        "atlasagent-loop-test",
		ClusterType: "bare-metal",
		Actor:       actor.Actor{Subject: "atlasagent-loop-test", DisplayName: "Atlas Agent Loop Test"},
	})
	if err != nil {
		t.Fatalf("create cluster registration: %v", err)
	}

	// A TLS listener in the real API server's shape: the enrollment CA
	// verifies Agent client certificates while the serving certificate
	// is signed by the same CA — the shape ATLAS_AGENT_ATLAS_CA_PATH
	// trusts.
	apiServer := httptest.NewUnstartedServer(atlasapi.NewServer(application).Routes())
	apiServer.TLS = &tls.Config{
		Certificates: []tls.Certificate{newServerCertificate(t, authority)},
		ClientCAs:    certPoolWith(t, authority),
		ClientAuth:   tls.VerifyClientCertIfGiven,
	}
	apiServer.StartTLS()
	t.Cleanup(apiServer.Close)

	return &loopHarness{
		db:         db,
		apiServer:  apiServer,
		authority:  authority,
		caCertPath: certPath,
		credential: credential.Token,
		stateDir:   t.TempDir(),
	}
}

func cleanupLoopRows(t *testing.T, db *sql.DB) {
	t.Helper()
	testdb.DeleteClusters(t, db, "name LIKE 'atlasagent-loop-test%' OR fsid::text = $1", dashtest.FSID)
}

func (h *loopHarness) newRunner(t *testing.T, credential string) *Runner {
	t.Helper()
	dashboard := dashtest.New(t, dashtest.ModeSuccess)
	provider, err := ceph.New(ceph.Config{
		BaseURL:  dashboard.URL(),
		Username: dashtest.Username,
		Password: dashtest.Password,
	})
	if err != nil {
		t.Fatalf("new dashboard provider: %v", err)
	}
	return NewRunner(Options{
		Config: config.AgentConfig{
			AtlasURL:             h.apiServer.URL,
			AtlasRootCAPath:      h.caCertPath,
			EnrollmentCredential: credential,
			StateDir:             h.stateDir,
			CollectInterval:      time.Hour,
			RetryInitial:         time.Millisecond,
			RetryMax:             4 * time.Millisecond,
		},
		Provider: provider,
		Log:      testLogger(t),
	})
}

// TestEnrolledLoopEndToEnd drives the whole v0.7 Agent contract
// against the real API server: enroll with the one-time credential,
// collect from the fake Dashboard, push over mutual TLS, and persist
// as provider "agent". The second run reuses the stored certificate —
// its credential was burned by the first enrollment.
func TestEnrolledLoopEndToEnd(t *testing.T) {
	h := newLoopHarness(t)
	ctx := context.Background()

	if err := h.newRunner(t, h.credential).RunOnce(ctx); err != nil {
		t.Fatalf("first run (enroll + collect + push): %v", err)
	}

	var snapshots int
	if err := h.db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM inventory_snapshots AS snapshots
		JOIN atlas_clusters AS clusters ON clusters.id = snapshots.cluster_id
		WHERE clusters.fsid::text = $1 AND snapshots.provider = 'agent'
	`, dashtest.FSID).Scan(&snapshots); err != nil {
		t.Fatalf("query snapshots: %v", err)
	}
	if snapshots != 1 {
		t.Fatalf("agent snapshots = %d, want 1", snapshots)
	}

	var osdObservations, deviceObservations, daemonObservations, poolObservations int
	if err := h.db.QueryRowContext(ctx, `
		SELECT
			(SELECT count(*) FROM osd_observations o JOIN inventory_snapshots s ON s.id = o.snapshot_id
			 JOIN atlas_clusters c ON c.id = s.cluster_id WHERE c.fsid::text = $1),
			(SELECT count(*) FROM storage_device_observations o JOIN inventory_snapshots s ON s.id = o.snapshot_id
			 JOIN atlas_clusters c ON c.id = s.cluster_id WHERE c.fsid::text = $1),
			(SELECT count(*) FROM daemon_observations o JOIN inventory_snapshots s ON s.id = o.snapshot_id
			 JOIN atlas_clusters c ON c.id = s.cluster_id WHERE c.fsid::text = $1),
			(SELECT count(*) FROM pool_observations o JOIN inventory_snapshots s ON s.id = o.snapshot_id
			 JOIN atlas_clusters c ON c.id = s.cluster_id WHERE c.fsid::text = $1)
	`, dashtest.FSID).Scan(&osdObservations, &deviceObservations, &daemonObservations, &poolObservations); err != nil {
		t.Fatalf("query observations: %v", err)
	}
	if osdObservations != 3 || deviceObservations != 3 || daemonObservations != 5 || poolObservations != 2 {
		t.Fatalf("observations = osd %d / device %d / daemon %d / pool %d, want 3/3/5/2",
			osdObservations, deviceObservations, daemonObservations, poolObservations)
	}

	// The cluster row carries the observed identity: registered as
	// "atlasagent-loop-test", renamed to the observed cluster name by
	// the push's upsert semantics.
	var boundFSID sql.NullString
	var name string
	if err := h.db.QueryRowContext(ctx, `
		SELECT fsid::text, name FROM atlas_clusters WHERE name = 'ceph'
	`).Scan(&boundFSID, &name); err != nil {
		t.Fatalf("query bound cluster: %v", err)
	}
	if !boundFSID.Valid || boundFSID.String != dashtest.FSID {
		t.Fatalf("bound fsid = %v, want %s", boundFSID, dashtest.FSID)
	}

	// Second run: no credential, stored certificate.
	if err := h.newRunner(t, "").RunOnce(ctx); err != nil {
		t.Fatalf("second run (stored certificate): %v", err)
	}
	if err := h.db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM inventory_snapshots AS snapshots
		JOIN atlas_clusters AS clusters ON clusters.id = snapshots.cluster_id
		WHERE clusters.fsid::text = $1 AND snapshots.provider = 'agent'
	`, dashtest.FSID).Scan(&snapshots); err != nil {
		t.Fatalf("query snapshots after second run: %v", err)
	}
	if snapshots != 2 {
		t.Fatalf("agent snapshots after second run = %d, want 2", snapshots)
	}
}

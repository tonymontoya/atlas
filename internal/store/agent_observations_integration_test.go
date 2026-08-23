package store

import (
	"context"
	"database/sql"
	"testing"

	"github.com/tonymontoya/ceph-atlas/internal/apperr"
	"github.com/tonymontoya/ceph-atlas/internal/ca"
	"github.com/tonymontoya/ceph-atlas/internal/ca/catest"
	"github.com/tonymontoya/ceph-atlas/internal/testdb"
)

const agentObservationTestFSID = "00000000-0000-4000-8000-000000000701"

func cleanupAgentObservationRows(t *testing.T, db *sql.DB) {
	t.Helper()
	testdb.DeleteClusters(t, db, "name LIKE 'store-agentobs-test%'")
}

// enrollTestAgent runs a full enrollment for a freshly registered
// cluster and returns the issued certificate's serial number and the
// enrolled cluster's id.
func enrollTestAgent(t *testing.T, store *PostgresStore, authority *catest.TestCA, name, fsid string) (serial string, clusterID int64) {
	t.Helper()
	registration, credential := createTestRegistration(t, store, name)
	certificate, err := authority.Issue(catest.NewCSR(t))
	if err != nil {
		t.Fatalf("issue test certificate: %v", err)
	}
	if _, err := store.EnrollAgent(context.Background(), EnrollAgentInput{
		CredentialToken: credential.Token,
		FSID:            fsid,
	}, func() (ca.IssuedCertificate, error) { return certificate, nil }); err != nil {
		t.Fatalf("enroll test agent: %v", err)
	}
	return certificate.SerialNumber, registration.ID
}

func TestResolveAgentClusterMapsSerialToEnrolledCluster(t *testing.T) {
	db, _ := testdb.Open(t)
	store := NewPostgres(db)
	authority := catest.New(t)
	cleanupAgentObservationRows(t, db)
	t.Cleanup(func() { cleanupAgentObservationRows(t, db) })

	serial, wantID := enrollTestAgent(t, store, authority, "store-agentobs-test-a", agentObservationTestFSID)

	resolved, err := store.ResolveAgentCluster(context.Background(), serial)
	if err != nil {
		t.Fatalf("ResolveAgentCluster returned error: %v", err)
	}
	if resolved.ClusterID != wantID {
		t.Fatalf("resolved cluster id = %d, want %d", resolved.ClusterID, wantID)
	}
	if resolved.FSID != agentObservationTestFSID {
		t.Fatalf("resolved fsid = %q, want %q", resolved.FSID, agentObservationTestFSID)
	}
}

func TestResolveAgentClusterRejectsUnknownSerial(t *testing.T) {
	db, _ := testdb.Open(t)
	store := NewPostgres(db)

	_, err := store.ResolveAgentCluster(context.Background(), "cafebabe")
	assertEnrollClass(t, err, apperr.Unauthorized, "not valid for an enrolled cluster")
}

func TestResolveAgentClusterRejectsEmptySerial(t *testing.T) {
	db, _ := testdb.Open(t)
	store := NewPostgres(db)

	_, err := store.ResolveAgentCluster(context.Background(), "")
	assertEnrollClass(t, err, apperr.InvalidRequest, "serial number")
}

func TestResolveAgentClusterRejectsRevokedCertificate(t *testing.T) {
	db, _ := testdb.Open(t)
	store := NewPostgres(db)
	authority := catest.New(t)
	cleanupAgentObservationRows(t, db)
	t.Cleanup(func() { cleanupAgentObservationRows(t, db) })

	serial, _ := enrollTestAgent(t, store, authority, "store-agentobs-test-revoked", agentObservationTestFSID)
	if _, err := db.Exec("UPDATE cluster_agent_certificates SET revoked_at = now() WHERE serial_number = $1", serial); err != nil {
		t.Fatalf("revoke certificate: %v", err)
	}

	_, err := store.ResolveAgentCluster(context.Background(), serial)
	assertEnrollClass(t, err, apperr.Unauthorized, "not valid for an enrolled cluster")
}

func TestResolveAgentClusterRejectsExpiredCertificate(t *testing.T) {
	db, _ := testdb.Open(t)
	store := NewPostgres(db)
	authority := catest.New(t)
	cleanupAgentObservationRows(t, db)
	t.Cleanup(func() { cleanupAgentObservationRows(t, db) })

	serial, _ := enrollTestAgent(t, store, authority, "store-agentobs-test-expired", agentObservationTestFSID)
	if _, err := db.Exec("UPDATE cluster_agent_certificates SET not_before = now() - interval '2 years', not_after = now() - interval '1 year' WHERE serial_number = $1", serial); err != nil {
		t.Fatalf("expire certificate: %v", err)
	}

	_, err := store.ResolveAgentCluster(context.Background(), serial)
	assertEnrollClass(t, err, apperr.Unauthorized, "not valid for an enrolled cluster")
}

func TestResolveAgentClusterRejectsDeregisteredCluster(t *testing.T) {
	db, _ := testdb.Open(t)
	store := NewPostgres(db)
	authority := catest.New(t)
	cleanupAgentObservationRows(t, db)
	t.Cleanup(func() { cleanupAgentObservationRows(t, db) })

	serial, clusterID := enrollTestAgent(t, store, authority, "store-agentobs-test-deregistered", agentObservationTestFSID)
	if _, err := store.DeregisterCluster(context.Background(), DeregisterClusterInput{
		ClusterID: clusterID,
		Actor:     registrationActor(),
	}); err != nil {
		t.Fatalf("deregister cluster: %v", err)
	}

	_, err := store.ResolveAgentCluster(context.Background(), serial)
	assertEnrollClass(t, err, apperr.Unauthorized, "not valid for an enrolled cluster")
}

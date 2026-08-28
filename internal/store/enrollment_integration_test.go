package store

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tonymontoya/ceph-atlas/internal/apperr"
	"github.com/tonymontoya/ceph-atlas/internal/ca"
	"github.com/tonymontoya/ceph-atlas/internal/ca/catest"
	"github.com/tonymontoya/ceph-atlas/internal/testdb"
)

const enrollTestFSID = "00000000-0000-4000-8000-000000000501"

// testIssuer records whether the CA was consulted and signs real
// certificates from an in-process test CA (ADR-0026: tests never touch
// real key material).
func testIssuer(t *testing.T, authority *catest.TestCA) IssueCertificate {
	t.Helper()
	return func() (ca.IssuedCertificate, error) {
		return authority.Issue(catest.NewCSR(t))
	}
}

func failingIssuer() (ca.IssuedCertificate, error) {
	return ca.IssuedCertificate{}, apperr.Error{Class: apperr.Internal, Message: "signing failed for test"}
}

func assertEnrollClass(t *testing.T, err error, want apperr.Class, wantMessagePart string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with class %q, got nil", want)
	}
	var appErr apperr.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("error type = %T, want apperr.Error", err)
	}
	if appErr.Class != want {
		t.Fatalf("error class = %q, want %q (message: %s)", appErr.Class, want, appErr.Message)
	}
	if wantMessagePart != "" && !strings.Contains(appErr.Message, wantMessagePart) {
		t.Fatalf("error %q does not contain %q", appErr.Message, wantMessagePart)
	}
}

func TestEnrollAgentIssuesCertificateBurnsCredentialBindsFSID(t *testing.T) {
	db, _ := testdb.Open(t)
	ctx := context.Background()
	store := NewPostgres(db)
	authority := catest.New(t)

	registration, credential := createTestRegistration(t, store, "store-enroll-test-a")
	defer testdb.DeleteClusters(t, db, "id = $1", registration.ID)

	result, err := store.EnrollAgent(ctx, EnrollAgentInput{
		CredentialToken: credential.Token,
		FSID:            enrollTestFSID,
	}, testIssuer(t, authority))
	if err != nil {
		t.Fatalf("EnrollAgent returned error: %v", err)
	}

	if result.Cluster.FSID == nil || *result.Cluster.FSID != enrollTestFSID {
		t.Fatalf("enrolled cluster fsid = %v, want %s bound at enrollment", result.Cluster.FSID, enrollTestFSID)
	}
	if result.Certificate.SerialNumber == "" || len(result.Certificate.PEMChain) == 0 {
		t.Fatalf("enrollment certificate incomplete: %+v", result.Certificate)
	}

	var recordedClusterID int64
	var revokedAt *string
	if err := db.QueryRowContext(ctx, `
		SELECT cluster_id, revoked_at
		FROM cluster_agent_certificates
		WHERE serial_number = $1
	`, result.Certificate.SerialNumber).Scan(&recordedClusterID, &revokedAt); err != nil {
		t.Fatalf("query recorded certificate: %v", err)
	}
	if recordedClusterID != registration.ID {
		t.Fatalf("certificate maps to cluster %d, want %d", recordedClusterID, registration.ID)
	}
	if revokedAt != nil {
		t.Fatal("freshly issued certificate must not be revoked")
	}

	// Replay with the same credential fails: it burned on first use.
	_, err = store.EnrollAgent(ctx, EnrollAgentInput{
		CredentialToken: credential.Token,
		FSID:            "00000000-0000-4000-8000-000000000502",
	}, testIssuer(t, authority))
	assertEnrollClass(t, err, apperr.Unauthorized, "already used")
}

func TestEnrollAgentRejectsDuplicateFSID(t *testing.T) {
	db, _ := testdb.Open(t)
	ctx := context.Background()
	store := NewPostgres(db)
	authority := catest.New(t)

	first, firstCredential := createTestRegistration(t, store, "store-enroll-test-dup-a")
	defer testdb.DeleteClusters(t, db, "id = $1", first.ID)
	second, secondCredential := createTestRegistration(t, store, "store-enroll-test-dup-b")
	defer testdb.DeleteClusters(t, db, "id = $1", second.ID)

	if _, err := store.EnrollAgent(ctx, EnrollAgentInput{
		CredentialToken: firstCredential.Token,
		FSID:            enrollTestFSID,
	}, testIssuer(t, authority)); err != nil {
		t.Fatalf("first EnrollAgent returned error: %v", err)
	}

	_, err := store.EnrollAgent(ctx, EnrollAgentInput{
		CredentialToken: secondCredential.Token,
		FSID:            enrollTestFSID,
	}, testIssuer(t, authority))
	assertEnrollClass(t, err, apperr.Conflict, "another cluster")
}

func TestEnrollAgentRejectsExpiredCredential(t *testing.T) {
	db, _ := testdb.Open(t)
	ctx := context.Background()
	store := NewPostgres(db)
	authority := catest.New(t)

	registration, credential := createTestRegistration(t, store, "store-enroll-test-expired")
	defer testdb.DeleteClusters(t, db, "id = $1", registration.ID)

	if _, err := db.ExecContext(ctx, `
		UPDATE cluster_enrollment_credentials
		SET expires_at = now() - interval '1 minute'
		WHERE cluster_id = $1 AND consumed_at IS NULL
	`, registration.ID); err != nil {
		t.Fatalf("expire credential: %v", err)
	}

	_, err := store.EnrollAgent(ctx, EnrollAgentInput{
		CredentialToken: credential.Token,
		FSID:            enrollTestFSID,
	}, testIssuer(t, authority))
	assertEnrollClass(t, err, apperr.Unauthorized, "expired")

	// The expired credential must not have been consumed.
	var consumed int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM cluster_enrollment_credentials
		WHERE cluster_id = $1 AND consumed_at IS NOT NULL
	`, registration.ID).Scan(&consumed); err != nil {
		t.Fatalf("count consumed credentials: %v", err)
	}
	if consumed != 0 {
		t.Fatalf("expired enrollment consumed %d credentials, want 0", consumed)
	}
}

func TestEnrollAgentKeepsCredentialWhenIssuanceFails(t *testing.T) {
	db, _ := testdb.Open(t)
	ctx := context.Background()
	store := NewPostgres(db)
	authority := catest.New(t)

	registration, credential := createTestRegistration(t, store, "store-enroll-test-issuefail")
	defer testdb.DeleteClusters(t, db, "id = $1", registration.ID)

	if _, err := store.EnrollAgent(ctx, EnrollAgentInput{
		CredentialToken: credential.Token,
		FSID:            enrollTestFSID,
	}, failingIssuer); err == nil {
		t.Fatal("expected issuance failure to fail enrollment")
	}

	// Nothing persisted: no FSID, no certificate, credential still live.
	boundRegistration, err := store.GetClusterRegistration(ctx, registration.ID)
	if err != nil {
		t.Fatalf("get registration: %v", err)
	}
	if boundRegistration.FSID != nil {
		t.Fatalf("fsid = %v bound despite failed issuance", boundRegistration.FSID)
	}
	var certificates int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM cluster_agent_certificates WHERE cluster_id = $1
	`, registration.ID).Scan(&certificates); err != nil {
		t.Fatalf("count certificates: %v", err)
	}
	if certificates != 0 {
		t.Fatalf("failed enrollment recorded %d certificates, want 0", certificates)
	}

	// The same credential still enrolls once issuance works again.
	result, err := store.EnrollAgent(ctx, EnrollAgentInput{
		CredentialToken: credential.Token,
		FSID:            enrollTestFSID,
	}, testIssuer(t, authority))
	if err != nil {
		t.Fatalf("retry EnrollAgent returned error: %v", err)
	}
	if result.Cluster.FSID == nil || *result.Cluster.FSID != enrollTestFSID {
		t.Fatalf("retry fsid = %v, want bound", result.Cluster.FSID)
	}
}

func TestEnrollAgentRejectsMalformedInput(t *testing.T) {
	db, _ := testdb.Open(t)
	ctx := context.Background()
	store := NewPostgres(db)
	authority := catest.New(t)

	registration, credential := createTestRegistration(t, store, "store-enroll-test-invalid")
	defer testdb.DeleteClusters(t, db, "id = $1", registration.ID)

	_, err := store.EnrollAgent(ctx, EnrollAgentInput{
		CredentialToken: "",
		FSID:            enrollTestFSID,
	}, testIssuer(t, authority))
	assertEnrollClass(t, err, apperr.InvalidRequest, "credentialToken")

	_, err = store.EnrollAgent(ctx, EnrollAgentInput{
		CredentialToken: credential.Token,
		FSID:            "not-a-uuid",
	}, testIssuer(t, authority))
	assertEnrollClass(t, err, apperr.InvalidRequest, "UUID")
}

func TestEnrollAgentRejectsDeregisteredClusterCredential(t *testing.T) {
	db, _ := testdb.Open(t)
	ctx := context.Background()
	store := NewPostgres(db)
	authority := catest.New(t)

	registration, credential := createTestRegistration(t, store, "store-enroll-test-deregistered")
	defer testdb.DeleteClusters(t, db, "id = $1", registration.ID)

	if _, err := store.DeregisterCluster(ctx, DeregisterClusterInput{
		ClusterID: registration.ID,
		Actor:     registrationActor(),
	}); err != nil {
		t.Fatalf("deregister cluster: %v", err)
	}

	// Deregistration burns live credentials (#33), so the token no
	// longer authenticates anything.
	_, err := store.EnrollAgent(ctx, EnrollAgentInput{
		CredentialToken: credential.Token,
		FSID:            enrollTestFSID,
	}, testIssuer(t, authority))
	assertEnrollClass(t, err, apperr.Unauthorized, "invalid, expired, or already used")
}

// TestEnrollAgentTransfersFSIDFromDeregisteredCluster pins the renewal
// semantics (#44, ADR-0026 amendment): a deregistered cluster's FSID is
// a stale claim that a fresh enrollment of the same physical cluster
// takes over, while a live holder keeps blocking.
func TestEnrollAgentTransfersFSIDFromDeregisteredCluster(t *testing.T) {
	db, _ := testdb.Open(t)
	ctx := context.Background()
	store := NewPostgres(db)
	authority := catest.New(t)

	first, firstCredential := createTestRegistration(t, store, "store-enroll-renew-a")
	defer testdb.DeleteClusters(t, db, "id = $1", first.ID)
	firstResult, err := store.EnrollAgent(ctx, EnrollAgentInput{
		CredentialToken: firstCredential.Token,
		FSID:            enrollTestFSID,
	}, testIssuer(t, authority))
	if err != nil {
		t.Fatalf("first EnrollAgent returned error: %v", err)
	}
	if _, err := store.DeregisterCluster(ctx, DeregisterClusterInput{
		ClusterID: first.ID,
		Actor:     registrationActor(),
	}); err != nil {
		t.Fatalf("deregister first cluster: %v", err)
	}

	// The retired Agent's certificate no longer maps anywhere.
	if _, err := store.ResolveAgentCluster(ctx, firstResult.Certificate.SerialNumber); err == nil {
		t.Fatal("deregistered cluster's certificate must not resolve")
	} else {
		assertEnrollClass(t, err, apperr.Unauthorized, "not valid for an enrolled cluster")
	}

	second, secondCredential := createTestRegistration(t, store, "store-enroll-renew-b")
	defer testdb.DeleteClusters(t, db, "id = $1", second.ID)
	secondResult, err := store.EnrollAgent(ctx, EnrollAgentInput{
		CredentialToken: secondCredential.Token,
		FSID:            enrollTestFSID,
	}, testIssuer(t, authority))
	if err != nil {
		t.Fatalf("re-enrollment with the deregistered holder's FSID: %v", err)
	}
	if secondResult.Cluster.FSID == nil || *secondResult.Cluster.FSID != enrollTestFSID {
		t.Fatalf("re-enrolled fsid = %v, want %s", secondResult.Cluster.FSID, enrollTestFSID)
	}
	if secondResult.Cluster.ID != second.ID {
		t.Fatalf("re-enrollment bound cluster %d, want the fresh registration %d", secondResult.Cluster.ID, second.ID)
	}

	// The stale claim is gone: the old row released its FSID, the new
	// row is the only holder, and the new Agent's certificate maps to
	// the new registration.
	oldRow, err := store.GetClusterRegistration(ctx, first.ID)
	if err != nil {
		t.Fatalf("get deregistered registration: %v", err)
	}
	if oldRow.DeregisteredAt == nil || oldRow.FSID != nil {
		t.Fatalf("deregistered row = %+v, want retained deregistration with a released fsid", oldRow)
	}
	resolved, err := store.ResolveAgentCluster(ctx, secondResult.Certificate.SerialNumber)
	if err != nil {
		t.Fatalf("resolve new certificate: %v", err)
	}
	if resolved.ClusterID != second.ID || resolved.FSID != enrollTestFSID {
		t.Fatalf("new certificate maps to %+v, want cluster %d fsid %s", resolved, second.ID, enrollTestFSID)
	}
}

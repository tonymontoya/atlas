package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tonymontoya/ceph-atlas/internal/actor"
	"github.com/tonymontoya/ceph-atlas/internal/apperr"
	"github.com/tonymontoya/ceph-atlas/internal/fleet"
	"github.com/tonymontoya/ceph-atlas/internal/testdb"
)

func registrationActor() actor.Actor {
	return actor.Actor{Subject: "registration-operator", DisplayName: "Registration Operator"}
}

func createTestRegistration(t *testing.T, store *PostgresStore, name string) (fleet.ClusterRegistration, fleet.EnrollmentCredential) {
	t.Helper()
	registration, credential, err := store.CreateClusterRegistration(context.Background(), ClusterRegistrationInput{
		Name:        name,
		ClusterType: string(fleet.ClusterTypeBareMetal),
		Actor:       registrationActor(),
	})
	if err != nil {
		t.Fatalf("create registration: %v", err)
	}
	return registration, credential
}

func TestCreateClusterRegistrationReturnsOneTimeCredential(t *testing.T) {
	db, _ := testdb.Open(t)
	ctx := context.Background()
	store := NewPostgres(db)

	registration, credential := createTestRegistration(t, store, "store-registration-test-a")
	defer testdb.DeleteClusters(t, db, "id = $1", registration.ID)

	if registration.ID <= 0 {
		t.Fatalf("registration id = %d, want positive", registration.ID)
	}
	if registration.FSID != nil {
		t.Fatalf("registration fsid = %v, want nil until enrollment", registration.FSID)
	}
	if registration.CephVersion != nil {
		t.Fatalf("registration cephVersion = %v, want nil until first observation", registration.CephVersion)
	}
	if registration.Type != fleet.ClusterTypeBareMetal {
		t.Fatalf("registration type = %q, want bare-metal", registration.Type)
	}
	if registration.RegisteredBy != "registration-operator" {
		t.Fatalf("registration registeredBy = %q", registration.RegisteredBy)
	}
	if registration.DeregisteredAt != nil {
		t.Fatal("registration deregisteredAt should be nil on creation")
	}

	if !strings.HasPrefix(credential.Token, "atl_enroll_") {
		t.Fatalf("credential token %q should carry the enrollment prefix", credential.Token)
	}
	if len(credential.Token) != len("atl_enroll_")+43 {
		t.Fatalf("credential token length = %d, want prefix + 43 base64url chars", len(credential.Token))
	}
	ttl := time.Until(credential.ExpiresAt)
	if ttl <= 0 || ttl > EnrollmentCredentialTTL {
		t.Fatalf("credential ttl = %v, want within %v", ttl, EnrollmentCredentialTTL)
	}

	var storedHash string
	var consumedAt *time.Time
	err := db.QueryRowContext(ctx, `
		SELECT credential_hash, consumed_at
		FROM cluster_enrollment_credentials
		WHERE cluster_id = $1
	`, registration.ID).Scan(&storedHash, &consumedAt)
	if err != nil {
		t.Fatalf("query credential row: %v", err)
	}
	if consumedAt != nil {
		t.Fatal("fresh credential must be unconsumed")
	}
	digest := sha256.Sum256([]byte(credential.Token))
	if storedHash != hex.EncodeToString(digest[:]) {
		t.Fatal("stored credential hash does not match sha256 of the returned token")
	}
	if storedHash == credential.Token {
		t.Fatal("credential must not be stored in plaintext")
	}
}

func TestCreateClusterRegistrationValidation(t *testing.T) {
	db, _ := testdb.Open(t)
	store := NewPostgres(db)

	cases := []struct {
		name  string
		input ClusterRegistrationInput
		want  string
	}{
		{"missing name", ClusterRegistrationInput{Name: "", ClusterType: "bare-metal", Actor: registrationActor()}, "name is required"},
		{"unsupported type", ClusterRegistrationInput{Name: "x", ClusterType: "rook", Actor: registrationActor()}, "clusterType must be bare-metal"},
		{"missing actor", ClusterRegistrationInput{Name: "x", ClusterType: "bare-metal"}, "actor subject is required"},
	}
	for _, testCase := range cases {
		_, _, err := store.CreateClusterRegistration(context.Background(), testCase.input)
		var appErr apperr.Error
		if !errors.As(err, &appErr) || appErr.Class != apperr.InvalidRequest || !strings.Contains(appErr.Message, testCase.want) {
			t.Fatalf("%s: err = %v, want InvalidRequest containing %q", testCase.name, err, testCase.want)
		}
	}
}

func TestGetClusterRegistration(t *testing.T) {
	db, _ := testdb.Open(t)
	ctx := context.Background()
	store := NewPostgres(db)

	created, _ := createTestRegistration(t, store, "store-registration-test-b")
	defer testdb.DeleteClusters(t, db, "id = $1", created.ID)

	fetched, err := store.GetClusterRegistration(ctx, created.ID)
	if err != nil {
		t.Fatalf("get registration: %v", err)
	}
	if fetched.ID != created.ID || fetched.Name != created.Name {
		t.Fatalf("fetched = %+v, want created %+v", fetched, created)
	}

	if _, err := store.GetClusterRegistration(ctx, created.ID+1000000); err == nil {
		t.Fatal("expected notFound for unknown cluster id")
	}
}

func TestDeregisterClusterLifecycle(t *testing.T) {
	db, _ := testdb.Open(t)
	ctx := context.Background()
	store := NewPostgres(db)

	created, _ := createTestRegistration(t, store, "store-registration-test-c")
	defer testdb.DeleteClusters(t, db, "id = $1", created.ID)

	deregistered, err := store.DeregisterCluster(ctx, DeregisterClusterInput{ClusterID: created.ID, Actor: registrationActor()})
	if err != nil {
		t.Fatalf("deregister: %v", err)
	}
	if deregistered.DeregisteredAt == nil {
		t.Fatal("deregistered registration should carry deregisteredAt")
	}

	var live int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM cluster_enrollment_credentials
		WHERE cluster_id = $1 AND consumed_at IS NULL
	`, created.ID).Scan(&live); err != nil {
		t.Fatalf("count live credentials: %v", err)
	}
	if live != 0 {
		t.Fatalf("live credentials after deregistration = %d, want 0", live)
	}

	_, err = store.DeregisterCluster(ctx, DeregisterClusterInput{ClusterID: created.ID, Actor: registrationActor()})
	var appErr apperr.Error
	if !errors.As(err, &appErr) || appErr.Class != apperr.Conflict {
		t.Fatalf("second deregister err = %v, want Conflict", err)
	}

	// History stays readable after deregistration.
	fetched, err := store.GetClusterRegistration(ctx, created.ID)
	if err != nil {
		t.Fatalf("get deregistered registration: %v", err)
	}
	if fetched.DeregisteredAt == nil {
		t.Fatal("deregistered registration should still expose deregisteredAt")
	}
}

func TestDeregisterClusterPreservesSnapshotsAndCases(t *testing.T) {
	db, _ := testdb.Open(t)
	ctx := context.Background()
	store := NewPostgres(db)

	created, _ := createTestRegistration(t, store, "store-registration-test-d")
	defer testdb.DeleteClusters(t, db, "id = $1", created.ID)
	defer testdb.DeleteCases(t, db, "title = 'store-registration-test case'")

	if _, err := store.BindClusterFSID(ctx, created.ID, "00000000-0000-4000-8000-000000000d01"); err != nil {
		t.Fatalf("bind fsid: %v", err)
	}

	var snapshotID int64
	err := db.QueryRowContext(ctx, `
		INSERT INTO inventory_snapshots (cluster_id, provider, observed_at)
		VALUES ($1, 'fake', now())
		RETURNING id
	`, created.ID).Scan(&snapshotID)
	if err != nil {
		t.Fatalf("insert snapshot: %v", err)
	}

	createdCase, err := store.CreateManualCase(ctx, ManualCaseInput{
		Title:       "store-registration-test case",
		Summary:     "Case that must survive deregistration.",
		Severity:    "medium",
		ClusterFSID: "00000000-0000-4000-8000-000000000d01",
		Actor:       registrationActor(),
	})
	if err != nil {
		t.Fatalf("create case: %v", err)
	}

	if _, err := store.DeregisterCluster(ctx, DeregisterClusterInput{ClusterID: created.ID, Actor: registrationActor()}); err != nil {
		t.Fatalf("deregister: %v", err)
	}

	var snapshotExists, caseExists bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM inventory_snapshots WHERE id = $1)`, snapshotID).Scan(&snapshotExists); err != nil {
		t.Fatalf("check snapshot: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM cases WHERE id = $1)`, createdCase.ID).Scan(&caseExists); err != nil {
		t.Fatalf("check case: %v", err)
	}
	if !snapshotExists || !caseExists {
		t.Fatalf("deregistration must preserve snapshots and cases (snapshot=%v case=%v)", snapshotExists, caseExists)
	}
}

func TestBindClusterFSID(t *testing.T) {
	db, _ := testdb.Open(t)
	ctx := context.Background()
	store := NewPostgres(db)

	first, _ := createTestRegistration(t, store, "store-registration-test-e1")
	second, _ := createTestRegistration(t, store, "store-registration-test-e2")
	defer testdb.DeleteClusters(t, db, "id = ANY($1)", []int64{first.ID, second.ID})

	bound, err := store.BindClusterFSID(ctx, first.ID, "00000000-0000-4000-8000-000000000E01")
	if err != nil {
		t.Fatalf("bind fsid: %v", err)
	}
	if bound.FSID == nil || *bound.FSID != "00000000-0000-4000-8000-000000000e01" {
		t.Fatalf("bound fsid = %v, want lowered uuid", bound.FSID)
	}

	if _, err := store.BindClusterFSID(ctx, first.ID, "00000000-0000-4000-8000-000000000e01"); err == nil {
		t.Fatal("rebinding the same cluster must conflict (fsid is immutable)")
	}
	if _, err := store.BindClusterFSID(ctx, first.ID, "00000000-0000-4000-8000-000000000e02"); err == nil {
		t.Fatal("rebinding to a different fsid must conflict")
	}
	if _, err := store.BindClusterFSID(ctx, second.ID, "00000000-0000-4000-8000-000000000e01"); err == nil {
		t.Fatal("binding an fsid owned by another cluster must conflict")
	}
	if _, err := store.BindClusterFSID(ctx, first.ID+1000000, "00000000-0000-4000-8000-000000000e03"); err == nil {
		t.Fatal("binding an unknown cluster must be notFound")
	}
	if _, err := store.BindClusterFSID(ctx, second.ID, "not-a-uuid"); err == nil {
		t.Fatal("binding a non-uuid must be invalid")
	}
}

func TestBindClusterFSIDRejectedAfterDeregistration(t *testing.T) {
	db, _ := testdb.Open(t)
	ctx := context.Background()
	store := NewPostgres(db)

	created, _ := createTestRegistration(t, store, "store-registration-test-f")
	defer testdb.DeleteClusters(t, db, "id = $1", created.ID)

	if _, err := store.DeregisterCluster(ctx, DeregisterClusterInput{ClusterID: created.ID, Actor: registrationActor()}); err != nil {
		t.Fatalf("deregister: %v", err)
	}
	if _, err := store.BindClusterFSID(ctx, created.ID, "00000000-0000-4000-8000-000000000f01"); err == nil {
		t.Fatal("binding a deregistered cluster must conflict")
	}
}

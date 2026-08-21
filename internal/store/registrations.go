package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"

	"github.com/tonymontoya/ceph-atlas/internal/actor"
	"github.com/tonymontoya/ceph-atlas/internal/apperr"
	"github.com/tonymontoya/ceph-atlas/internal/fleet"
)

// EnrollmentCredentialTTL bounds how long a registration's one-time
// credential may sit unused before the operator must re-register.
const EnrollmentCredentialTTL = 24 * time.Hour

const enrollmentTokenPrefix = "atl_enroll_"

const clusterRegistrationColumns = "id, fsid::text, name, ceph_version, cluster_type, registered_at, registered_by, deregistered_at"

type ClusterRegistrationInput struct {
	Name        string
	ClusterType string
	Actor       actor.Actor
}

type DeregisterClusterInput struct {
	ClusterID int64
	Actor     actor.Actor
}

func conflictError(message string) apperr.Error {
	return apperr.Error{Class: apperr.Conflict, Message: message}
}

// CreateClusterRegistration registers a Cluster (ADR-0025) and mints its
// one-time Enrollment Credential. The token is returned exactly once and
// exists in the database only as a SHA-256 hash.
func (s *PostgresStore) CreateClusterRegistration(ctx context.Context, input ClusterRegistrationInput) (fleet.ClusterRegistration, fleet.EnrollmentCredential, error) {
	if input.Name == "" {
		return fleet.ClusterRegistration{}, fleet.EnrollmentCredential{}, inputError("name is required")
	}
	if input.ClusterType != string(fleet.ClusterTypeBareMetal) {
		return fleet.ClusterRegistration{}, fleet.EnrollmentCredential{}, inputError("clusterType must be bare-metal")
	}
	if err := validateActor(input.Actor); err != nil {
		return fleet.ClusterRegistration{}, fleet.EnrollmentCredential{}, err
	}

	token, err := newEnrollmentToken()
	if err != nil {
		return fleet.ClusterRegistration{}, fleet.EnrollmentCredential{}, err
	}

	occurredAt := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fleet.ClusterRegistration{}, fleet.EnrollmentCredential{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	row := tx.QueryRowContext(ctx, `
		INSERT INTO atlas_clusters (name, cluster_type, registered_at, registered_by)
		VALUES ($1, $2, $3, $4)
		RETURNING `+clusterRegistrationColumns,
		input.Name, input.ClusterType, occurredAt, input.Actor.Subject)
	registration, err := scanClusterRegistration(row)
	if err != nil {
		return fleet.ClusterRegistration{}, fleet.EnrollmentCredential{}, err
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO cluster_enrollment_credentials (cluster_id, credential_hash, expires_at)
		VALUES ($1, $2, $3)
	`, registration.ID, hashEnrollmentToken(token), occurredAt.Add(EnrollmentCredentialTTL))
	if err != nil {
		return fleet.ClusterRegistration{}, fleet.EnrollmentCredential{}, err
	}

	if err := tx.Commit(); err != nil {
		return fleet.ClusterRegistration{}, fleet.EnrollmentCredential{}, err
	}
	return registration, fleet.EnrollmentCredential{Token: token, ExpiresAt: occurredAt.Add(EnrollmentCredentialTTL)}, nil
}

// GetClusterRegistration returns a registration by cluster id, including
// deregistered clusters.
func (s *PostgresStore) GetClusterRegistration(ctx context.Context, clusterID int64) (fleet.ClusterRegistration, error) {
	if clusterID <= 0 {
		return fleet.ClusterRegistration{}, notFound("cluster not found")
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT `+clusterRegistrationColumns+`
		FROM atlas_clusters
		WHERE id = $1
	`, clusterID)
	registration, err := scanClusterRegistration(row)
	if errors.Is(err, sql.ErrNoRows) {
		return fleet.ClusterRegistration{}, notFound("cluster not found")
	}
	if err != nil {
		return fleet.ClusterRegistration{}, err
	}
	return registration, nil
}

// DeregisterCluster retires a registration: the row and its history stay,
// every live Enrollment Credential is consumed, and pushed observations or
// Cases are preserved untouched.
func (s *PostgresStore) DeregisterCluster(ctx context.Context, input DeregisterClusterInput) (fleet.ClusterRegistration, error) {
	if err := validateActor(input.Actor); err != nil {
		return fleet.ClusterRegistration{}, err
	}

	occurredAt := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fleet.ClusterRegistration{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	row := tx.QueryRowContext(ctx, `
		UPDATE atlas_clusters
		SET deregistered_at = $2, updated_at = $2
		WHERE id = $1 AND deregistered_at IS NULL
		RETURNING `+clusterRegistrationColumns,
		input.ClusterID, occurredAt)
	registration, err := scanClusterRegistration(row)
	if errors.Is(err, sql.ErrNoRows) {
		// The UPDATE only misses when the row is gone (notFound) or
		// already deregistered (conflict); the WHERE pinned
		// deregistered_at IS NULL, so an existing row here has it set.
		_, getErr := s.GetClusterRegistration(ctx, input.ClusterID)
		if getErr != nil {
			return fleet.ClusterRegistration{}, getErr
		}
		return fleet.ClusterRegistration{}, conflictError("cluster is already deregistered")
	}
	if err != nil {
		return fleet.ClusterRegistration{}, err
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE cluster_enrollment_credentials
		SET consumed_at = $2
		WHERE cluster_id = $1 AND consumed_at IS NULL
	`, input.ClusterID, occurredAt); err != nil {
		return fleet.ClusterRegistration{}, err
	}

	if err := tx.Commit(); err != nil {
		return fleet.ClusterRegistration{}, err
	}
	return registration, nil
}

// BindClusterFSID binds a registration's FSID at Enrollment (ADR-0026):
// NULLable until first bind, immutable after, and unique across clusters.
func (s *PostgresStore) BindClusterFSID(ctx context.Context, clusterID int64, fsid string) (fleet.ClusterRegistration, error) {
	if clusterID <= 0 {
		return fleet.ClusterRegistration{}, notFound("cluster not found")
	}
	if !IsUUIDShape(fsid) {
		return fleet.ClusterRegistration{}, inputError("fsid must be a UUID")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fleet.ClusterRegistration{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	registration, err := bindClusterFSIDInTx(ctx, tx, clusterID, fsid)
	if err != nil {
		return fleet.ClusterRegistration{}, err
	}
	if err := tx.Commit(); err != nil {
		return fleet.ClusterRegistration{}, err
	}
	return registration, nil
}

func newEnrollmentToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return enrollmentTokenPrefix + base64.RawURLEncoding.EncodeToString(raw), nil
}

func hashEnrollmentToken(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

func scanClusterRegistration(row rowScanner) (fleet.ClusterRegistration, error) {
	var registration fleet.ClusterRegistration
	var fsid sql.NullString
	var cephVersion sql.NullString
	var registeredAt sql.NullTime
	var registeredBy sql.NullString
	var deregisteredAt sql.NullTime
	err := row.Scan(
		&registration.ID,
		&fsid,
		&registration.Name,
		&cephVersion,
		&registration.Type,
		&registeredAt,
		&registeredBy,
		&deregisteredAt,
	)
	if err != nil {
		return fleet.ClusterRegistration{}, err
	}
	registration.FSID = nullableString(fsid)
	registration.CephVersion = nullableString(cephVersion)
	if registeredAt.Valid {
		stamp := registeredAt.Time
		registration.RegisteredAt = &stamp
	}
	registration.RegisteredBy = registeredBy.String
	if deregisteredAt.Valid {
		stamp := deregisteredAt.Time
		registration.DeregisteredAt = &stamp
	}
	return registration, nil
}

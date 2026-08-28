package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/tonymontoya/ceph-atlas/internal/apperr"
	"github.com/tonymontoya/ceph-atlas/internal/ca"
	"github.com/tonymontoya/ceph-atlas/internal/fleet"
)

type EnrollAgentInput struct {
	CredentialToken string
	FSID            string
}

type EnrollmentResult struct {
	Cluster     fleet.ClusterRegistration
	Certificate ca.IssuedCertificate
}

// IssueCertificate signs the Agent's client certificate inside the
// enrollment transaction. Atlas calls it after the credential is
// consumed and the FSID bound; a signing failure rolls the whole
// enrollment back, so a failed handshake never burns the credential.
type IssueCertificate func() (ca.IssuedCertificate, error)

func unauthorizedError(message string) apperr.Error {
	return apperr.Error{Class: apperr.Unauthorized, Message: message}
}

// EnrollAgent performs the enrollment handshake (ADR-0026) in one
// transaction: burn the one-time Enrollment Credential, bind the
// Cluster's self-reported FSID, sign the Agent's certificate, and
// record its serial number so the certificate maps to exactly one
// registered Cluster.
func (s *PostgresStore) EnrollAgent(ctx context.Context, input EnrollAgentInput, issue IssueCertificate) (EnrollmentResult, error) {
	if input.CredentialToken == "" {
		return EnrollmentResult{}, inputError("credentialToken is required")
	}
	if !IsUUIDShape(input.FSID) {
		return EnrollmentResult{}, inputError("fsid must be a UUID")
	}

	occurredAt := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return EnrollmentResult{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	// Consume the credential atomically: the pinned consumed_at IS NULL
	// makes replay lose the race by construction, and the expiry check
	// runs in the same predicate. The single generic message keeps the
	// endpoint from leaking which part failed.
	var clusterID int64
	err = tx.QueryRowContext(ctx, `
		UPDATE cluster_enrollment_credentials
		SET consumed_at = $2
		WHERE credential_hash = $1 AND consumed_at IS NULL AND expires_at > $2
		RETURNING cluster_id
	`, hashEnrollmentToken(input.CredentialToken), occurredAt).Scan(&clusterID)
	if errors.Is(err, sql.ErrNoRows) {
		return EnrollmentResult{}, unauthorizedError("enrollment credential is invalid, expired, or already used")
	}
	if err != nil {
		return EnrollmentResult{}, err
	}

	registration, err := bindClusterFSIDInTx(ctx, tx, clusterID, input.FSID)
	if err != nil {
		return EnrollmentResult{}, err
	}

	certificate, err := issue()
	if err != nil {
		return EnrollmentResult{}, err
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO cluster_agent_certificates (cluster_id, serial_number, fingerprint, common_name, not_before, not_after, issued_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, clusterID, certificate.SerialNumber, certificate.Fingerprint, certificate.CommonName, certificate.NotBefore, certificate.NotAfter, occurredAt)
	if err != nil {
		return EnrollmentResult{}, err
	}

	if err := tx.Commit(); err != nil {
		return EnrollmentResult{}, err
	}
	return EnrollmentResult{Cluster: registration, Certificate: certificate}, nil
}

// bindClusterFSIDInTx is the in-transaction core of BindClusterFSID:
// FSID binds once, immutably, and uniquely across clusters. A fresh
// enrollment of a physical cluster whose FSID a deregistered row still
// holds releases that stale claim first (#44, ADR-0026 amendment):
// total uniqueness stays, the release touches deregistered rows only,
// and a live holder keeps blocking the bind.
func bindClusterFSIDInTx(ctx context.Context, tx *sql.Tx, clusterID int64, fsid string) (fleet.ClusterRegistration, error) {
	if _, err := tx.ExecContext(ctx, `
		UPDATE atlas_clusters
		SET fsid = NULL, updated_at = now()
		WHERE fsid = $1::uuid AND deregistered_at IS NOT NULL
	`, strings.ToLower(fsid)); err != nil {
		return fleet.ClusterRegistration{}, err
	}

	row := tx.QueryRowContext(ctx, `
		UPDATE atlas_clusters
		SET fsid = $2, updated_at = now()
		WHERE id = $1 AND fsid IS NULL AND deregistered_at IS NULL
		RETURNING `+clusterRegistrationColumns,
		clusterID, strings.ToLower(fsid))
	registration, err := scanClusterRegistration(row)
	if errors.Is(err, sql.ErrNoRows) {
		existing, getErr := scanClusterRegistration(tx.QueryRowContext(ctx, `
			SELECT `+clusterRegistrationColumns+`
			FROM atlas_clusters
			WHERE id = $1
		`, clusterID))
		if getErr != nil {
			return fleet.ClusterRegistration{}, notFound("cluster not found")
		}
		if existing.DeregisteredAt != nil {
			return fleet.ClusterRegistration{}, conflictError("cluster is deregistered and cannot be enrolled")
		}
		return fleet.ClusterRegistration{}, conflictError("cluster fsid is already bound")
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return fleet.ClusterRegistration{}, conflictError("fsid is already registered to another cluster")
		}
		return fleet.ClusterRegistration{}, err
	}
	return registration, nil
}

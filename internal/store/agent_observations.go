package store

import (
	"context"
	"database/sql"
	"errors"
)

// AgentCluster is the Cluster an enrolled Agent certificate maps to
// (ADR-0026): attribution for pushed observations comes from this
// resolution, never from payload claims.
type AgentCluster struct {
	ClusterID int64
	FSID      string
}

// ResolveAgentCluster maps a client-certificate serial number to the
// one registered Cluster the certificate was issued for. A serial that
// is unknown, revoked, outside its validity window, or attached to a
// deregistered cluster is reported with one generic Unauthorized
// message, so callers cannot use the result to probe which condition
// matched.
func (s *PostgresStore) ResolveAgentCluster(ctx context.Context, serialNumber string) (AgentCluster, error) {
	if serialNumber == "" {
		return AgentCluster{}, inputError("serial number is required")
	}
	var resolved AgentCluster
	err := s.db.QueryRowContext(ctx, `
		SELECT clusters.id, clusters.fsid::text
		FROM cluster_agent_certificates AS certificates
		JOIN atlas_clusters AS clusters ON clusters.id = certificates.cluster_id
		WHERE certificates.serial_number = $1
			AND certificates.revoked_at IS NULL
			AND certificates.not_before <= now()
			AND certificates.not_after > now()
			AND clusters.deregistered_at IS NULL
			AND clusters.fsid IS NOT NULL
	`, serialNumber).Scan(&resolved.ClusterID, &resolved.FSID)
	if errors.Is(err, sql.ErrNoRows) {
		return AgentCluster{}, unauthorizedError("client certificate is not valid for an enrolled cluster")
	}
	if err != nil {
		return AgentCluster{}, err
	}
	return resolved, nil
}

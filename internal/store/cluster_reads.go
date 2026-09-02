package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/tonymontoya/ceph-atlas/internal/apperr"
	"github.com/tonymontoya/ceph-atlas/internal/fleet"
	"github.com/tonymontoya/ceph-atlas/internal/inventory"
	"github.com/tonymontoya/ceph-atlas/internal/inventory/entities"
)

// ListClustersQuery scopes the cluster index: a case-insensitive
// substring Search over name and FSID, plus limit/offset paging.
type ListClustersQuery struct {
	Search string
	Limit  int
	Offset int
}

// ClusterSummary is one row of the cluster index: the registration
// fields Operators address a cluster by, the latest observed health,
// and when an enrolled Agent last pushed (nil when no agent batch has
// arrived). ID is nil in provider mode, where the index serves a
// provider's implicit cluster rather than a registration row.
type ClusterSummary struct {
	ID            *int64            `json:"id"`
	FSID          *string           `json:"fsid"`
	Name          string            `json:"name"`
	ClusterType   fleet.ClusterType `json:"clusterType"`
	CephVersion   *string           `json:"cephVersion"`
	HealthStatus  *string           `json:"healthStatus"`
	HealthSummary *string           `json:"healthSummary"`
	AgentLastSeen *time.Time        `json:"agentLastSeen"`
	// AgentLastPushAt is when Atlas last received an Agent push — the
	// sync run's server-side start, as distinct from AgentLastSeen's
	// observation timestamp, which is the Agent's own clock.
	AgentLastPushAt *time.Time `json:"agentLastPushAt"`
}

// ClusterIndex is one page of the cluster index plus the total the
// query matches.
type ClusterIndex struct {
	Clusters []ClusterSummary `json:"clusters"`
	Total    int              `json:"total"`
	Limit    int              `json:"limit"`
	Offset   int              `json:"offset"`
}

// normalizeClusterFSID lowers and validates an FSID route or filter
// value. A malformed FSID addresses no cluster, so it reports NotFound
// rather than a cast failure.
func normalizeClusterFSID(fsid string) (string, bool) {
	fsid = strings.ToLower(fsid)
	if !IsUUIDShape(fsid) {
		return "", false
	}
	return fsid, true
}

func (s *PostgresStore) clusterExistsByFSID(ctx context.Context, fsid string) (bool, error) {
	normalized, ok := normalizeClusterFSID(fsid)
	if !ok {
		return false, nil
	}
	var exists bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM atlas_clusters WHERE fsid = $1::uuid)
	`, normalized).Scan(&exists)
	return exists, err
}

// ClampPage applies the shared paging contract: limits land in 1-100
// with 50 the default, offsets stay non-negative.
func ClampPage(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

// ListClusterSummaries returns one page of the active-cluster index
// (deregistered clusters never list) with each cluster's latest
// observed health and Agent last-seen time.
func (s *PostgresStore) ListClusterSummaries(ctx context.Context, query ListClustersQuery) (ClusterIndex, error) {
	query.Limit, query.Offset = ClampPage(query.Limit, query.Offset)
	search := strings.ToLower(query.Search)
	// strpos substring matching keeps user input literal: no LIKE
	// wildcard escaping to get wrong. $1 is the search term in both
	// the count and the page query; columns are cluster-qualified so
	// the joined health view cannot make them ambiguous.
	predicate := `clusters.deregistered_at IS NULL
		AND ($1::text = ''
			OR strpos(lower(clusters.name), $1) > 0
			OR strpos(clusters.fsid::text, $1) > 0)`

	var total int
	if err := s.db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM atlas_clusters AS clusters
		WHERE `+predicate, search).Scan(&total); err != nil {
		return ClusterIndex{}, err
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT clusters.id, clusters.fsid::text, clusters.name, clusters.cluster_type, clusters.ceph_version,
			health.status, health.summary, agent.last_seen, agent_push.last_push
		FROM atlas_clusters AS clusters
		LEFT JOIN cluster_current_health AS health
			ON health.fsid = clusters.fsid
		LEFT JOIN LATERAL (
			SELECT max(observed_at) AS last_seen
			FROM inventory_snapshots
			WHERE cluster_id = clusters.id AND provider = 'agent'
		) AS agent ON true
		LEFT JOIN LATERAL (
			SELECT max(started_at) AS last_push
			FROM inventory_sync_runs
			WHERE cluster_id = clusters.id AND provider = 'agent'
		) AS agent_push ON true
		WHERE `+predicate+`
		ORDER BY clusters.name, clusters.id
		LIMIT $2 OFFSET $3
	`, search, query.Limit, query.Offset)
	if err != nil {
		return ClusterIndex{}, err
	}
	defer func() { _ = rows.Close() }()

	index := ClusterIndex{Clusters: make([]ClusterSummary, 0), Total: total, Limit: query.Limit, Offset: query.Offset}
	for rows.Next() {
		var summary ClusterSummary
		var id int64
		var fsid sql.NullString
		var cephVersion, healthStatus, healthSummary sql.NullString
		var agentLastSeen, agentLastPushAt sql.NullTime
		if err := rows.Scan(
			&id,
			&fsid,
			&summary.Name,
			&summary.ClusterType,
			&cephVersion,
			&healthStatus,
			&healthSummary,
			&agentLastSeen,
			&agentLastPushAt,
		); err != nil {
			return ClusterIndex{}, err
		}
		summary.ID = &id
		if fsid.Valid {
			value := fsid.String
			summary.FSID = &value
		}
		if cephVersion.Valid {
			value := cephVersion.String
			summary.CephVersion = &value
		}
		if healthStatus.Valid {
			value := healthStatus.String
			summary.HealthStatus = &value
		}
		if healthSummary.Valid {
			value := healthSummary.String
			summary.HealthSummary = &value
		}
		if agentLastSeen.Valid {
			value := agentLastSeen.Time
			summary.AgentLastSeen = &value
		}
		if agentLastPushAt.Valid {
			value := agentLastPushAt.Time
			summary.AgentLastPushAt = &value
		}
		index.Clusters = append(index.Clusters, summary)
	}
	if err := rows.Err(); err != nil {
		return ClusterIndex{}, err
	}
	return index, nil
}

// clusterFSIDNotFound reports the shared unknown-cluster failure every
// scoped read leads with.
func clusterFSIDNotFound() error {
	return notFound("cluster not found")
}

func (s *PostgresStore) ClusterHealth(ctx context.Context, fsid string) (inventory.Health, error) {
	normalized, ok := normalizeClusterFSID(fsid)
	if !ok {
		return inventory.Health{}, clusterFSIDNotFound()
	}
	var health inventory.Health
	var checksJSON []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT status, summary, checks
		FROM cluster_current_health
		WHERE fsid = $1::uuid
	`, normalized).Scan(&health.Status, &health.Summary, &checksJSON)
	if errors.Is(err, sql.ErrNoRows) {
		exists, existsErr := s.clusterExistsByFSID(ctx, fsid)
		if existsErr != nil {
			return inventory.Health{}, existsErr
		}
		if !exists {
			return inventory.Health{}, clusterFSIDNotFound()
		}
		return inventory.Health{}, notFound("cluster health not found")
	}
	if err != nil {
		return inventory.Health{}, err
	}
	if err := jsonUnmarshalChecks(checksJSON, &health.Checks); err != nil {
		return inventory.Health{}, err
	}
	return health, nil
}

// clusterRows serves one declared entity's latest-snapshot rows for
// the cluster the FSID addresses. A malformed or unknown FSID reports
// the shared cluster not-found; a known cluster whose latest snapshot
// lacks the entity reports the entity's declared not-found message
// (ADR-0014).
func clusterRows[T any](ctx context.Context, s *PostgresStore, fsid string, entity entities.Entity, scan func(rowScanner) (T, error)) ([]T, error) {
	normalized, ok := normalizeClusterFSID(fsid)
	if !ok {
		return nil, clusterFSIDNotFound()
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+entity.Columns+`
		FROM `+entity.View+`
		WHERE fsid = $1::uuid
		ORDER BY `+entity.OrderBy+`
	`, normalized)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]T, 0)
	for rows.Next() {
		value, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, s.scopedNotFound(ctx, fsid, entity.NotFound)
	}
	return out, nil
}

func scanStorageDevice(scanner rowScanner) (inventory.StorageDevice, error) {
	var device inventory.StorageDevice
	var deviceType sql.NullString
	var devicePath sql.NullString
	var deviceHealth sql.NullString
	var osdID sql.NullInt64
	if err := scanner.Scan(
		&device.Host,
		&device.Serial,
		&deviceType,
		&devicePath,
		&deviceHealth,
		&osdID,
	); err != nil {
		return inventory.StorageDevice{}, err
	}
	if deviceType.Valid {
		device.Type = deviceType.String
	}
	if devicePath.Valid {
		device.Path = devicePath.String
	}
	if deviceHealth.Valid {
		device.Health = deviceHealth.String
	}
	if osdID.Valid {
		id := int(osdID.Int64)
		device.OSDID = &id
	}
	return device, nil
}

func scanPool(scanner rowScanner) (inventory.Pool, error) {
	var pool inventory.Pool
	var size sql.NullInt64
	var minSize sql.NullInt64
	if err := scanner.Scan(
		&pool.ID,
		&pool.Name,
		&pool.Type,
		&size,
		&minSize,
	); err != nil {
		return inventory.Pool{}, err
	}
	if size.Valid {
		value := int(size.Int64)
		pool.Size = &value
	}
	if minSize.Valid {
		value := int(minSize.Int64)
		pool.MinSize = &value
	}
	return pool, nil
}

func scanOSD(scanner rowScanner) (inventory.OSD, error) {
	var osd inventory.OSD
	var device sql.NullString
	if err := scanner.Scan(&osd.ID, &osd.Host, &osd.Up, &osd.In, &device); err != nil {
		return inventory.OSD{}, err
	}
	if device.Valid {
		osd.Device = device.String
	}
	return osd, nil
}

func scanHost(scanner rowScanner) (inventory.Host, error) {
	var host inventory.Host
	var address sql.NullString
	if err := scanner.Scan(&host.Name, &address); err != nil {
		return inventory.Host{}, err
	}
	if address.Valid {
		host.Address = address.String
	}
	return host, nil
}

func scanDaemon(scanner rowScanner) (inventory.Daemon, error) {
	var daemon inventory.Daemon
	var version sql.NullString
	if err := scanner.Scan(&daemon.Type, &daemon.Name, &daemon.Host, &daemon.Status, &version); err != nil {
		return inventory.Daemon{}, err
	}
	if version.Valid {
		daemon.Version = version.String
	}
	return daemon, nil
}

// The list-shaped cluster reads delegate to the declaration-driven
// helper; signatures stay on the interface (completeness is enforced
// by TestEveryDeclaredEntityHasStoreReadMethod).

func (s *PostgresStore) ClusterOSDs(ctx context.Context, fsid string) ([]inventory.OSD, error) {
	return clusterRows(ctx, s, fsid, entities.OSDs, scanOSD)
}

func (s *PostgresStore) ClusterHosts(ctx context.Context, fsid string) ([]inventory.Host, error) {
	return clusterRows(ctx, s, fsid, entities.Hosts, scanHost)
}

func (s *PostgresStore) ClusterStorageDevices(ctx context.Context, fsid string) ([]inventory.StorageDevice, error) {
	return clusterRows(ctx, s, fsid, entities.StorageDevices, scanStorageDevice)
}

func (s *PostgresStore) ClusterDaemons(ctx context.Context, fsid string) ([]inventory.Daemon, error) {
	return clusterRows(ctx, s, fsid, entities.Daemons, scanDaemon)
}

func (s *PostgresStore) ClusterPools(ctx context.Context, fsid string) ([]inventory.Pool, error) {
	return clusterRows(ctx, s, fsid, entities.Pools, scanPool)
}

// scopedNotFound distinguishes an unknown cluster from a known cluster
// whose latest snapshot lacks the entity.
func (s *PostgresStore) scopedNotFound(ctx context.Context, fsid, entityMessage string) error {
	exists, err := s.clusterExistsByFSID(ctx, fsid)
	if err != nil {
		return err
	}
	if !exists {
		return clusterFSIDNotFound()
	}
	return notFound(entityMessage)
}

func jsonUnmarshalChecks(data []byte, checks *[]inventory.HealthCheck) error {
	if len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, checks); err != nil {
		return apperr.Error{
			Class:   apperr.MalformedResponse,
			Message: "parse cluster health checks: " + err.Error(),
		}
	}
	return nil
}

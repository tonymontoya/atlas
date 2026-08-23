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
			health.status, health.summary, agent.last_seen
		FROM atlas_clusters AS clusters
		LEFT JOIN cluster_current_health AS health
			ON health.fsid = clusters.fsid
		LEFT JOIN LATERAL (
			SELECT max(observed_at) AS last_seen
			FROM inventory_snapshots
			WHERE cluster_id = clusters.id AND provider = 'agent'
		) AS agent ON true
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
		var agentLastSeen sql.NullTime
		if err := rows.Scan(
			&id,
			&fsid,
			&summary.Name,
			&summary.ClusterType,
			&cephVersion,
			&healthStatus,
			&healthSummary,
			&agentLastSeen,
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

func (s *PostgresStore) ClusterOSDs(ctx context.Context, fsid string) ([]inventory.OSD, error) {
	normalized, ok := normalizeClusterFSID(fsid)
	if !ok {
		return nil, clusterFSIDNotFound()
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT osd_id, host, osd_up, osd_in, device
		FROM cluster_current_osds
		WHERE fsid = $1::uuid
		ORDER BY osd_id
	`, normalized)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var osds []inventory.OSD
	for rows.Next() {
		var osd inventory.OSD
		var device sql.NullString
		if err := rows.Scan(&osd.ID, &osd.Host, &osd.Up, &osd.In, &device); err != nil {
			return nil, err
		}
		if device.Valid {
			osd.Device = device.String
		}
		osds = append(osds, osd)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(osds) == 0 {
		return nil, s.scopedNotFound(ctx, fsid, "current OSD inventory not found")
	}
	return osds, nil
}

func (s *PostgresStore) ClusterHosts(ctx context.Context, fsid string) ([]inventory.Host, error) {
	normalized, ok := normalizeClusterFSID(fsid)
	if !ok {
		return nil, clusterFSIDNotFound()
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT host_name, address
		FROM cluster_current_hosts
		WHERE fsid = $1::uuid
		ORDER BY host_name
	`, normalized)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var hosts []inventory.Host
	for rows.Next() {
		var host inventory.Host
		var address sql.NullString
		if err := rows.Scan(&host.Name, &address); err != nil {
			return nil, err
		}
		if address.Valid {
			host.Address = address.String
		}
		hosts = append(hosts, host)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(hosts) == 0 {
		return nil, s.scopedNotFound(ctx, fsid, "current host inventory not found")
	}
	return hosts, nil
}

func (s *PostgresStore) ClusterStorageDevices(ctx context.Context, fsid string) ([]inventory.StorageDevice, error) {
	normalized, ok := normalizeClusterFSID(fsid)
	if !ok {
		return nil, clusterFSIDNotFound()
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT host_name, serial, device_type, device_path, device_health, osd_id
		FROM cluster_current_storage_devices
		WHERE fsid = $1::uuid
		ORDER BY host_name, serial
	`, normalized)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	devices := make([]inventory.StorageDevice, 0)
	for rows.Next() {
		device, err := scanStorageDevice(rows)
		if err != nil {
			return nil, err
		}
		devices = append(devices, device)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(devices) == 0 {
		return nil, s.scopedNotFound(ctx, fsid, "current storage device inventory not found")
	}
	return devices, nil
}

func (s *PostgresStore) ClusterDaemons(ctx context.Context, fsid string) ([]inventory.Daemon, error) {
	normalized, ok := normalizeClusterFSID(fsid)
	if !ok {
		return nil, clusterFSIDNotFound()
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT daemon_type, daemon_name, host_name, status, ceph_version
		FROM cluster_current_daemons
		WHERE fsid = $1::uuid
		ORDER BY daemon_type, daemon_name
	`, normalized)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var daemons []inventory.Daemon
	for rows.Next() {
		var daemon inventory.Daemon
		var version sql.NullString
		if err := rows.Scan(&daemon.Type, &daemon.Name, &daemon.Host, &daemon.Status, &version); err != nil {
			return nil, err
		}
		if version.Valid {
			daemon.Version = version.String
		}
		daemons = append(daemons, daemon)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(daemons) == 0 {
		return nil, s.scopedNotFound(ctx, fsid, "current Ceph Daemon inventory not found")
	}
	return daemons, nil
}

func (s *PostgresStore) ClusterPools(ctx context.Context, fsid string) ([]inventory.Pool, error) {
	normalized, ok := normalizeClusterFSID(fsid)
	if !ok {
		return nil, clusterFSIDNotFound()
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT pool_id, name, pool_type, size, min_size
		FROM cluster_current_pools
		WHERE fsid = $1::uuid
		ORDER BY pool_id
	`, normalized)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	pools := make([]inventory.Pool, 0)
	for rows.Next() {
		pool, err := scanPool(rows)
		if err != nil {
			return nil, err
		}
		pools = append(pools, pool)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(pools) == 0 {
		return nil, s.scopedNotFound(ctx, fsid, "current Pool inventory not found")
	}
	return pools, nil
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

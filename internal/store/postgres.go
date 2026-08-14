package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/tonymontoya/ceph-atlas/internal/cases"
	"github.com/tonymontoya/ceph-atlas/internal/fleet"
	"github.com/tonymontoya/ceph-atlas/internal/inventory"
	"github.com/tonymontoya/ceph-atlas/internal/providers"
)

type InventoryObservation struct {
	Provider   string
	Scenario   string
	ObservedAt time.Time
	Cluster    fleet.ClusterIdentity
	Health     inventory.Health
	OSDs       []inventory.OSD
	Hosts      []inventory.Host
	Devices    []inventory.StorageDevice
	Daemons    []inventory.Daemon
	Pools      []inventory.Pool
	Metadata   json.RawMessage
}

type SaveInventoryResult struct {
	ClusterID  int64
	SnapshotID int64
}

type BeginSyncRun struct {
	Provider string
	Scenario string
}

type SyncRunFailure struct {
	RunID        int64
	ErrorClass   string
	ErrorMessage string
}

type SyncRunResult struct {
	RunID      int64
	SnapshotID int64
}

type InventorySyncRun struct {
	ID           int64      `json:"id"`
	Provider     string     `json:"provider"`
	Scenario     string     `json:"scenario,omitempty"`
	Status       string     `json:"status"`
	StartedAt    time.Time  `json:"startedAt"`
	FinishedAt   *time.Time `json:"finishedAt,omitempty"`
	SnapshotID   *int64     `json:"snapshotId,omitempty"`
	ErrorClass   string     `json:"errorClass,omitempty"`
	ErrorMessage string     `json:"errorMessage,omitempty"`
}

const currentSnapshotCTE = `WITH current_cluster AS (
	SELECT id AS snapshot_id
	FROM inventory_snapshots
	ORDER BY observed_at DESC, id DESC
	LIMIT 1
)
`

type PostgresStore struct {
	db *sql.DB
}

func OpenPostgres(ctx context.Context, databaseURL string) (*sql.DB, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func NewPostgres(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

func (s *PostgresStore) ListCases(ctx context.Context, limit int) ([]cases.Case, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, title, summary, status, severity, source, cluster_fsid::text, created_at, updated_at, closed_at
		FROM cases
		ORDER BY updated_at DESC, id DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	result := make([]cases.Case, 0)
	for rows.Next() {
		item, err := scanCase(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *PostgresStore) GetCase(ctx context.Context, id int64) (cases.Case, error) {
	if id <= 0 {
		return cases.Case{}, notFound("case not found")
	}

	row := s.db.QueryRowContext(ctx, `
		SELECT id, title, summary, status, severity, source, cluster_fsid::text, created_at, updated_at, closed_at
		FROM cases
		WHERE id = $1
	`, id)
	item, err := scanCase(row)
	if errors.Is(err, sql.ErrNoRows) {
		return cases.Case{}, notFound("case not found")
	}
	if err != nil {
		return cases.Case{}, err
	}
	return item, nil
}

func (s *PostgresStore) ListCaseTimeline(ctx context.Context, caseID int64) ([]cases.TimelineEvent, error) {
	if caseID <= 0 {
		return nil, notFound("case not found")
	}

	var exists bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM cases
			WHERE id = $1
		)
	`, caseID).Scan(&exists)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, notFound("case not found")
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, case_id, event_type, message, occurred_at, actor_type, actor_id, actor_display_name, payload
		FROM case_timeline_events
		WHERE case_id = $1
		ORDER BY occurred_at ASC, id ASC
	`, caseID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	result := make([]cases.TimelineEvent, 0)
	for rows.Next() {
		item, err := scanTimelineEvent(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *PostgresStore) BeginInventorySyncRun(ctx context.Context, run BeginSyncRun) (int64, error) {
	if run.Provider == "" {
		return 0, errors.New("provider is required")
	}
	var runID int64
	scenario := sql.NullString{String: run.Scenario, Valid: run.Scenario != ""}
	err := s.db.QueryRowContext(ctx, `
		INSERT INTO inventory_sync_runs (provider, scenario, status)
		VALUES ($1, $2, 'running')
		RETURNING id
	`, run.Provider, scenario).Scan(&runID)
	return runID, err
}

func (s *PostgresStore) SucceedInventorySyncRun(ctx context.Context, result SyncRunResult) error {
	commandTag, err := s.db.ExecContext(ctx, `
		UPDATE inventory_sync_runs
		SET status = 'succeeded',
			finished_at = now(),
			snapshot_id = $2,
			error_class = NULL,
			error_message = NULL
		WHERE id = $1 AND status = 'running'
	`, result.RunID, result.SnapshotID)
	if err != nil {
		return err
	}
	rows, err := commandTag.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return errors.New("running sync run not found")
	}
	return nil
}

func (s *PostgresStore) FailInventorySyncRun(ctx context.Context, failure SyncRunFailure) error {
	if failure.ErrorClass == "" {
		return errors.New("error class is required")
	}
	if failure.ErrorMessage == "" {
		return errors.New("error message is required")
	}
	commandTag, err := s.db.ExecContext(ctx, `
		UPDATE inventory_sync_runs
		SET status = 'failed',
			finished_at = now(),
			error_class = $2,
			error_message = $3
		WHERE id = $1 AND status = 'running'
	`, failure.RunID, failure.ErrorClass, failure.ErrorMessage)
	if err != nil {
		return err
	}
	rows, err := commandTag.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return errors.New("running sync run not found")
	}
	return nil
}

func (s *PostgresStore) ListInventorySyncRuns(ctx context.Context, limit int) ([]InventorySyncRun, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, provider, scenario, status, started_at, finished_at, snapshot_id, error_class, error_message
		FROM inventory_sync_runs
		ORDER BY started_at DESC, id DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	runs := make([]InventorySyncRun, 0)
	for rows.Next() {
		var run InventorySyncRun
		var scenario sql.NullString
		var finishedAt sql.NullTime
		var snapshotID sql.NullInt64
		var errorClass sql.NullString
		var errorMessage sql.NullString
		if err := rows.Scan(
			&run.ID,
			&run.Provider,
			&scenario,
			&run.Status,
			&run.StartedAt,
			&finishedAt,
			&snapshotID,
			&errorClass,
			&errorMessage,
		); err != nil {
			return nil, err
		}
		if scenario.Valid {
			run.Scenario = scenario.String
		}
		if finishedAt.Valid {
			run.FinishedAt = &finishedAt.Time
		}
		if snapshotID.Valid {
			run.SnapshotID = &snapshotID.Int64
		}
		if errorClass.Valid {
			run.ErrorClass = errorClass.String
		}
		if errorMessage.Valid {
			run.ErrorMessage = errorMessage.String
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return runs, nil
}

func (s *PostgresStore) ClusterIdentity(ctx context.Context) (fleet.ClusterIdentity, error) {
	var identity fleet.ClusterIdentity
	err := s.db.QueryRowContext(ctx, `
		SELECT clusters.fsid::text, clusters.name, clusters.ceph_version, clusters.cluster_type
		FROM atlas_clusters AS clusters
		JOIN inventory_snapshots AS snapshots
			ON snapshots.cluster_id = clusters.id
		ORDER BY snapshots.observed_at DESC, snapshots.id DESC
		LIMIT 1
	`).Scan(&identity.FSID, &identity.Name, &identity.CephVersion, &identity.Type)
	if errors.Is(err, sql.ErrNoRows) {
		return fleet.ClusterIdentity{}, notFound("current cluster identity not found")
	}
	if err != nil {
		return fleet.ClusterIdentity{}, err
	}
	return identity, nil
}

func (s *PostgresStore) Health(ctx context.Context) (inventory.Health, error) {
	var health inventory.Health
	var checksJSON []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT status, summary, checks
		FROM cluster_current_health
		ORDER BY observed_at DESC, completed_at DESC
		LIMIT 1
	`).Scan(&health.Status, &health.Summary, &checksJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return inventory.Health{}, notFound("current cluster health not found")
	}
	if err != nil {
		return inventory.Health{}, err
	}
	if err := json.Unmarshal(checksJSON, &health.Checks); err != nil {
		return inventory.Health{}, providers.ProviderError{
			Class:   providers.ErrorMalformedResponse,
			Message: "parse current cluster health checks: " + err.Error(),
		}
	}
	return health, nil
}

func (s *PostgresStore) OSDs(ctx context.Context) ([]inventory.OSD, error) {
	rows, err := s.db.QueryContext(ctx, currentSnapshotCTE+`		SELECT osds.osd_id, osds.host, osds.osd_up, osds.osd_in, osds.device
		FROM osd_observations AS osds
		WHERE osds.snapshot_id = (SELECT snapshot_id FROM current_cluster)
		ORDER BY osd_id
	`)
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
		return nil, notFound("current OSD inventory not found")
	}
	return osds, nil
}

func (s *PostgresStore) Hosts(ctx context.Context) ([]inventory.Host, error) {
	rows, err := s.db.QueryContext(ctx, currentSnapshotCTE+`		SELECT hosts.host_name, hosts.address
		FROM host_observations AS hosts
		WHERE hosts.snapshot_id = (SELECT snapshot_id FROM current_cluster)
		ORDER BY hosts.host_name
	`)
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
		return nil, notFound("current host inventory not found")
	}
	return hosts, nil
}

func (s *PostgresStore) HostDevices(ctx context.Context, host string) ([]inventory.StorageDevice, error) {
	if host == "" {
		return nil, notFound("host not found")
	}
	var exists bool
	err := s.db.QueryRowContext(ctx, currentSnapshotCTE+`		SELECT EXISTS (
			SELECT 1
			FROM host_observations AS hosts
			WHERE hosts.snapshot_id = (SELECT snapshot_id FROM current_cluster)
				AND hosts.host_name = $1
		)
	`, host).Scan(&exists)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, notFound(fmt.Sprintf("host %q not found", host))
	}

	rows, err := s.db.QueryContext(ctx, currentSnapshotCTE+`		SELECT devices.host_name, devices.serial, devices.device_type, devices.device_path, devices.device_health, devices.osd_id
		FROM storage_device_observations AS devices
		WHERE devices.snapshot_id = (SELECT snapshot_id FROM current_cluster)
			AND devices.host_name = $1
		ORDER BY devices.serial
	`, host)
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
	return devices, nil
}

func (s *PostgresStore) Daemons(ctx context.Context) ([]inventory.Daemon, error) {
	rows, err := s.db.QueryContext(ctx, currentSnapshotCTE+`		SELECT daemons.daemon_type, daemons.daemon_name, daemons.host_name, daemons.status, daemons.ceph_version
		FROM daemon_observations AS daemons
		WHERE daemons.snapshot_id = (SELECT snapshot_id FROM current_cluster)
		ORDER BY daemons.daemon_type, daemons.daemon_name
	`)
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
		return nil, notFound("current Ceph Daemon inventory not found")
	}
	return daemons, nil
}

func (s *PostgresStore) Pools(ctx context.Context) ([]inventory.Pool, error) {
	rows, err := s.db.QueryContext(ctx, currentSnapshotCTE+`		SELECT pools.pool_id, pools.name, pools.pool_type, pools.size, pools.min_size
		FROM pool_observations AS pools
		WHERE pools.snapshot_id = (SELECT snapshot_id FROM current_cluster)
		ORDER BY pools.pool_id
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var pools []inventory.Pool
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
		return nil, notFound("current Pool inventory not found")
	}
	return pools, nil
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

func (s *PostgresStore) SaveInventoryObservation(ctx context.Context, obs InventoryObservation) (SaveInventoryResult, error) {
	obs, err := normalizeInventoryObservation(obs)
	if err != nil {
		return SaveInventoryResult{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SaveInventoryResult{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	clusterID, err := upsertCluster(ctx, tx, obs.Cluster)
	if err != nil {
		return SaveInventoryResult{}, err
	}

	snapshotID, err := insertSnapshot(ctx, tx, clusterID, obs)
	if err != nil {
		return SaveInventoryResult{}, err
	}

	healthObservationID, err := insertHealth(ctx, tx, snapshotID, obs.Health)
	if err != nil {
		return SaveInventoryResult{}, err
	}
	for _, check := range obs.Health.Checks {
		if err := insertHealthCheck(ctx, tx, healthObservationID, check); err != nil {
			return SaveInventoryResult{}, err
		}
	}
	for _, osd := range obs.OSDs {
		if err := insertOSD(ctx, tx, snapshotID, osd); err != nil {
			return SaveInventoryResult{}, err
		}
	}
	for _, host := range obs.Hosts {
		if err := insertHost(ctx, tx, snapshotID, host); err != nil {
			return SaveInventoryResult{}, err
		}
	}
	for _, device := range obs.Devices {
		if err := insertStorageDevice(ctx, tx, snapshotID, device); err != nil {
			return SaveInventoryResult{}, err
		}
	}
	for _, daemon := range obs.Daemons {
		if err := insertDaemon(ctx, tx, snapshotID, daemon); err != nil {
			return SaveInventoryResult{}, err
		}
	}
	for _, pool := range obs.Pools {
		if err := insertPool(ctx, tx, snapshotID, pool); err != nil {
			return SaveInventoryResult{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return SaveInventoryResult{}, err
	}
	return SaveInventoryResult{ClusterID: clusterID, SnapshotID: snapshotID}, nil
}

func notFound(message string) providers.ProviderError {
	return providers.ProviderError{Class: providers.ErrorNotFound, Message: message}
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanCase(scanner rowScanner) (cases.Case, error) {
	var item cases.Case
	var clusterFSID sql.NullString
	var closedAt sql.NullTime
	if err := scanner.Scan(
		&item.ID,
		&item.Title,
		&item.Summary,
		&item.Status,
		&item.Severity,
		&item.Source,
		&clusterFSID,
		&item.CreatedAt,
		&item.UpdatedAt,
		&closedAt,
	); err != nil {
		return cases.Case{}, err
	}
	if clusterFSID.Valid {
		item.ClusterFSID = clusterFSID.String
	}
	if closedAt.Valid {
		item.ClosedAt = &closedAt.Time
	}
	return item, nil
}

func scanTimelineEvent(scanner rowScanner) (cases.TimelineEvent, error) {
	var item cases.TimelineEvent
	var actorID sql.NullString
	var payloadJSON []byte
	if err := scanner.Scan(
		&item.ID,
		&item.CaseID,
		&item.Type,
		&item.Message,
		&item.OccurredAt,
		&item.Actor.Type,
		&actorID,
		&item.Actor.DisplayName,
		&payloadJSON,
	); err != nil {
		return cases.TimelineEvent{}, err
	}
	if actorID.Valid {
		item.Actor.ID = actorID.String
	}
	if len(payloadJSON) == 0 {
		item.Payload = map[string]any{}
		return item, nil
	}
	if err := json.Unmarshal(payloadJSON, &item.Payload); err != nil {
		return cases.TimelineEvent{}, providers.ProviderError{
			Class:   providers.ErrorMalformedResponse,
			Message: "parse case timeline payload: " + err.Error(),
		}
	}
	if item.Payload == nil {
		item.Payload = map[string]any{}
	}
	return item, nil
}

func normalizeInventoryObservation(obs InventoryObservation) (InventoryObservation, error) {
	if obs.Provider == "" {
		return InventoryObservation{}, errors.New("provider is required")
	}
	if obs.ObservedAt.IsZero() {
		return InventoryObservation{}, errors.New("observed_at is required")
	}
	if obs.Cluster.FSID == "" {
		return InventoryObservation{}, errors.New("cluster fsid is required")
	}
	if obs.Cluster.Name == "" {
		return InventoryObservation{}, errors.New("cluster name is required")
	}
	if obs.Cluster.CephVersion == "" {
		return InventoryObservation{}, errors.New("cluster ceph version is required")
	}
	if len(obs.Metadata) == 0 {
		obs.Metadata = json.RawMessage(`{}`)
	}
	if !json.Valid(obs.Metadata) {
		return InventoryObservation{}, errors.New("metadata json is invalid")
	}
	return obs, nil
}

func upsertCluster(ctx context.Context, tx *sql.Tx, cluster fleet.ClusterIdentity) (int64, error) {
	var clusterID int64
	err := tx.QueryRowContext(ctx, `
		INSERT INTO atlas_clusters (fsid, name, ceph_version, cluster_type)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (fsid) DO UPDATE SET
			name = EXCLUDED.name,
			ceph_version = EXCLUDED.ceph_version,
			cluster_type = EXCLUDED.cluster_type,
			updated_at = now()
		RETURNING id
	`, cluster.FSID, cluster.Name, cluster.CephVersion, string(cluster.Type)).Scan(&clusterID)
	return clusterID, err
}

func insertSnapshot(ctx context.Context, tx *sql.Tx, clusterID int64, obs InventoryObservation) (int64, error) {
	var snapshotID int64
	scenario := sql.NullString{String: obs.Scenario, Valid: obs.Scenario != ""}
	err := tx.QueryRowContext(ctx, `
		INSERT INTO inventory_snapshots (cluster_id, provider, scenario, observed_at, metadata)
		VALUES ($1, $2, $3, $4, $5::jsonb)
		RETURNING id
	`, clusterID, obs.Provider, scenario, obs.ObservedAt, string(obs.Metadata)).Scan(&snapshotID)
	return snapshotID, err
}

func insertHealth(ctx context.Context, tx *sql.Tx, snapshotID int64, health inventory.Health) (int64, error) {
	var healthObservationID int64
	err := tx.QueryRowContext(ctx, `
		INSERT INTO cluster_health_observations (snapshot_id, status, summary)
		VALUES ($1, $2, $3)
		RETURNING id
	`, snapshotID, string(health.Status), health.Summary).Scan(&healthObservationID)
	return healthObservationID, err
}

func insertHealthCheck(ctx context.Context, tx *sql.Tx, healthObservationID int64, check inventory.HealthCheck) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO cluster_health_checks (health_observation_id, name, severity, summary)
		VALUES ($1, $2, $3, $4)
	`, healthObservationID, check.Name, check.Severity, check.Summary)
	return err
}

func insertOSD(ctx context.Context, tx *sql.Tx, snapshotID int64, osd inventory.OSD) error {
	device := sql.NullString{String: osd.Device, Valid: osd.Device != ""}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO osd_observations (snapshot_id, osd_id, host, osd_up, osd_in, device)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, snapshotID, osd.ID, osd.Host, osd.Up, osd.In, device)
	return err
}

func insertHost(ctx context.Context, tx *sql.Tx, snapshotID int64, host inventory.Host) error {
	address := sql.NullString{String: host.Address, Valid: host.Address != ""}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO host_observations (snapshot_id, host_name, address)
		VALUES ($1, $2, $3)
	`, snapshotID, host.Name, address)
	return err
}

func insertStorageDevice(ctx context.Context, tx *sql.Tx, snapshotID int64, device inventory.StorageDevice) error {
	deviceType := sql.NullString{String: device.Type, Valid: device.Type != ""}
	devicePath := sql.NullString{String: device.Path, Valid: device.Path != ""}
	deviceHealth := sql.NullString{String: device.Health, Valid: device.Health != ""}
	osdID := sql.NullInt64{}
	if device.OSDID != nil {
		osdID.Int64, osdID.Valid = int64(*device.OSDID), true
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO storage_device_observations (snapshot_id, host_name, serial, device_type, device_path, device_health, osd_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, snapshotID, device.Host, device.Serial, deviceType, devicePath, deviceHealth, osdID)
	return err
}

func insertDaemon(ctx context.Context, tx *sql.Tx, snapshotID int64, daemon inventory.Daemon) error {
	version := sql.NullString{String: daemon.Version, Valid: daemon.Version != ""}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO daemon_observations (snapshot_id, daemon_type, daemon_name, host_name, status, ceph_version)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, snapshotID, daemon.Type, daemon.Name, daemon.Host, daemon.Status, version)
	return err
}

func insertPool(ctx context.Context, tx *sql.Tx, snapshotID int64, pool inventory.Pool) error {
	size := sql.NullInt64{}
	if pool.Size != nil {
		size.Int64, size.Valid = int64(*pool.Size), true
	}
	minSize := sql.NullInt64{}
	if pool.MinSize != nil {
		minSize.Int64, minSize.Valid = int64(*pool.MinSize), true
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO pool_observations (snapshot_id, pool_id, name, pool_type, size, min_size)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, snapshotID, pool.ID, pool.Name, pool.Type, size, minSize)
	return err
}

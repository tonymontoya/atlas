package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/tonymontoya/ceph-atlas/internal/fleet"
	"github.com/tonymontoya/ceph-atlas/internal/inventory"
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
	`, snapshotID, string(daemon.Type), daemon.Name, daemon.Host, string(daemon.Status), version)
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

BEGIN;

CREATE TABLE host_observations (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    snapshot_id BIGINT NOT NULL REFERENCES inventory_snapshots(id) ON DELETE CASCADE,
    host_name TEXT NOT NULL,
    address TEXT,
    CHECK (host_name <> ''),
    CHECK (address IS NULL OR address <> ''),
    UNIQUE (snapshot_id, host_name)
);

CREATE INDEX host_observations_snapshot_idx
    ON host_observations (snapshot_id, host_name);

CREATE TABLE storage_device_observations (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    snapshot_id BIGINT NOT NULL REFERENCES inventory_snapshots(id) ON DELETE CASCADE,
    host_name TEXT NOT NULL,
    serial TEXT NOT NULL,
    device_type TEXT,
    device_path TEXT,
    device_health TEXT,
    osd_id INTEGER,
    CHECK (host_name <> ''),
    CHECK (serial <> ''),
    CHECK (device_type IS NULL OR device_type <> ''),
    CHECK (device_path IS NULL OR device_path <> ''),
    CHECK (device_health IS NULL OR device_health <> ''),
    CHECK (osd_id IS NULL OR osd_id >= 0),
    UNIQUE (snapshot_id, host_name, serial)
);

CREATE INDEX storage_device_observations_snapshot_idx
    ON storage_device_observations (snapshot_id, host_name);

CREATE INDEX storage_device_observations_serial_idx
    ON storage_device_observations (serial);

CREATE TABLE daemon_observations (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    snapshot_id BIGINT NOT NULL REFERENCES inventory_snapshots(id) ON DELETE CASCADE,
    daemon_type TEXT NOT NULL CHECK (daemon_type IN ('mon', 'mgr', 'osd', 'mds', 'rgw')),
    daemon_name TEXT NOT NULL,
    host_name TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('running', 'stopped')),
    ceph_version TEXT,
    CHECK (daemon_name <> ''),
    CHECK (host_name <> ''),
    CHECK (ceph_version IS NULL OR ceph_version <> ''),
    UNIQUE (snapshot_id, daemon_type, daemon_name)
);

CREATE INDEX daemon_observations_snapshot_idx
    ON daemon_observations (snapshot_id, daemon_type);

CREATE TABLE pool_observations (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    snapshot_id BIGINT NOT NULL REFERENCES inventory_snapshots(id) ON DELETE CASCADE,
    pool_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    pool_type TEXT NOT NULL CHECK (pool_type IN ('replicated', 'erasure')),
    size INTEGER,
    min_size INTEGER,
    CHECK (pool_id >= 0),
    CHECK (name <> ''),
    CHECK (size IS NULL OR size > 0),
    CHECK (min_size IS NULL OR min_size > 0),
    UNIQUE (snapshot_id, pool_id)
);

CREATE INDEX pool_observations_snapshot_idx
    ON pool_observations (snapshot_id, pool_id);

CREATE VIEW cluster_current_hosts AS
SELECT DISTINCT ON (snapshots.cluster_id, hosts.host_name)
    snapshots.cluster_id,
    clusters.fsid,
    hosts.host_name,
    hosts.address,
    snapshots.observed_at,
    snapshots.completed_at
FROM host_observations AS hosts
JOIN inventory_snapshots AS snapshots
    ON snapshots.id = hosts.snapshot_id
JOIN atlas_clusters AS clusters
    ON clusters.id = snapshots.cluster_id
ORDER BY snapshots.cluster_id, hosts.host_name, snapshots.observed_at DESC, snapshots.id DESC;

CREATE VIEW cluster_current_storage_devices AS
SELECT DISTINCT ON (snapshots.cluster_id, devices.host_name, devices.serial)
    snapshots.cluster_id,
    clusters.fsid,
    devices.host_name,
    devices.serial,
    devices.device_type,
    devices.device_path,
    devices.device_health,
    devices.osd_id,
    snapshots.observed_at,
    snapshots.completed_at
FROM storage_device_observations AS devices
JOIN inventory_snapshots AS snapshots
    ON snapshots.id = devices.snapshot_id
JOIN atlas_clusters AS clusters
    ON clusters.id = snapshots.cluster_id
ORDER BY snapshots.cluster_id, devices.host_name, devices.serial, snapshots.observed_at DESC, snapshots.id DESC;

CREATE VIEW cluster_current_daemons AS
SELECT DISTINCT ON (snapshots.cluster_id, daemons.daemon_type, daemons.daemon_name)
    snapshots.cluster_id,
    clusters.fsid,
    daemons.daemon_type,
    daemons.daemon_name,
    daemons.host_name,
    daemons.status,
    daemons.ceph_version,
    snapshots.observed_at,
    snapshots.completed_at
FROM daemon_observations AS daemons
JOIN inventory_snapshots AS snapshots
    ON snapshots.id = daemons.snapshot_id
JOIN atlas_clusters AS clusters
    ON clusters.id = snapshots.cluster_id
ORDER BY snapshots.cluster_id, daemons.daemon_type, daemons.daemon_name, snapshots.observed_at DESC, snapshots.id DESC;

CREATE VIEW cluster_current_pools AS
SELECT DISTINCT ON (snapshots.cluster_id, pools.pool_id)
    snapshots.cluster_id,
    clusters.fsid,
    pools.pool_id,
    pools.name,
    pools.pool_type,
    pools.size,
    pools.min_size,
    snapshots.observed_at,
    snapshots.completed_at
FROM pool_observations AS pools
JOIN inventory_snapshots AS snapshots
    ON snapshots.id = pools.snapshot_id
JOIN atlas_clusters AS clusters
    ON clusters.id = snapshots.cluster_id
ORDER BY snapshots.cluster_id, pools.pool_id, snapshots.observed_at DESC, snapshots.id DESC;

CREATE VIEW storage_device_osd_history AS
SELECT
    clusters.id AS cluster_id,
    clusters.fsid,
    devices.host_name,
    devices.serial,
    devices.osd_id,
    min(snapshots.observed_at) AS first_observed_at,
    max(snapshots.observed_at) AS last_observed_at
FROM storage_device_observations AS devices
JOIN inventory_snapshots AS snapshots
    ON snapshots.id = devices.snapshot_id
JOIN atlas_clusters AS clusters
    ON clusters.id = snapshots.cluster_id
WHERE devices.osd_id IS NOT NULL
GROUP BY clusters.id, clusters.fsid, devices.host_name, devices.serial, devices.osd_id;

COMMIT;

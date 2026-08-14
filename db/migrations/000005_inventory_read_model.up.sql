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

-- Current-state views list exactly what the cluster's latest snapshot
-- observed: objects absent from the latest snapshot are no longer current.
-- Historical Storage Device to OSD links remain queryable through
-- storage_device_osd_history.

CREATE VIEW cluster_current_hosts AS
WITH latest AS (
    SELECT DISTINCT ON (cluster_id)
        cluster_id,
        id AS snapshot_id,
        observed_at,
        completed_at
    FROM inventory_snapshots
    ORDER BY cluster_id, observed_at DESC, id DESC
)
SELECT
    latest.cluster_id,
    clusters.fsid,
    hosts.host_name,
    hosts.address,
    latest.observed_at,
    latest.completed_at
FROM latest
JOIN host_observations AS hosts
    ON hosts.snapshot_id = latest.snapshot_id
JOIN atlas_clusters AS clusters
    ON clusters.id = latest.cluster_id;

CREATE VIEW cluster_current_storage_devices AS
WITH latest AS (
    SELECT DISTINCT ON (cluster_id)
        cluster_id,
        id AS snapshot_id,
        observed_at,
        completed_at
    FROM inventory_snapshots
    ORDER BY cluster_id, observed_at DESC, id DESC
)
SELECT
    latest.cluster_id,
    clusters.fsid,
    devices.host_name,
    devices.serial,
    devices.device_type,
    devices.device_path,
    devices.device_health,
    devices.osd_id,
    latest.observed_at,
    latest.completed_at
FROM latest
JOIN storage_device_observations AS devices
    ON devices.snapshot_id = latest.snapshot_id
JOIN atlas_clusters AS clusters
    ON clusters.id = latest.cluster_id;

CREATE VIEW cluster_current_daemons AS
WITH latest AS (
    SELECT DISTINCT ON (cluster_id)
        cluster_id,
        id AS snapshot_id,
        observed_at,
        completed_at
    FROM inventory_snapshots
    ORDER BY cluster_id, observed_at DESC, id DESC
)
SELECT
    latest.cluster_id,
    clusters.fsid,
    daemons.daemon_type,
    daemons.daemon_name,
    daemons.host_name,
    daemons.status,
    daemons.ceph_version,
    latest.observed_at,
    latest.completed_at
FROM latest
JOIN daemon_observations AS daemons
    ON daemons.snapshot_id = latest.snapshot_id
JOIN atlas_clusters AS clusters
    ON clusters.id = latest.cluster_id;

CREATE VIEW cluster_current_pools AS
WITH latest AS (
    SELECT DISTINCT ON (cluster_id)
        cluster_id,
        id AS snapshot_id,
        observed_at,
        completed_at
    FROM inventory_snapshots
    ORDER BY cluster_id, observed_at DESC, id DESC
)
SELECT
    latest.cluster_id,
    clusters.fsid,
    pools.pool_id,
    pools.name,
    pools.pool_type,
    pools.size,
    pools.min_size,
    latest.observed_at,
    latest.completed_at
FROM latest
JOIN pool_observations AS pools
    ON pools.snapshot_id = latest.snapshot_id
JOIN atlas_clusters AS clusters
    ON clusters.id = latest.cluster_id;

-- Align OSD current-state reads with latest-snapshot semantics (the 0001
-- view kept destroyed OSDs current indefinitely).
CREATE OR REPLACE VIEW cluster_current_osds AS
WITH latest AS (
    SELECT DISTINCT ON (cluster_id)
        cluster_id,
        id AS snapshot_id,
        observed_at,
        completed_at
    FROM inventory_snapshots
    ORDER BY cluster_id, observed_at DESC, id DESC
)
SELECT
    latest.cluster_id,
    clusters.fsid,
    osds.osd_id,
    osds.host,
    osds.osd_up,
    osds.osd_in,
    osds.device,
    osds.weight,
    osds.ceph_version,
    osds.rook_namespace,
    osds.rook_pod,
    latest.observed_at,
    latest.completed_at
FROM latest
JOIN osd_observations AS osds
    ON osds.snapshot_id = latest.snapshot_id
JOIN atlas_clusters AS clusters
    ON clusters.id = latest.cluster_id;

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

BEGIN;

-- Restore the pre-000016 view definitions (no deregistered filter).

CREATE OR REPLACE VIEW cluster_current_health AS
SELECT DISTINCT ON (snapshots.cluster_id)
    snapshots.cluster_id,
    clusters.fsid,
    health.status,
    health.summary,
    checks.checks,
    snapshots.observed_at,
    snapshots.completed_at
FROM inventory_snapshots AS snapshots
JOIN atlas_clusters AS clusters
    ON clusters.id = snapshots.cluster_id
JOIN cluster_health_observations AS health
    ON health.snapshot_id = snapshots.id
LEFT JOIN LATERAL (
    SELECT COALESCE(
        jsonb_agg(
            jsonb_build_object(
                'name', health_checks.name,
                'severity', health_checks.severity,
                'summary', health_checks.summary
            )
            ORDER BY health_checks.name
        ),
        '[]'::jsonb
    ) AS checks
    FROM cluster_health_checks AS health_checks
    WHERE health_checks.health_observation_id = health.id
) AS checks ON TRUE
ORDER BY snapshots.cluster_id, snapshots.observed_at DESC, snapshots.id DESC;

CREATE OR REPLACE VIEW cluster_current_hosts AS
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

CREATE OR REPLACE VIEW cluster_current_storage_devices AS
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

CREATE OR REPLACE VIEW cluster_current_daemons AS
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

CREATE OR REPLACE VIEW cluster_current_pools AS
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

CREATE OR REPLACE VIEW storage_device_osd_history AS
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

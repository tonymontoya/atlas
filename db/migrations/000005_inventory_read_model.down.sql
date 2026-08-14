BEGIN;

DROP VIEW storage_device_osd_history;
DROP VIEW cluster_current_pools;
DROP VIEW cluster_current_daemons;
DROP VIEW cluster_current_storage_devices;
DROP VIEW cluster_current_hosts;
DROP TABLE pool_observations;
DROP TABLE daemon_observations;
DROP TABLE storage_device_observations;
DROP TABLE host_observations;

-- Restore the 0001 definition of cluster_current_osds.
DROP VIEW cluster_current_osds;
CREATE VIEW cluster_current_osds AS
SELECT DISTINCT ON (snapshots.cluster_id, osds.osd_id)
    snapshots.cluster_id,
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
    snapshots.observed_at,
    snapshots.completed_at
FROM osd_observations AS osds
JOIN inventory_snapshots AS snapshots
    ON snapshots.id = osds.snapshot_id
JOIN atlas_clusters AS clusters
    ON clusters.id = snapshots.cluster_id
ORDER BY snapshots.cluster_id, osds.osd_id, snapshots.observed_at DESC, snapshots.id DESC;

COMMIT;

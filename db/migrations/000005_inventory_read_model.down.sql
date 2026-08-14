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

COMMIT;

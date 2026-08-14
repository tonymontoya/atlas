BEGIN;

DROP VIEW IF EXISTS cluster_current_osds;
DROP VIEW IF EXISTS cluster_current_health;
DROP TABLE IF EXISTS osd_observations;
DROP TABLE IF EXISTS cluster_health_checks;
DROP TABLE IF EXISTS cluster_health_observations;
DROP TABLE IF EXISTS inventory_snapshots;
DROP TABLE IF EXISTS atlas_clusters;

COMMIT;

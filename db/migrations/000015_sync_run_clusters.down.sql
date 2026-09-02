BEGIN;

DROP INDEX inventory_sync_runs_cluster_started_idx;

ALTER TABLE inventory_sync_runs
    DROP COLUMN cluster_id;

COMMIT;

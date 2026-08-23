BEGIN;

ALTER TABLE inventory_sync_runs
    DROP CONSTRAINT inventory_sync_runs_provider_check;

ALTER TABLE inventory_sync_runs
    ADD CONSTRAINT inventory_sync_runs_provider_check
    CHECK (provider IN ('fake', 'ceph'));

ALTER TABLE inventory_snapshots
    DROP CONSTRAINT inventory_snapshots_provider_check;

ALTER TABLE inventory_snapshots
    ADD CONSTRAINT inventory_snapshots_provider_check
    CHECK (provider IN ('fake', 'ceph', 'rook'));

COMMIT;

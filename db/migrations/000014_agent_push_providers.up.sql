BEGIN;

-- Widen the inventory provider checks for the Agent push path
-- (ADR-0025, ADR-0026): an enrolled Atlas Agent pushes observation
-- batches over mutual TLS, and those batches persist with
-- provider = 'agent'. Fake and Ceph Dashboard provider rows are
-- unchanged.

ALTER TABLE inventory_sync_runs
    DROP CONSTRAINT inventory_sync_runs_provider_check;

ALTER TABLE inventory_sync_runs
    ADD CONSTRAINT inventory_sync_runs_provider_check
    CHECK (provider IN ('fake', 'ceph', 'agent'));

ALTER TABLE inventory_snapshots
    DROP CONSTRAINT inventory_snapshots_provider_check;

ALTER TABLE inventory_snapshots
    ADD CONSTRAINT inventory_snapshots_provider_check
    CHECK (provider IN ('fake', 'ceph', 'rook', 'agent'));

COMMIT;

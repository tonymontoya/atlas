BEGIN;

-- Widen inventory sync-run and daemon observation checks for the real
-- Ceph read provider (ADR-0023):
--
-- - Sync runs recorded by the Dashboard-backed provider use
--   provider = 'ceph'; fake-provider rows are unchanged.
-- - Daemon statuses preserve what the orchestrator reports. The fake
--   fixtures only produce running/stopped, but the Dashboard daemon
--   list also yields starting, error, and unknown.
--
-- inventory_snapshots.provider already allowed 'ceph'.
ALTER TABLE inventory_sync_runs
    DROP CONSTRAINT inventory_sync_runs_provider_check;

ALTER TABLE inventory_sync_runs
    ADD CONSTRAINT inventory_sync_runs_provider_check
    CHECK (provider IN ('fake', 'ceph'));

ALTER TABLE daemon_observations
    DROP CONSTRAINT daemon_observations_status_check;

ALTER TABLE daemon_observations
    ADD CONSTRAINT daemon_observations_status_check
    CHECK (status IN ('running', 'stopped', 'starting', 'error', 'unknown'));

COMMIT;

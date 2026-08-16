BEGIN;

ALTER TABLE inventory_sync_runs
    DROP CONSTRAINT inventory_sync_runs_provider_check;

ALTER TABLE inventory_sync_runs
    ADD CONSTRAINT inventory_sync_runs_provider_check
    CHECK (provider = 'fake');

ALTER TABLE daemon_observations
    DROP CONSTRAINT daemon_observations_status_check;

ALTER TABLE daemon_observations
    ADD CONSTRAINT daemon_observations_status_check
    CHECK (status IN ('running', 'stopped'));

COMMIT;

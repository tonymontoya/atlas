BEGIN;

CREATE TABLE inventory_sync_runs (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    provider TEXT NOT NULL CHECK (provider = 'fake'),
    scenario TEXT,
    status TEXT NOT NULL CHECK (status IN ('running', 'succeeded', 'failed')),
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at TIMESTAMPTZ,
    snapshot_id BIGINT REFERENCES inventory_snapshots(id),
    error_class TEXT,
    error_message TEXT,
    CHECK (scenario IS NULL OR scenario <> ''),
    CHECK (
        (status = 'running' AND finished_at IS NULL AND error_class IS NULL AND error_message IS NULL)
        OR
        (status = 'succeeded' AND finished_at IS NOT NULL AND snapshot_id IS NOT NULL AND error_class IS NULL AND error_message IS NULL)
        OR
        (status = 'failed' AND finished_at IS NOT NULL AND error_class IS NOT NULL AND error_message IS NOT NULL)
    )
);

CREATE INDEX inventory_sync_runs_started_idx
    ON inventory_sync_runs (started_at DESC, id DESC);

CREATE INDEX inventory_sync_runs_status_idx
    ON inventory_sync_runs (status, started_at DESC);

COMMIT;

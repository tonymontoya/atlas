BEGIN;

-- Give inventory sync runs cluster identity (issue #42): the web UI
-- renders sync runs scoped to the selected cluster, so runs must carry
-- the cluster they touched. Agent pushes stamp it at begin (the client
-- certificate resolves the cluster before the run starts), succeeded
-- runs stamp it from the saved snapshot's cluster, and failed pulls
-- keep NULL — a pull that never reached a cluster is honestly
-- unattributed.

ALTER TABLE inventory_sync_runs
    ADD COLUMN cluster_id BIGINT REFERENCES atlas_clusters(id) ON DELETE CASCADE;

CREATE INDEX inventory_sync_runs_cluster_started_idx
    ON inventory_sync_runs (cluster_id, started_at DESC, id DESC);

COMMIT;

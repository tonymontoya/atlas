BEGIN;

CREATE TABLE atlas_clusters (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    fsid UUID NOT NULL UNIQUE,
    name TEXT NOT NULL,
    ceph_version TEXT NOT NULL,
    cluster_type TEXT NOT NULL CHECK (cluster_type IN ('bare-metal', 'rook')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (name <> ''),
    CHECK (ceph_version <> '')
);

CREATE TABLE inventory_snapshots (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    cluster_id BIGINT NOT NULL REFERENCES atlas_clusters(id) ON DELETE CASCADE,
    provider TEXT NOT NULL CHECK (provider IN ('fake', 'ceph', 'rook')),
    scenario TEXT,
    observed_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    CHECK (scenario IS NULL OR scenario <> '')
);

CREATE INDEX inventory_snapshots_cluster_observed_idx
    ON inventory_snapshots (cluster_id, observed_at DESC, id DESC);

CREATE TABLE cluster_health_observations (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    snapshot_id BIGINT NOT NULL UNIQUE REFERENCES inventory_snapshots(id) ON DELETE CASCADE,
    status TEXT NOT NULL CHECK (status IN ('HEALTH_OK', 'HEALTH_WARN', 'HEALTH_ERR')),
    summary TEXT NOT NULL DEFAULT ''
);

CREATE TABLE cluster_health_checks (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    health_observation_id BIGINT NOT NULL REFERENCES cluster_health_observations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    severity TEXT NOT NULL,
    summary TEXT NOT NULL DEFAULT '',
    CHECK (name <> ''),
    CHECK (severity <> '')
);

CREATE INDEX cluster_health_checks_observation_idx
    ON cluster_health_checks (health_observation_id, name);

CREATE TABLE osd_observations (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    snapshot_id BIGINT NOT NULL REFERENCES inventory_snapshots(id) ON DELETE CASCADE,
    osd_id INTEGER NOT NULL,
    host TEXT NOT NULL,
    osd_up BOOLEAN NOT NULL,
    osd_in BOOLEAN NOT NULL,
    device TEXT,
    weight NUMERIC,
    ceph_version TEXT,
    rook_namespace TEXT,
    rook_pod TEXT,
    CHECK (osd_id >= 0),
    CHECK (host <> ''),
    CHECK (device IS NULL OR device <> ''),
    CHECK (weight IS NULL OR weight >= 0),
    CHECK (ceph_version IS NULL OR ceph_version <> ''),
    CHECK (rook_namespace IS NULL OR rook_namespace <> ''),
    CHECK (rook_pod IS NULL OR rook_pod <> ''),
    UNIQUE (snapshot_id, osd_id)
);

CREATE INDEX osd_observations_snapshot_idx
    ON osd_observations (snapshot_id, osd_id);

CREATE VIEW cluster_current_health AS
SELECT DISTINCT ON (snapshots.cluster_id)
    snapshots.cluster_id,
    clusters.fsid,
    health.status,
    health.summary,
    checks.checks,
    snapshots.observed_at,
    snapshots.completed_at
FROM inventory_snapshots AS snapshots
JOIN atlas_clusters AS clusters
    ON clusters.id = snapshots.cluster_id
JOIN cluster_health_observations AS health
    ON health.snapshot_id = snapshots.id
LEFT JOIN LATERAL (
    SELECT COALESCE(
        jsonb_agg(
            jsonb_build_object(
                'name', health_checks.name,
                'severity', health_checks.severity,
                'summary', health_checks.summary
            )
            ORDER BY health_checks.name
        ),
        '[]'::jsonb
    ) AS checks
    FROM cluster_health_checks AS health_checks
    WHERE health_checks.health_observation_id = health.id
) AS checks ON TRUE
ORDER BY snapshots.cluster_id, snapshots.observed_at DESC, snapshots.id DESC;

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
ORDER BY snapshots.cluster_id, osds.osd_id, snapshots.observed_at DESC, osds.id DESC;

COMMIT;

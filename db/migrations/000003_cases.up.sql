BEGIN;

CREATE TABLE cases (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    title TEXT NOT NULL,
    summary TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL CHECK (status IN ('detected', 'triaged', 'closed')),
    severity TEXT NOT NULL CHECK (severity IN ('info', 'low', 'medium', 'high', 'critical')),
    source TEXT NOT NULL CHECK (source IN ('manual', 'prometheus', 'ceph', 'rook', 'atlas')),
    cluster_fsid UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    closed_at TIMESTAMPTZ,
    CHECK (title <> ''),
    CHECK (summary <> ''),
    CHECK (closed_at IS NULL OR status = 'closed'),
    CHECK (updated_at >= created_at),
    CHECK (closed_at IS NULL OR closed_at >= created_at)
);

CREATE INDEX cases_status_updated_idx
    ON cases (status, updated_at DESC, id DESC);

CREATE INDEX cases_cluster_updated_idx
    ON cases (cluster_fsid, updated_at DESC, id DESC)
    WHERE cluster_fsid IS NOT NULL;

INSERT INTO cases (
    title,
    summary,
    status,
    severity,
    source,
    cluster_fsid,
    created_at,
    updated_at
) VALUES
    (
        'OSD down requires triage',
        'Local seed case representing an OSD_DOWN health check that needs operator triage.',
        'detected',
        'high',
        'prometheus',
        '00000000-0000-4000-8000-000000000101',
        '2026-08-13T12:00:00Z',
        '2026-08-13T12:00:00Z'
    ),
    (
        'Review weekly capacity trend',
        'Local seed case for capacity review without an automation workflow attached.',
        'triaged',
        'medium',
        'manual',
        '00000000-0000-4000-8000-000000000101',
        '2026-08-13T12:05:00Z',
        '2026-08-13T12:10:00Z'
    );

COMMIT;

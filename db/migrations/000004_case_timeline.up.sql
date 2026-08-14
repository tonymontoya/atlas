BEGIN;

CREATE TABLE case_timeline_events (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    case_id BIGINT NOT NULL REFERENCES cases (id) ON DELETE CASCADE,
    event_type TEXT NOT NULL CHECK (
        event_type IN (
            'case_detected',
            'case_triaged',
            'case_status_changed',
            'case_note_added',
            'workflow_attached',
            'workflow_state_changed'
        )
    ),
    message TEXT NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    actor_type TEXT NOT NULL CHECK (
        actor_type IN (
            'system',
            'user',
            'atlas_agent',
            'provider'
        )
    ),
    actor_id TEXT,
    actor_display_name TEXT NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (message <> ''),
    CHECK (actor_display_name <> ''),
    CHECK (payload = '{}'::jsonb OR jsonb_typeof(payload) = 'object')
);

CREATE INDEX case_timeline_events_case_order_idx
    ON case_timeline_events (case_id, occurred_at ASC, id ASC);

INSERT INTO case_timeline_events (
    case_id,
    event_type,
    message,
    occurred_at,
    actor_type,
    actor_display_name,
    payload
)
SELECT
    id,
    'case_detected',
    'OSD down case detected from Prometheus alert context.',
    '2026-08-13T12:00:00Z',
    'system',
    'Atlas',
    jsonb_build_object(
        'source', source,
        'clusterFsid', cluster_fsid::text,
        'signal', 'OSD_DOWN'
    )
FROM cases
WHERE title = 'OSD down requires triage';

INSERT INTO case_timeline_events (
    case_id,
    event_type,
    message,
    occurred_at,
    actor_type,
    actor_display_name,
    payload
)
SELECT
    id,
    'case_detected',
    'Manual capacity review case created.',
    '2026-08-13T12:05:00Z',
    'user',
    'Storage Operator',
    jsonb_build_object(
        'source', source,
        'clusterFsid', cluster_fsid::text
    )
FROM cases
WHERE title = 'Review weekly capacity trend';

INSERT INTO case_timeline_events (
    case_id,
    event_type,
    message,
    occurred_at,
    actor_type,
    actor_display_name,
    payload
)
SELECT
    id,
    'case_triaged',
    'Capacity review case triaged.',
    '2026-08-13T12:10:00Z',
    'user',
    'Storage Operator',
    jsonb_build_object(
        'previousStatus', 'detected',
        'newStatus', 'triaged'
    )
FROM cases
WHERE title = 'Review weekly capacity trend';

COMMIT;

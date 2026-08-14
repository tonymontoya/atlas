BEGIN;

ALTER TABLE cases
    ADD COLUMN assignee TEXT,
    ADD COLUMN assignee_display_name TEXT,
    ADD CONSTRAINT cases_assignee_columns CHECK (
        (assignee IS NULL AND assignee_display_name IS NULL)
        OR (assignee IS NOT NULL AND assignee_display_name IS NOT NULL AND assignee <> '' AND assignee_display_name <> '')
    );

CREATE INDEX cases_assignee_updated_idx
    ON cases (assignee, updated_at DESC, id DESC)
    WHERE assignee IS NOT NULL;

ALTER TABLE case_timeline_events
    DROP CONSTRAINT case_timeline_events_event_type_check;

ALTER TABLE case_timeline_events
    ADD CONSTRAINT case_timeline_events_event_type_check CHECK (
        event_type IN (
            'case_detected',
            'case_triaged',
            'case_status_changed',
            'case_note_added',
            'case_assigned',
            'workflow_attached',
            'workflow_state_changed'
        )
    );

CREATE TABLE case_notes (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    case_id BIGINT NOT NULL REFERENCES cases (id) ON DELETE CASCADE,
    author_id TEXT NOT NULL CHECK (author_id <> ''),
    author_display_name TEXT NOT NULL CHECK (author_display_name <> ''),
    body TEXT NOT NULL CHECK (body <> ''),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX case_notes_case_order_idx
    ON case_notes (case_id, created_at ASC, id ASC);

COMMIT;

BEGIN;

DROP TABLE case_notes;

ALTER TABLE case_timeline_events
    DROP CONSTRAINT case_timeline_events_event_type_check;

ALTER TABLE case_timeline_events
    ADD CONSTRAINT case_timeline_events_event_type_check CHECK (
        event_type IN (
            'case_detected',
            'case_triaged',
            'case_status_changed',
            'case_note_added',
            'workflow_attached',
            'workflow_state_changed'
        )
    );

ALTER TABLE cases
    DROP CONSTRAINT cases_assignee_columns;

ALTER TABLE cases
    DROP COLUMN assignee,
    DROP COLUMN assignee_display_name;

COMMIT;

BEGIN;

-- Task completions are immutable once written: no update or delete paths
-- exist in the application. They are the durable evidence that a human
-- Task was performed, in the same posture as workflow_approvals
-- (ADR-0019, ADR-0020).
CREATE TABLE workflow_task_completions (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    workflow_instance_id BIGINT NOT NULL REFERENCES workflow_instances (id) ON DELETE CASCADE,
    task_id TEXT NOT NULL CHECK (task_id <> ''),
    operator_id TEXT NOT NULL CHECK (operator_id <> ''),
    operator_display_name TEXT NOT NULL CHECK (operator_display_name <> ''),
    note TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (workflow_instance_id, task_id)
);

COMMIT;

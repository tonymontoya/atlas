BEGIN;

CREATE TABLE workflow_instances (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    case_id BIGINT NOT NULL REFERENCES cases (id) ON DELETE CASCADE,
    definition_id TEXT NOT NULL CHECK (definition_id <> ''),
    definition_version INT NOT NULL CHECK (definition_version >= 1),
    current_step TEXT,
    state TEXT NOT NULL CHECK (
        state IN (
            'pending',
            'running',
            'waiting_for_approval',
            'waiting_for_operator',
            'succeeded',
            'failed',
            'cancelled'
        )
    ),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at TIMESTAMPTZ,
    CHECK (
        state NOT IN ('succeeded', 'failed', 'cancelled')
        OR finished_at IS NOT NULL
    ),
    CHECK (updated_at >= created_at),
    CHECK (finished_at IS NULL OR finished_at >= created_at)
);

COMMENT ON COLUMN workflow_instances.current_step IS
    'The definition step id the instance is paused at; NULL while running or terminal.';

CREATE INDEX workflow_instances_case_idx
    ON workflow_instances (case_id, created_at ASC, id ASC);

CREATE INDEX workflow_instances_active_idx
    ON workflow_instances (id)
    WHERE state IN ('pending', 'running', 'waiting_for_approval', 'waiting_for_operator');

CREATE TABLE workflow_jobs (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    workflow_instance_id BIGINT NOT NULL REFERENCES workflow_instances (id) ON DELETE CASCADE,
    position INT NOT NULL CHECK (position >= 1),
    step_id TEXT NOT NULL CHECK (step_id <> ''),
    operation_type TEXT NOT NULL CHECK (operation_type <> ''),
    state TEXT NOT NULL CHECK (state IN ('pending', 'dispatched', 'succeeded', 'failed')),
    attempt INT NOT NULL CHECK (attempt >= 1),
    max_attempts INT NOT NULL CHECK (max_attempts >= 1),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at TIMESTAMPTZ,
    UNIQUE (workflow_instance_id, step_id),
    UNIQUE (workflow_instance_id, position),
    CHECK (attempt <= max_attempts),
    CHECK (state NOT IN ('succeeded', 'failed') OR finished_at IS NOT NULL),
    CHECK (updated_at >= created_at),
    CHECK (finished_at IS NULL OR finished_at >= created_at)
);

CREATE INDEX workflow_jobs_instance_idx
    ON workflow_jobs (workflow_instance_id, position ASC);

-- Approvals are immutable once written: no update or delete paths exist
-- in the application (ADR-0020).
CREATE TABLE workflow_approvals (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    workflow_instance_id BIGINT NOT NULL REFERENCES workflow_instances (id) ON DELETE CASCADE,
    gate_id TEXT NOT NULL CHECK (gate_id <> ''),
    approver_id TEXT NOT NULL CHECK (approver_id <> ''),
    approver_display_name TEXT NOT NULL CHECK (approver_display_name <> ''),
    reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (workflow_instance_id, gate_id)
);

COMMIT;

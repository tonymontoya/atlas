BEGIN;

CREATE TABLE alert_evaluation_runs (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    provider TEXT NOT NULL CHECK (provider = 'fake'),
    scenario TEXT,
    status TEXT NOT NULL CHECK (status IN ('running', 'succeeded', 'failed')),
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at TIMESTAMPTZ,
    error_class TEXT,
    error_message TEXT,
    alerts_evaluated INT CHECK (alerts_evaluated IS NULL OR alerts_evaluated >= 0),
    cases_created INT CHECK (cases_created IS NULL OR cases_created >= 0),
    CHECK (scenario IS NULL OR scenario <> ''),
    CHECK (
        (status = 'running' AND finished_at IS NULL AND error_class IS NULL AND error_message IS NULL)
        OR
        (status = 'succeeded' AND finished_at IS NOT NULL AND error_class IS NULL AND error_message IS NULL)
        OR
        (status = 'failed' AND finished_at IS NOT NULL AND error_class IS NOT NULL AND error_message IS NOT NULL)
    )
);

CREATE INDEX alert_evaluation_runs_started_idx
    ON alert_evaluation_runs (started_at DESC, id DESC);

CREATE INDEX alert_evaluation_runs_status_idx
    ON alert_evaluation_runs (status, started_at DESC);

CREATE TABLE case_alert_dedup (
    fingerprint TEXT PRIMARY KEY CHECK (fingerprint <> ''),
    case_id BIGINT NOT NULL REFERENCES cases(id),
    state TEXT NOT NULL CHECK (state IN ('open', 'resolved')),
    alert_name TEXT NOT NULL CHECK (alert_name <> ''),
    cluster_label TEXT NOT NULL DEFAULT '',
    first_seen_at TIMESTAMPTZ NOT NULL,
    last_seen_at TIMESTAMPTZ NOT NULL,
    CHECK (last_seen_at >= first_seen_at)
);

CREATE INDEX case_alert_dedup_case_idx
    ON case_alert_dedup (case_id);

COMMIT;

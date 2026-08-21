BEGIN;

-- Widen the alert evaluation run provider check for the real Prometheus
-- observability provider (ADR-0027): runs evaluated against a live
-- Prometheus use provider = 'prometheus'; fake fixture runs are
-- unchanged.

ALTER TABLE alert_evaluation_runs
    DROP CONSTRAINT alert_evaluation_runs_provider_check;

ALTER TABLE alert_evaluation_runs
    ADD CONSTRAINT alert_evaluation_runs_provider_check
    CHECK (provider IN ('fake', 'prometheus'));

COMMIT;

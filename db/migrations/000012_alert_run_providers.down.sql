BEGIN;

-- Re-tighten the alert evaluation run provider check to fake-only. The
-- downgrade fails if any 'prometheus' runs exist; delete them first.

ALTER TABLE alert_evaluation_runs
    DROP CONSTRAINT alert_evaluation_runs_provider_check;

ALTER TABLE alert_evaluation_runs
    ADD CONSTRAINT alert_evaluation_runs_provider_check
    CHECK (provider = 'fake');

COMMIT;

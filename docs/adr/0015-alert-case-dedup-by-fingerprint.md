# Alert-Driven Case Deduplication by Fingerprint Link Table

Atlas deduplicates automatic Case creation with a `case_alert_dedup` link table keyed by an alert fingerprint: SHA-256 over the alert name plus all non-context labels in canonical (sorted) order. Labels that describe presentation rather than condition identity (`severity`, `summary`, `description`) are excluded, so a re-graded alert keeps its fingerprint while a different OSD or cluster does not. The table stays source-agnostic on `cases`: one row per fingerprint points at the Case currently matching that condition, which structurally enforces at most one Case per firing condition without a fingerprint column on `cases` itself.

Evaluation semantics: a firing alert with no row (or a row whose linked Case is closed) creates a new Case plus a `case_detected` Timeline Event and the dedup row in one transaction; a firing alert matching an open Case updates `last_seen_at` only. Non-firing observations update the dedup row (`state = 'resolved'`) without closing the Case: auto-close is a policy decision and no policy exists yet. Reopen after a manual close creates a new Case and re-points the row. Concurrent evaluations serialize on a transaction-scoped advisory lock so the check-then-act on fingerprints cannot race; the whole losing transaction rolls back.

`alert_evaluation_runs` mirrors `inventory_sync_runs` (ADR-0005: durable, replayable automation records) and additionally records `alerts_evaluated` and `cases_created` counts.

**Alternatives considered**: a fingerprint column on `cases` with a partial unique index over open Cases (couples the Case record to the alert concept; rejected); query-only matching by title/labels with no durable structure (fragile against wording changes; rejected). Deferring dedup entirely until policy exists was rejected because duplicate Cases from every evaluation cycle would make the detection slice unusable.

**Consequences**

- One row per fingerprint: dedup history beyond first/last seen is not retained; per-sighting observation rows can extend this later if a read needs them.
- Cluster resolution uses the alert's `cluster` label against Cluster name first, then fsid; unresolved labels still create Cases with a NULL `cluster_fsid` so the signal is not lost.
- Seeded Cases have no dedup rows and never match fingerprints; tests must isolate seeded from detected rows.

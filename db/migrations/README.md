# Migrations

PostgreSQL migrations live here.

Migrations are plain SQL and should be append-only after merge.

## Initial Inventory Schema

`000001_initial_inventory.up.sql` creates the first durable boundary for:

- `atlas_clusters`: Ceph cluster identity and cluster type.
- `inventory_snapshots`: append-only provider observation batches.
- `cluster_health_observations` and `cluster_health_checks`: normalized health observations.
- `osd_observations`: append-only OSD inventory and state observations.
- `cluster_current_health` and `cluster_current_osds`: API-facing views over the latest observations.

## Inventory Sync Runs

`000002_inventory_sync_runs.up.sql` creates:

- `inventory_sync_runs`: durable status records for explicit inventory sync attempts.

## Cases

`000003_cases.up.sql` creates:

- `cases`: the first read-only operational work record table for local case
  listing and detail endpoints.

The migration also seeds two local development cases. They are deliberately
small, static examples and do not imply mutation, workflow, RBAC, or audit
behavior.

## Case Timelines

`000004_case_timeline.up.sql` creates:

- `case_timeline_events`: user-facing chronological events for Case progress.

The migration seeds local development Timeline Events for the seeded cases.
These records are display-oriented operational history, not Audit Events.

## Inventory Read Model

`000005_inventory_read_model.up.sql` creates:

- `host_observations`, `storage_device_observations`, `daemon_observations`,
  and `pool_observations`: append-only per-snapshot inventory observations.
- `cluster_current_hosts`, `cluster_current_storage_devices`,
  `cluster_current_daemons`, and `cluster_current_pools`: API-facing views
  over the latest observations.
- `storage_device_osd_history`: historical Storage Device to OSD identity
  links with first/last observed timestamps (see ADR-0014).

## Alert Case Detection

`000006_alert_case_detection.up.sql` creates:

- `alert_evaluation_runs`: durable records of alert evaluation runs with
  `alerts_evaluated` and `cases_created` counts.
- `case_alert_dedup`: one row per alert fingerprint linking the currently
  matching Case (see ADR-0015). Enforces at most one Case per firing
  condition; alert resolution is recorded on the row without closing Cases.

The API can optionally read inventory, Case, and Case Timeline records from
PostgreSQL with `ATLAS_READ_SOURCE=postgres`. Inventory endpoints need an
explicit fake-provider sync before they have current cluster data. Seeded Case
endpoints are available after migrations. Provider reads remain the default.

For local development, start PostgreSQL with `make db-up` and apply migrations
with `make db-migrate`.

`make db-migrate` records applied files in `atlas_schema_migrations`, so it can
be rerun during local development without reapplying successful migrations.

## Manual Case Writes

`000007_manual_case_writes.up.sql` adds:

- `cases.assignee` and `cases.assignee_display_name` with a consistency
  constraint (both set or both clear) and an assignee-scoped partial index.
- `case_assigned` to the Case Timeline event type vocabulary (CHECK
  replacement).
- `case_notes`: durable, addressable Case Notes with author subject and
  display-name snapshots, ordered per case.

Manual writes arrive through the authenticated write API (ADR-0016): manual
Case creation, status transitions (closed is terminal; reopen means a new
Case, mirroring ADR-0015), assignment, and notes. Each write records its
matching Timeline Event with the acting operator as a user actor.

## Workflow State

`000008_workflow_state.up.sql` adds:

- `workflow_instances`: durable Workflow Instance state machines with
  CHECK-constrained states from ADR-0019 (`pending`, `running`,
  `waiting_for_approval`, `waiting_for_operator`, terminal `succeeded`,
  `failed`, `cancelled`), the definition id/version they reference
  (ADR-0017), and `current_step` recording the definition step an
  instance is paused at.
- `workflow_jobs`: Job state machines (`pending`, `dispatched`,
  `succeeded`, `failed`) with definition order, typed operation type,
  and attempt/max-attempts retry bookkeeping. The failed -> pending
  retry edge is governed by the definition's retry policy.
- `workflow_approvals`: immutable Approval records bound to a Workflow
  Instance gate with approver identity snapshots and an optional reason
  (ADR-0020). One approval per gate; no update or delete paths exist.

Instance creation writes a `workflow_attached` Timeline Event on the
owning Case; every instance state transition writes
`workflow_state_changed` with the acting operator or the Atlas system
actor.

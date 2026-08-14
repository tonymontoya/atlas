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

The API can optionally read inventory, Case, and Case Timeline records from
PostgreSQL with `ATLAS_READ_SOURCE=postgres`. Inventory endpoints need an
explicit fake-provider sync before they have current cluster data. Seeded Case
endpoints are available after migrations. Provider reads remain the default.

For local development, start PostgreSQL with `make db-up` and apply migrations
with `make db-migrate`.

`make db-migrate` records applied files in `atlas_schema_migrations`, so it can
be rerun during local development without reapplying successful migrations.

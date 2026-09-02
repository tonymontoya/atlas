# Agent Guidance

This file is the first stop for coding agents working in this repository.
Follow it alongside `CONTRIBUTING.md`, `CONTEXT.md`, `dev-plans/`, and
`docs/adr/`.

## Agent skills

The agentic-engineering skill set is vendored into this repository at
`.agents/skills/`, so every contributor — human or agent — has access to the
same process. OpenCode and other agents-compatible tools discover them
automatically; the repository copy is canonical for work in this repo.

### Main flow: idea → ship

1. `/grill-with-docs` — interview to sharpen the idea; captures `CONTEXT.md` + ADRs.
2. `/to-tickets` or `/to-spec` — publish Definition-of-Done tickets to the tracker.
3. `/implement` — build, driving `/tdd`, closing with `/code-review`.

Use `/ask-ai` any time you're unsure which skill fits. The `/setup-skills`
precondition (tracker, labels, doc layout) is already satisfied — see the
subsections below and `docs/agents/`.

These skills describe how the maintainer works. They are an offer, not a
gate: contributors may follow any process they like. Changes are judged on
tests, lint, and documentation staying in sync.

### Issue tracker

Issues live as GitHub issues in this repo; use `gh` CLI. See `docs/agents/issue-tracker.md`.

### Triage labels

The five canonical triage roles use their default label strings. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: root `CONTEXT.md` plus `docs/adr/`. See `docs/agents/domain.md`.

## Current Project Posture

Atlas is being built carefully from a fake-provider, local-development scaffold.
Do not jump directly to production Ceph, Rook, agent mutation, or fleet
automation work.

The current implementation supports:

- Go backend scaffolding.
- React and TypeScript UI scaffolding.
- PostgreSQL local development through Docker Compose.
- Full local Docker Compose stack for PostgreSQL, migrations, fake inventory
  sync, API, web UI, a dev OIDC issuer, and the full enrolled Agent loop
  (fake Dashboard container, ephemeral dev CA, mutual-TLS API listener,
  registration bootstrap, real atlas-agent).
- Plain SQL migrations.
- Fake-provider inventory fixtures.
- A REST API v1 scaffold: the cluster index (`GET /api/v1/clusters`,
  searchable and paginated, with health summary and Agent last-seen and
  last-push times), cluster-scoped inventory reads
  (`GET /api/v1/clusters/{fsid}/health`,
  `/osds`, `/hosts`, `/storage-devices`, `/daemons`, `/pools`),
  cluster-filtered Case lists (`GET /api/v1/cases?cluster=…`) and
  sync-run history (`GET /api/v1/inventory-sync-runs?cluster=…`; runs
  carry `clusterFsid`/`clusterName` when attributed — agent pushes from
  the begin that follows certificate resolution, succeeded runs from
  their snapshot — and failed pulls that never reached a cluster stay
  unattributed), and read-only Case endpoints; manual Case writes
  authenticate with OIDC bearer tokens (ADR-0016). The single-cluster
  `/api/v1/clusters/current/*` family was removed as a documented 0.x
  breaking change. Provider read source serves the same cluster-scoped
  shape through `internal/providers/singlecluster` (its one provider is
  the only addressable cluster; anything else 404s).
- An inventory entity declaration (`internal/inventory/entities`) that
  the list-shaped cluster reads derive from: the Postgres read
  methods, the singlecluster adapter bindings, the API routes and
  handlers, and the provider contract-test coverage all loop the
  registry, and a declared entity missing its wiring fails that
  consumer's construction or completeness tests. Adding a list entity
  means one declaration entry plus the artifacts that cannot be
  derived (migration, OpenAPI path, web page, and one typed binding
  entry per consumer).
- An IBM Carbon web UI (ADR-0028): app shell with a cluster switcher
  in the header (selection is the URL — picking a cluster navigates to
  its overview), cluster index (searchable, paginated, health + Agent
  last-seen), per-cluster views over the cluster-scoped reads — an
  Overview page (metric tiles including an Agent tile with last-seen
  and last-push age, health checks, inventory tables, all five Daemon
  statuses treated deliberately: running/stopped/starting/error/unknown
  each with their own badge tone and tally) plus cluster-scoped Cases
  and Sync runs sections behind section tabs (`/clusters/{fsid}/cases`,
  `/clusters/{fsid}/sync-runs`) — global Cases and Sync Runs pages for
  the fleet-wide views, operator bearer-token sign-in, manual Case
  writes, Workflow attach/approve/resume forms, and Cluster Registration
  through the UI — a Register flow that shows the one-time Enrollment
  Credential and Agent install instructions exactly once behind an
  explicit acknowledgment (the credential is never re-displayable; it
  lives only in the registration response), plus a deregister row
  action on the index. The load/submit choreography (abort,
  stale-result ignoring, data retention, double-submit guarding,
  formatted errors) is centralized in two hook-tested seams —
  `useResource` and `useMutation` in `web/app/src/resources.ts` — and
  every page load, the Case detail loader, and every Case-write
  handler goes through them; the hooks render under jsdom +
  @testing-library/react, so new load/submit behavior is hook-tested
  rather than re-authored per page.
- Agent Enrollment (ADR-0026): `POST /api/v1/agent/enroll` exchanges a
  CSR plus the one-time Enrollment Credential for a client certificate
  from an internal CA, burning the credential and binding the Cluster's
  FSID in one transaction. Binding first releases a stale FSID claim
  from a deregistered row (renewal is re-enrollment with a fresh
  registration and credential; live holders still conflict, total
  uniqueness stays — 2026-08-28 amendment). The CA is control-plane
  configuration (`ATLAS_ENROLLMENT_CA_CERT_PATH`/`ATLAS_ENROLLMENT_CA_KEY_PATH`,
  all-or-none); no default local path configures key material, and tests
  use an in-process test CA (`internal/ca/catest`). Certificates map to
  clusters by recorded serial (migration 000013).
- Agent observation ingestion (ADR-0025): `POST
  /api/v1/agent/observations` accepts one typed Observation Batch per
  collection cycle from an enrolled Agent, authenticated by its client
  certificate over mutual TLS (opt-in TLS listener via
  `ATLAS_API_TLS_CERT_PATH`/`ATLAS_API_TLS_KEY_PATH`; the enrollment CA
  verifies client certificates). Cluster attribution comes from the
  certificate's recorded serial (`store.ResolveAgentCluster`), never
  payload claims — an FSID mismatch is a 409. Batches persist through
  the existing single-transaction save path with provider `agent`
  (migration 000014 widens both provider CHECKs); revoked,
  deregistered, or expired certificates push nothing.
- A fake inventory sync command that writes one observation batch to PostgreSQL.
- The atlas-agent binary (`cmd/atlas-agent`, ADR-0025/0026): one-shot
  (`-once`) or daemon operation from agent-local `ATLAS_AGENT_*`
  config (`internal/config.LoadAgent`). It enrolls on first start with
  a locally generated Ed25519 key plus the one-time Enrollment
  Credential, persists the issued certificate chain and key under
  `ATLAS_AGENT_STATE_DIR`, re-enrolls when the stored certificate is
  expired and a fresh credential is provided, then collects full
  inventory batches through the Ceph Dashboard read provider running
  inside the Agent and pushes them over mutual TLS with backoff
  (`internal/atlasagent`). Permanent failures (4xx from Atlas, corrupt
  state, missing credential) stop the Agent; the binary is read-only
  by construction — no dispatch or command surface — and Dashboard
  credentials never leave the Agent. Tests drive the whole loop
  against the real API server over TLS plus the in-process fake
  Dashboard and test CA; no real Ceph, no real certificates.
- A dev stack that runs the full enrolled loop (issue #43): the
  `stack` compose profile adds a fixture-backed fake Dashboard
  container (`cmd/atlas-dev-dashboard` over the pure
  `internal/providers/ceph/dashfake` handler that `dashtest` wraps),
  an ephemeral dev CA generated inside the API container at every
  start (`cmd/atlas-dev-ca`, `internal/ca/devca`), an additional
  API TLS listener with client-certificate verification
  (`ATLAS_HTTPS_ADDR`, in-network only), a bootstrap that retires the
  previous bring-up's rows and creates a fresh registration through
  the Operator API (`cmd/atlas-dev-agent-bootstrap`), and a real
  atlas-agent service that enrolls, collects from the fake Dashboard,
  and pushes over mutual TLS (its one-time credential arrives via
  `ATLAS_AGENT_ENROLLMENT_CREDENTIAL_FILE`). `make dev-stack-check`
  asserts register → enroll → push → read end to end and stays green
  across reruns against the persistent PostgreSQL volume.
- A read-only real Ceph provider over the Ceph Dashboard REST API
  (ADR-0023) that lives inside the enrolled Agent's collection path with
  `ATLAS_AGENT_DASHBOARD_*` env (ADR-0025): the Agent logs into a live
  Dashboard with a dedicated read-only user and never exports
  credentials. The control-plane pull path
  (`ATLAS_PROVIDER_MODE=ceph` + `ATLAS_CEPH_*` stored credentials) is
  removed as a documented 0.x breaking change — setting the removed
  variables fails fast in every control-plane command (the Agent's own
  env surface, `ATLAS_AGENT_*`, never read them), and
  `atlas-inventory-sync` remains the fake-mode dev seeder
  (`make db-sync-fake`). Tests run
  against an in-process fake Dashboard
  (`internal/providers/ceph/dashtest`), never real Ceph. The API read
  source (`ATLAS_READ_SOURCE`) still uses fake/postgres sources.
- Fake alert evaluation that creates and deduplicates Cases from alerts.
- A real Prometheus alert source (ADR-0027): `ATLAS_ALERT_SOURCE=prometheus`
  plus `ATLAS_PROMETHEUS_URL` (optional bearer token and insecure-TLS flag)
  points `atlas-alert-eval` at a live `/api/v1/alerts` endpoint through the
  same detection pipeline. `ATLAS_ALERT_EVAL_INTERVAL` enables loop mode;
  the one-shot default is preserved. Explicit opt-in only — tests run against
  an in-process fake Prometheus (`internal/providers/prometheus/promtest`),
  never a live one.
- Seeded read-only case records in PostgreSQL.
- Seeded read-only Case Timeline records in PostgreSQL.
- Manual Case writes (creation, transitions with closed-terminal semantics,
  assignment, notes) with actor-attributed Timeline Events.
- The Workflow skeleton (ADR-0017 through ADR-0022): the code-registered
  Replace OSD Workflow attaches to a Case with durable instance and Job
  state, pauses at its Approval Gate and human Task, and resumes through
  authenticated approval and task-completion endpoints. With
  `ATLAS_AGENT_MODE=fake` a fake Agent adapter drives Jobs (typed
  operation envelopes, retry policies, idempotent replay) to terminal
  state; every transition writes a Timeline Event. `make dev-stack-check`
  probes the full loop, including 401-without-token assertions.

## Non-Negotiable Decisions

- MVP is single-zone. Do not require global control-plane or federation behavior.
- Backend and Atlas Agent are Go.
- Web UI is React with TypeScript.
- Durable persistence is PostgreSQL.
- Public API direction is REST API v1.
- Ceph 18 is the primary MVP target; the support floor is Ceph 18 and
  newer (Ceph 16 retired 2026-09-01; Ceph 19/20 read support lands v0.0.11,
  mutation validation with v0.3.0).
- Bare-metal Ceph and Rook-managed Ceph are equal first-class cluster types.
- AWX and Ansible are not runtime dependencies.
- Atlas Agents must not expose generic shell execution, SSH proxying, or arbitrary remote command APIs.
- Privileged operations must be typed, approved, auditable, idempotency-aware, and guarded by policy/RBAC.

Changing any of these requires an ADR in `docs/adr/`.

## Work Style

- Prefer narrow, reviewable slices.
- Keep fake-provider and local Docker workflows working at all times.
- Add tests with behavior changes.
- Keep API behavior, OpenAPI, migrations, and docs in sync.
- Do not wire real Ceph or Rook providers until the local fake-provider and persistence paths are well tested.
- Do not add Redis, NATS, agent mutation workflows, RBAC, or audit infrastructure before a slice explicitly needs them.
- Do not introduce AWX/Ansible adapters except as clearly optional migration/discovery support.

## Local Commands

Use these before committing when relevant:

```sh
make test
make lint
make fixtures-check
make provider-contract-test
make web-test
make web-lint
```

For PostgreSQL-backed work:

```sh
make db-up
make db-migrate
make db-test
make db-sync-fake
```

The local PostgreSQL database listens on `127.0.0.1:15432` by default.

For a containerized local stack:

```sh
make dev-stack-up
make dev-stack-check
make dev-stack-down
```

The full stack publishes the API on `127.0.0.1:8080` and the web UI on
`127.0.0.1:5173` by default. Override `ATLAS_API_PORT` or `ATLAS_WEB_PORT`
when those ports are occupied.

## Repository Boundaries

- `cmd/` contains runnable commands.
- `internal/api` owns HTTP routing and response behavior.
- `internal/providers` owns provider contracts.
- `internal/providers/fake` owns fixture-backed provider behavior.
- `internal/store` owns PostgreSQL persistence.
- `internal/inventorysync` owns read-provider-to-store sync orchestration.
- `db/migrations` contains append-only plain SQL migrations.
- `dev/fixtures` contains local fake-provider fixtures.
- `api/openapi` contains the REST API contract.
- `docs/adr` records accepted architecture decisions.

## Domain Language

Use the terms in `CONTEXT.md`.

Especially:

- Case, not incident, unless describing a real operational incident.
- Workflow, not playbook.
- Job, not command.
- Storage Device or Device, not disk, unless physical media is meant.
- Timeline Event and Audit Event are distinct.

## Safety Defaults

When unsure, choose the path that keeps Atlas portable and environment-neutral.
Example operating environments can inform fixtures and discovery, but code must
not become dependent on any one environment.

Do not add hidden production assumptions, production endpoints, secrets, or
mandatory access to real Ceph, Rook, AWX, Ansible, Redis, or NATS in ordinary
local development paths.

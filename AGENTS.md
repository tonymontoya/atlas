# Agent Guidance

This file is the first stop for coding agents working in this repository.
Follow it alongside `CONTRIBUTING.md`, `CONTEXT.md`, `dev-plans/`, and
`docs/adr/`.

## Agent skills

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
  sync, API, web UI, and a dev OIDC issuer.
- Plain SQL migrations.
- Fake-provider inventory fixtures.
- A REST API v1 scaffold: read-only inventory and Case endpoints, plus
  authenticated manual Case write endpoints (create, transition, assign,
  note) verified through OIDC bearer tokens (ADR-0016).
- A fake inventory sync command that writes one observation batch to PostgreSQL.
- Fake alert evaluation that creates and deduplicates Cases from alerts.
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
- Ceph 18 is the primary MVP target.
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

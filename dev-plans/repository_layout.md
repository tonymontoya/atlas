# Atlas Repository Layout
**Version:** 0.1 (Draft)  
**Status:** Pre-development Design  
**Audience:** Engineering, Architecture, Contributors  
**Project:** Atlas

---

# 1. Purpose

This document defines the initial source tree for Atlas before implementation begins.

The layout should support a Go backend, Go Atlas Agent, React + TypeScript UI, PostgreSQL persistence, REST API v1, fake providers from day one, and optional later Redis, NATS, Ceph, Rook, and deployment tooling.

---

# 2. Layout Principles

- Keep the repo as one product monorepo.
- Keep Go service code under `cmd/` and `internal/`.
- Keep browser UI code under `web/`.
- Keep API contracts and generated artifacts outside service internals.
- Keep migrations and seed data explicit.
- Keep fake provider fixtures committed and scrubbed.
- Keep deployment manifests separate from local development scripts.
- Do not create AWX or Ansible runtime folders.
- Do not introduce Redis or NATS directories until code needs them.
- Do not scaffold mutating Agent operations yet.

---

# 3. Proposed Tree

```text
atlas/
├── api/
│   └── openapi/
│       └── README.md
├── cmd/
│   ├── atlas-api/
│   │   └── README.md
│   └── atlas-agent/
│       └── README.md
├── db/
│   ├── migrations/
│   │   └── README.md
│   └── seed/
│       └── README.md
├── deploy/
│   ├── containers/
│   │   └── README.md
│   ├── kubernetes/
│   │   └── README.md
│   └── podman/
│       └── README.md
├── dev/
│   ├── README.md
│   └── fixtures/
│       ├── README.md
│       ├── ceph/
│       ├── rook/
│       ├── prometheus/
│       └── normalized/
├── docs/
│   └── adr/
├── internal/
│   ├── agent/
│   ├── api/
│   ├── app/
│   ├── audit/
│   ├── cases/
│   ├── config/
│   ├── events/
│   ├── fleet/
│   ├── inventory/
│   ├── observability/
│   ├── policy/
│   ├── providers/
│   │   ├── agent/
│   │   ├── ceph/
│   │   ├── fake/
│   │   ├── prometheus/
│   │   └── rook/
│   ├── rbac/
│   ├── scheduler/
│   ├── search/
│   └── workflows/
├── pkg/
│   └── README.md
├── scripts/
│   └── README.md
├── web/
│   ├── README.md
│   └── app/
└── dev-plans/
```

The first scaffold should create only the directories needed for Mode 0 and Mode 1 local development. Empty directories should have a short `README.md` only when they clarify ownership.

---

# 4. Directory Responsibilities

## `api/`

Owns public API contracts.

Initial contents:

- OpenAPI source for REST API v1
- generated API documentation when introduced
- API compatibility notes

Rules:

- Public REST endpoints are versioned.
- UI uses the same public API contracts as external clients.
- No backend business logic belongs here.

## `cmd/atlas-api/`

Entrypoint for the Atlas API server.

Responsibilities:

- process startup
- config loading
- dependency wiring
- HTTP server lifecycle
- graceful shutdown

Rules:

- Keep business logic out of `cmd/`.
- Prefer wiring concrete implementations to interfaces defined under `internal/`.

## `cmd/atlas-agent/`

Entrypoint for the Atlas Agent.

Responsibilities:

- agent startup
- certificate/config loading
- registration and heartbeat
- typed operation server/client lifecycle
- graceful shutdown

Initial scaffold:

- may exist as a placeholder README
- no mutating operations
- no shell execution

## `internal/`

Owns private Go application code.

The Go module should keep most implementation here to avoid premature public API commitments.

Suggested module responsibilities:

| Directory | Responsibility |
|---|---|
| `internal/app` | application composition and shared service wiring |
| `internal/api` | HTTP handlers, middleware, request/response mapping |
| `internal/config` | typed configuration loading and validation |
| `internal/fleet` | Organizations, Zones, Datacenters, Clusters |
| `internal/inventory` | Hosts, Storage Devices, OSDs, daemons, pools |
| `internal/cases` | Case lifecycle, comments, timelines, evidence links |
| `internal/workflows` | Workflow templates, instances, jobs, durable state |
| `internal/policy` | safety/policy decisions and approval requirements |
| `internal/rbac` | authorization model and permission checks |
| `internal/audit` | immutable audit records |
| `internal/events` | durable events and event publication |
| `internal/scheduler` | future scheduling and retry timing |
| `internal/observability` | metrics/alerts/SLO context |
| `internal/search` | PostgreSQL FTS-backed search |
| `internal/providers` | provider contracts and implementations |
| `internal/agent` | shared agent protocol/domain code |

Rules:

- Domain modules own their domain behavior.
- Providers normalize external state into Atlas domain concepts.
- UI-specific shaping belongs in API handlers, not provider implementations.
- No module should depend directly on AWX or Ansible.

## `internal/providers/`

Owns provider contracts and implementations.

Initial shape:

```text
internal/providers/
├── contracts.go
├── fake/
├── ceph/
├── rook/
├── prometheus/
└── agent/
```

Rules:

- Provider contracts should match `dev-plans/provider_contracts.md`.
- Fake providers are required from day one.
- Real providers should depend on transport clients behind small interfaces.
- Raw upstream payloads do not become domain objects.
- Mutating provider methods remain unimplemented until safety/security gates exist.

## `db/`

Owns database migrations and seed data.

Initial contents:

- migration directory
- local seed data directory

Rules:

- Migrations are append-only after merge.
- Seed data must not contain production secrets or sensitive environment data.
- Fixture data belongs under `dev/fixtures`, not migrations.

## `dev/`

Owns local development assets that are not production deployment artifacts.

Initial contents:

- fixture documentation
- fake provider fixture data
- local development notes

Rules:

- Local development defaults to fake providers.
- Real Ceph/Rook validation setup should be opt-in and documented separately.
- Do not place production credentials or endpoints here.

## `dev/fixtures/`

Owns provider fixtures.

Suggested shape:

```text
dev/fixtures/
├── ceph/
│   ├── reef-healthy-baremetal/
│   ├── reef-osd-down-baremetal/
│   └── pacific-readonly/
├── rook/
│   ├── reef-healthy-rook/
│   └── reef-osd-down-rook/
├── prometheus/
│   └── osd-down-alert/
└── normalized/
    ├── cluster-identity/
    ├── health/
    ├── inventory/
    └── alerts/
```

Rules:

- Fixtures are scrubbed.
- Fixtures are deterministic.
- Fixtures include raw and normalized examples where useful.
- Fakes should load fixture files rather than hard-code large payloads.

## `web/`

Owns the React + TypeScript UI.

Initial contents:

- UI package under `web/app`
- frontend README

Rules:

- No business logic in UI.
- UI consumes public REST API v1.
- UI should be useful with fake provider data from Mode 1.

## `deploy/`

Owns production-like deployment assets.

Initial contents:

- placeholder READMEs only

Later contents:

- OCI container definitions
- Kubernetes manifests or Helm chart if selected
- Podman deployment notes if selected

Rules:

- Do not block initial development on full deployment manifests.
- Production deployment URLs, secrets, and environment-specific manifests do not belong in the generic repo.

## `scripts/`

Owns small developer scripts.

Rules:

- Scripts should be thin wrappers around documented commands.
- Scripts must fail fast.
- Scripts must not embed secrets or production endpoints.

## `pkg/`

Reserved for Go packages that are intentionally public for external consumers.

Initial posture:

- keep empty except README
- prefer `internal/` until a real external API need exists

---

# 5. First Scaffold Scope

The first code scaffold should create:

- `cmd/atlas-api`
- `internal/config`
- `internal/api`
- `internal/providers`
- `internal/providers/fake`
- `internal/fleet`
- `internal/inventory`
- `internal/cases`
- `db/migrations`
- `dev/fixtures`
- `web/app`
- basic `Makefile`
- Go module
- UI package config

The first scaffold should not create:

- real Ceph provider implementation
- real Rook provider implementation
- real Prometheus provider implementation
- Atlas Agent mutation implementation
- Redis integration
- NATS integration
- Kubernetes deployment manifests beyond placeholders
- AWX or Ansible adapters

---

# 6. Initial Build Commands

Target commands for first scaffold:

```text
make test
make lint
make dev
make dev-fake
make db-migrate
make fixtures-check
```

These commands should work without:

- production access
- Ceph
- Rook
- Prometheus
- Redis
- NATS
- Atlas Agent mutation provider

---

# 7. Package Boundary Rules

- `internal/providers` may depend on transport-specific clients.
- Domain modules should depend on provider contracts, not concrete provider implementations.
- `internal/api` maps domain objects to REST representations.
- `cmd/` wires concrete providers into the application.
- `web/` depends on REST API contracts, not Go internals.
- `dev/fixtures` may be imported by tests and fake providers, but not by production provider implementations.
- `db/migrations` should not import application code.

---

# 8. Open Questions

- The initial Go module path is `github.com/tonymontoya/ceph-atlas`. (Resolved.)
- Should the UI use Vite or another React build setup?
- Should OpenAPI be authored by hand first or generated from Go handlers later?
- Should migrations use a Go migration library, plain SQL files, or both?
- Should local orchestration start with Makefile plus direct commands, or a compose file?
- Should generated code be committed or produced during builds?

---

# 9. Near-Term Recommendation

After this document is accepted, create one final pre-code document:

- MVP test strategy

Then scaffold only the Mode 0 and Mode 1 layout:

- fake providers
- local PostgreSQL
- read-only contracts
- API health endpoint
- UI shell
- no real Ceph/Rook integration yet
- no mutations
- no Redis or NATS requirement yet

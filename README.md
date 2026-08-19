<p align="center">
  <img src="docs/assets/atlas-logo.png" alt="Atlas logo" width="192">
</p>

# Atlas

Atlas is an open-source operations platform for Ceph. It focuses on how organizations operate Ceph fleets: cases, workflows, policy, audit, RBAC, inventory, and safe automation.

Atlas is not a replacement for Ceph, Ceph Dashboard, Prometheus, Grafana, NetBox, OpenSearch, Splunk, or configuration management. It is the operational control plane that coordinates work across those systems.

## The problem Atlas solves

Ceph tells you *that* something is wrong — an OSD is down, a pool is near full, a host needs draining. It deliberately does not govern *how your team responds*. Operating a fleet today usually means some mix of these:

- Runbooks and tribal knowledge: the recovery procedure for a failed
  Storage Device lives in a wiki page, a chat thread, or one engineer's
  head, and drifts from what the cluster actually needs.
- Approval by chat: the destructive step between "we should replace this
  device" and `ceph orch osd rm` is a thumbs-up emoji with no durable
  record of who authorized what, when, or why.
- Risky manual mutations: operators paste commands against production
  storage, where a typo or a stale runbook can turn one dead drive into
  an outage.
- No operational history: once the incident closes, the reasoning, the
  approvals, and the exact steps taken are scattered or gone — nothing
  helps the next person facing the same failure.

Atlas closes that gap by making the *response itself* a durable, governed
object. A firing condition becomes a Case. A Case carries a Workflow:
typed, auditable steps where read-only collection runs automatically,
destructive mutations pause at an Approval Gate until a human records
authorization, and physical work pauses as a human Task. Every
transition — who did what, under whose approval, with what outcome — is
recorded on the Case Timeline. The result is a fleet where the safe path
is also the easy path, and where the operational history compounds
instead of evaporating.

If you want to contribute, these are the seams we are actively building:
the Case and Workflow model, the typed Agent operation contract, the
provider layer that reads real Ceph, and the attention-driven UI that
surfaces what needs a human. See [Status](#status) for what exists
today and [Roadmap](#roadmap) for where it goes next.

## Status

Atlas is in active early development as an open design-and-prototype project. It is **not production-ready** and should not be used to operate real Ceph clusters yet.

What exists today:

- Go backend with a REST API v1: read-only inventory and Case endpoints plus
  authenticated manual Case write endpoints (create, transition, assign, note)
- OIDC bearer-token identity verification (JWT against the issuer's JWKS) with
  a local dev issuer for development stacks
- React and TypeScript web UI scaffold with operator sign-in, manual Case
  writes, and Workflow attach/approve/resume forms
- PostgreSQL persistence with plain SQL migrations
- Fake-provider inventory fixtures and an inventory sync command
- A read-only real Ceph provider: `ATLAS_PROVIDER_MODE=ceph` points the
  inventory sync command at a live Ceph Dashboard REST API with a dedicated
  read-only user (ADR-0023). Explicit opt-in; local development and tests
  never require a real cluster (the provider is tested against an in-process
  fake Dashboard)
- Fake-provider alert evaluation that automatically creates a Case (with
  Timeline Events and deduplication) from a firing alert
- Seeded read-only Case and Case Timeline records
- A Workflow execution skeleton (ADR-0017 through ADR-0022): the Replace OSD
  Workflow attaches to a Case, pauses at its Approval Gate and human Task,
  and a fake Agent adapter drives its typed Jobs (with retry policies and
  idempotent re-dispatch) to terminal state — all state is durable in
  PostgreSQL
- A local Docker Compose stack for the full development environment,
  including the full fake workflow loop

What does not exist yet:

- Rook providers, a real alert source (alert detection reads fake Prometheus
  fixtures, not a live Prometheus), and pointing the API read source at a live
  cluster
- A real Atlas Agent or any mutating operation against Ceph (the fake Agent
  adapter only simulates Job execution)
- RBAC, policy, and Audit Events (any authenticated operator can approve
  gates and complete tasks; see ADR-0016)
- Notifications and cluster registration

## Roadmap

Atlas is working toward a production-usable single-zone 1.0. Each 0.x minor
is a coherent development milestone; stability commitments begin at 1.0.0.
The full ladder lives in [`dev-plans/roadmap.md`](dev-plans/roadmap.md).

- **v0.7 — Real Reads:** cluster registration, API reads against a live
  cluster, and a real Prometheus alert source creating Cases automatically
- **v0.8 — Safety Chain:** hierarchical RBAC, policy evaluation, and
  immutable Audit Events
- **v0.9 — Real Agent:** mutual TLS, typed approved operations, and the
  Replace OSD workflow executing real mutations end to end
- **v1.0 — Ship:** deployment artifacts, bootstrap runbook, user docs, and
  a security review — usable by a stranger, single-zone

After 1.0: Rook-managed Ceph support, chat notifications, and the broader
enterprise platform direction (NetBox, log links, external ticket trackers,
federation).

## Versioning

Atlas follows [Semantic Versioning](https://semver.org/). While Atlas is in
`0.x`, anything may change at any time and the API, database schema, and
configuration are not stable.

The initial public release will be tagged `v0.1.0`. During `0.x`, minor
versions (`v0.2.0`, `v0.3.0`, ...) mark coherent development milestones and may
include breaking changes, which will be described in the release notes.
Stability commitments begin at `1.0.0`.

The `/api/v1` path segment identifies the REST API contract direction; during
`0.x` it does not imply a stability guarantee.

## Documentation

- `dev-plans/product_vision.md` - product vision and principles
- `dev-plans/prd.md` - product requirements
- `dev-plans/hld.md` - high-level architecture
- `dev-plans/domain_model.md` - canonical product language
- `dev-plans/mvp.md` - v1.0 scope document
- `dev-plans/roadmap.md` - version ladder to 1.0 and post-1.0 direction
- `dev-plans/scale_tiers.md` - scale targets by deployment tier
- `dev-plans/environment_context.md` - example operating environment context and portability rules
- `dev-plans/ceph_compatibility.md` - Ceph version and cluster type compatibility posture
- `dev-plans/provider_api_research.md` - official Ceph and Rook API research for provider design
- `dev-plans/provider_contracts.md` - provider boundary and method contracts
- `dev-plans/case_timeline.md` - Case Timeline read model and event type proposal
- `dev-plans/local_development_topology.md` - local development modes and safety defaults
- `dev-plans/repository_layout.md` - initial source tree and package boundaries
- `dev-plans/mvp_test_strategy.md` - MVP test tiers and scaffold readiness criteria
- `dev-plans/security_review_checklist.md` - privileged operation review gate
- `dev-plans/public_readiness_checklist.md` - open-source publication audit trail
- `docs/adr/` - accepted architecture decisions
- `CONTEXT.md` - concise ubiquitous language for contributors

## Local Development

The narrow dependency-only path starts PostgreSQL on `127.0.0.1:15432`:

```sh
make db-up
```

The full local stack runs PostgreSQL, applies migrations, seeds one fake
inventory snapshot, runs one fake alert evaluation that creates a detected
Case, starts a local dev OIDC issuer on `127.0.0.1:18090`, starts the API on
`127.0.0.1:8080`, and starts the web UI on `127.0.0.1:5173`:

```sh
make dev-stack-up
```

If a local port is already occupied, override `ATLAS_API_PORT`,
`ATLAS_WEB_PORT`, or `ATLAS_DEV_ISSUER_PORT` for the compose process.

To sign in to the web UI or call write endpoints, request a dev token from the
dev issuer and use it as a bearer token:

```sh
curl -s -X POST http://127.0.0.1:18090/token | jq -r .token
```

### Try the workflow loop

The dev stack runs the API with the fake Agent adapter
(`ATLAS_AGENT_MODE=fake`), so the Replace OSD Workflow runs end to end
against simulated Job execution:

1. Open the web UI at `http://127.0.0.1:5173`, paste the dev token to sign
   in, and open the detected Case (`CephOSDDown on osd=1`) or create a new
   Case.
2. In the Workflows section of the Case detail view, attach `replace-osd`
   version `1`. The instance pauses at its Approval Gate
   (`waiting_for_approval`).
3. Approve the gate. The fake Agent executes the collect and destroy Jobs
   (visible with per-Job state and attempt counters) and the instance
   pauses at the human Task (`waiting_for_operator`).
4. Mark the device-replacement Task done. The final verification Job runs
   and the instance reaches terminal `succeeded`; every step is recorded
   as a Timeline Event on the Case.

The same loop is available over the REST API: `POST
/api/v1/cases/{id}/workflows`, `POST
/api/v1/workflow-instances/{id}/approvals`, and `POST
/api/v1/workflow-instances/{id}/task-completions` (see `api/openapi`).

Run the full-stack smoke check with:

```sh
make dev-stack-check
```

The smoke check exercises the whole loop — attach, approve, complete the
Task, terminal state, Timeline Events — plus 401-without-token assertions
on the write endpoints. It creates its own probe Case per run, so it stays
green against a persistent database volume.

Stop the full stack with:

```sh
make dev-stack-down
```

### Point inventory sync at a real Ceph cluster

The read-only real Ceph provider (ADR-0023) syncs inventory from a live
Ceph Dashboard REST API. It is explicit opt-in: the default local paths and
the dev stack never use it, and no credentials are required anywhere else.

Prerequisites on the Ceph side:

- Ceph 18 (Reef) with the Dashboard mgr module enabled and reachable
- a dedicated Dashboard user with a read-only role (for example
  `atlas-reader`)

Then run one sync with:

```sh
ATLAS_PROVIDER_MODE=ceph \
ATLAS_CEPH_DASHBOARD_URL=https://mon.example.invalid:8443 \
ATLAS_CEPH_DASHBOARD_USER=atlas-reader \
ATLAS_CEPH_DASHBOARD_PASSWORD='…' \
ATLAS_CEPH_CLUSTER_NAME=reef-lab \
go run ./cmd/atlas-inventory-sync
```

`ATLAS_CEPH_CLUSTER_NAME` is optional (defaults to `ceph`) because the
Dashboard API does not expose a cluster name. For lab clusters with
self-signed certificates, set `ATLAS_CEPH_DASHBOARD_INSECURE_TLS=true`.

Atlas validates its environment at startup and fails fast: every problem
is reported in one error naming the offending `ATLAS_*` variables.
`ATLAS_PROVIDER_MODE=ceph` requires the Dashboard URL to be an absolute
URL with a scheme plus the read-only credentials, in every command —
including `atlas-alert-eval`, whose alert source is still fake.

Scope notes: this path is read-only and writes one observation batch per
run through the same persistence as the fake provider; alert evaluation
still reads the fake Prometheus fixtures, and the API read source
(`ATLAS_READ_SOURCE`) still serves from the fake provider or PostgreSQL.

## Contributing

See `CONTRIBUTING.md`. Atlas is design-first: read `CONTEXT.md` and
`docs/adr/` before proposing changes that affect the domain model or
architecture. Please note this project has a `CODE_OF_CONDUCT.md`.

## Security

Atlas is not production-ready. See `SECURITY.md` for how to report
vulnerabilities privately.

## License

Apache License 2.0. See `LICENSE`.

## Trademarks

Atlas is an independent open-source project and is not affiliated with,
sponsored by, or endorsed by the Ceph project or Red Hat. "Ceph" is a
trademark or registered trademark of Red Hat, Inc. or its subsidiaries in
the United States and other countries.

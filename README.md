# Atlas

Atlas is an open-source operations platform for Ceph. It focuses on how organizations operate Ceph fleets: cases, workflows, policy, audit, RBAC, inventory, and safe automation.

Atlas is not a replacement for Ceph, Ceph Dashboard, Prometheus, Grafana, NetBox, OpenSearch, Splunk, or configuration management. It is the operational control plane that coordinates work across those systems.

## Status

Atlas is in active early development as an open design-and-prototype project. It is **not production-ready** and should not be used to operate real Ceph clusters yet.

What exists today:

- Go backend scaffold with a read-only REST API v1
- React and TypeScript web UI scaffold
- PostgreSQL persistence with plain SQL migrations
- Fake-provider inventory fixtures and an inventory sync command
- Seeded read-only Case and Case Timeline records
- A local Docker Compose stack for the full development environment

What does not exist yet:

- Real Ceph or Rook providers (the only provider is the fake provider)
- Atlas Agent and any mutating operation
- Authentication, RBAC, and audit enforcement

## Roadmap

The MVP is single-zone and aims to prove the core Atlas loop:

1. synchronize Ceph inventory for one zone
2. create and manage operational cases
3. enforce basic RBAC and audit
4. execute one tightly scoped workflow through an Atlas Agent
5. surface attention, assignments, approvals, and maintenance in the UI

Near-term work moves from the fake provider to read-only real Ceph and Rook providers. Federated global control-plane behavior remains a design goal, not an MVP requirement.

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
- `dev-plans/mvp.md` - first implementation slice
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
inventory snapshot, starts the API on `127.0.0.1:8080`, and starts the web UI on
`127.0.0.1:5173`:

```sh
make dev-stack-up
```

If a local port is already occupied, override `ATLAS_API_PORT` or
`ATLAS_WEB_PORT` for the compose process.

Run the full-stack smoke check with:

```sh
make dev-stack-check
```

Stop the full stack with:

```sh
make dev-stack-down
```

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

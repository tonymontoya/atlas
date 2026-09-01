# Atlas Local Development Topology
**Version:** 0.1 (Draft)  
**Status:** Pre-development Design  
**Audience:** Engineering, Architecture, Contributors  
**Project:** Atlas

---

# 1. Purpose

This document defines how contributors should run Atlas locally before and during early implementation.

The local topology must let developers work safely without production access, without AWX or Ansible, and without a full Ceph fleet. Real Ceph and Rook validation should be possible, but optional and explicit.

---

# 2. Principles

- Local development starts with fake providers.
- No production Ceph access is required for ordinary development.
- No AWX or Ansible runtime is required.
- Read-only provider flows come before mutating workflows.
- Mutating Agent operations are disabled by default.
- Real Ceph/Rook fixtures are opt-in validation paths.
- The same API and domain model should be used with fake, bare-metal, and Rook providers.
- Local state should be disposable.

---

# 3. Development Modes

## Mode 0: Unit Development

Purpose:

Fastest loop for domain, provider-contract, policy, RBAC, workflow-state, and API behavior.

Runs:

- Go unit tests
- TypeScript unit tests once UI exists
- fake providers
- in-memory or test database where appropriate

Does not run:

- PostgreSQL unless required by the package under test
- Redis
- NATS
- real Ceph
- real Rook
- Prometheus
- Atlas Agent

Required for:

- every commit once code exists

## Mode 1: Local App With Fake Providers

Purpose:

Primary developer environment for early Atlas.

Runs:

- Atlas API
- Atlas UI
- PostgreSQL
- fake Ceph provider
- fake Rook provider
- fake Observability provider
- fake Agent evidence provider

Optional:

- Redis when cache behavior is implemented
- NATS when async event delivery is implemented

Does not run:

- real Ceph
- real Rook
- real Prometheus
- real Atlas Agent mutation provider

Required for:

- API/UI development
- provider-normalization development
- Case and inventory workflows
- demo data

## Mode 2: Local App With Simulated Integrations

Purpose:

Exercise provider error handling and integration-shaped payloads without a real cluster.

Runs:

- Mode 1 services
- fixture-backed Ceph Dashboard API simulator
- fixture-backed Rook/Kubernetes API simulator where useful
- fixture-backed Prometheus API simulator

Does not run:

- real Ceph
- real Rook
- mutating Agent operations

Required for:

- parser/normalizer development
- error class validation
- read provider integration tests

## Mode 3: Ephemeral Ceph Validation

Purpose:

Validate Ceph 18 read provider behavior against a small non-production Ceph cluster.

Runs:

- Atlas API
- PostgreSQL
- bare-metal Ceph provider
- small local or lab Ceph 18 cluster

Optional:

- Ceph Dashboard API
- Prometheus
- Atlas Agent evidence provider in read-only mode

Does not run:

- mutating workflows by default

Required for:

- provider compatibility validation
- Ceph API assumptions
- inventory and health sync verification

## Mode 4: Ephemeral Rook Validation

Purpose:

Validate Rook provider behavior against a small non-production Rook-managed Ceph cluster.

Runs:

- Atlas API
- PostgreSQL
- Rook provider
- Kubernetes cluster
- Rook-managed Ceph 18

Optional:

- Ceph Dashboard API through Rook
- Prometheus
- Atlas Agent evidence provider in read-only mode

Does not run:

- mutating workflows by default
- Rook toolbox shell execution as a core path

Required for:

- Rook CRD assumptions
- Kubernetes/Rook deployment context
- read provider compatibility validation

## Mode 5: Controlled Lab Mutation

Purpose:

Validate typed Agent mutation operations only after safety, RBAC, approval, audit, workflow durability, and security review gates exist.

Runs:

- Atlas API
- PostgreSQL
- workflow engine
- policy engine
- audit service
- Atlas Agent
- non-production Ceph or Rook cluster

Required before enabling:

- security review checklist
- operation-specific safety contract
- idempotency contract
- rollback/escalation plan
- explicit lab approval

Does not run:

- production mutation
- arbitrary shell execution
- AWX/Ansible execution

---

# 4. Default Local Stack

The default local app should run:

- Atlas API
- Atlas UI
- PostgreSQL
- fake providers

The default local app should not require:

- Redis
- NATS
- Prometheus
- Ceph
- Rook
- Atlas Agent

Redis and NATS are part of the accepted implementation stack, but they should enter the local topology only when the code path under development actually needs them.

---

# 5. Fake Provider Requirements

Fake providers are required from day one.

They should provide deterministic scenarios:

- healthy Ceph 18 bare-metal cluster
- degraded Ceph 18 bare-metal cluster with one OSD down
- healthy Ceph 18 Rook cluster
- degraded Ceph 18 Rook cluster with one OSD down
- provider unavailable
- provider unauthorized
- malformed upstream response
- partial host/device inventory
- Prometheus alert that creates a Case

Fake provider data should be stored as fixtures, not embedded ad hoc in UI code.

Fake providers should exercise the same normalized Atlas models as real providers.

---

# 6. Fixture Policy

Fixtures should include:

- normalized Atlas output examples
- raw upstream payload examples where licensing and sensitivity allow
- error payload examples
- partial-result examples

Fixtures must not include:

- production secrets
- production tokens
- private keys
- real user credentials
- sensitive hostnames unless explicitly scrubbed
- live production endpoints

When using payloads derived from real systems, scrub them before committing.

---

# 7. Local Configuration

Local configuration should support:

- provider mode: `fake`, `ceph`, `rook`, `simulated`
- database URL
- fake scenario selection
- optional Prometheus endpoint
- optional Ceph Dashboard endpoint
- optional Kubernetes context
- disabled-by-default Agent mutation provider

Suggested local environment names:

- `ATLAS_PROVIDER_MODE`
- `ATLAS_FAKE_SCENARIO`
- `ATLAS_DATABASE_URL`
- `ATLAS_PROMETHEUS_URL`
- `ATLAS_CEPH_DASHBOARD_URL`
- `ATLAS_KUBECONFIG`
- `ATLAS_AGENT_MUTATION_ENABLED`

These names are suggestions for future implementation, not current code —
except the read-only Ceph path implemented in the v0.0.6 line (ADR-0023):
`ATLAS_PROVIDER_MODE=ceph` selects the bare-metal Dashboard provider for the
inventory sync command, configured through `ATLAS_CEPH_DASHBOARD_URL`,
`ATLAS_CEPH_DASHBOARD_USER`, `ATLAS_CEPH_DASHBOARD_PASSWORD`, optional
`ATLAS_CEPH_CLUSTER_NAME`, and optional
`ATLAS_CEPH_DASHBOARD_INSECURE_TLS`. This path is an explicit opt-in: the
default remains `fake`, no ordinary local development or test path reads
these variables, and selecting `ceph` without a complete configuration
fails fast.

---

# 8. Local Safety Defaults

Local development should default to:

- no mutating provider
- no production endpoints
- fake provider mode
- seed data resettable at any time
- explicit opt-in for real Ceph/Rook validation
- explicit opt-in for Agent evidence collection
- impossible-to-enable production mutation without separate configuration and approval gates

The first code scaffold should make the safe path the easy path.

---

# 9. Suggested Early Commands

These are target commands for future implementation.

```text
make test
make dev
make dev-fake
make fixtures-check
make provider-contract-test
make dev-reset
```

Do not implement these commands until the repository layout is defined.

---

# 10. Open Questions

- Should local orchestration use Docker Compose, Podman Compose, a Makefile around `podman`, or another tool?
- Should ephemeral Rook validation use kind, k3d, minikube, or an existing lab Kubernetes cluster?
- Should ephemeral bare-metal Ceph validation use containerized Ceph, a small lab cluster, or both?
- Should fixture-backed API simulators be hand-written test servers or generated from recorded payloads?
- Which parts of Redis and NATS are needed for the first scaffold, and can they remain absent until event delivery/caching is implemented?

---

# 11. Near-Term Recommendation

The first implementation scaffold should support Mode 0 and Mode 1 only.

That means:

- Go service skeleton
- React/TypeScript UI skeleton
- PostgreSQL migrations
- fake providers
- read-only provider contracts
- no Redis requirement yet
- no NATS requirement yet
- no real Ceph requirement yet
- no real Rook requirement yet
- no Agent mutation provider

Mode 2 should follow once provider normalizers need realistic upstream payloads.

Mode 3 and Mode 4 should follow once the read providers are ready to validate against real non-production clusters.

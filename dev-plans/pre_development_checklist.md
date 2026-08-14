# Atlas Pre-Development Checklist
**Version:** 0.1 (Draft)  
**Status:** Working Checklist  
**Audience:** Engineering, Architecture, Product, Contributors  
**Project:** Atlas

---

# 1. Purpose

This checklist captures the work that should be complete before implementation begins.

---

# 2. Addressed Gates

- [x] Narrow V1 into a real MVP.
  - See `dev-plans/mvp.md`.
  - Decision recorded in `docs/adr/0001-single-zone-mvp.md`.

- [x] Resolve the external ticket tracker integration inconsistency.
  - The external ticket tracker (for example, Jira) is optional during MVP.
  - External ticket tracking is a post-MVP V1 candidate unless a pilot requires it earlier.

- [x] Write initial ADRs before code.
  - See `docs/adr/0001-single-zone-mvp.md` through `docs/adr/0008-scale-by-deployment-tier.md`.

- [x] Threat-model the Atlas Agent early.
  - Agent safety constraints are captured in `docs/adr/0003-agent-typed-operations-only.md`.
  - The HLD now requires typed operations, explicit authorization context, and audit correlation.

- [x] Define the workflow engine contract.
  - Durable workflow state is captured in `docs/adr/0004-durable-workflow-state.md`.
  - The HLD now defines Workflow Instances and Jobs as durable state machines.

- [x] Make federation concrete, but do not build it first.
  - MVP is single-zone.
  - Federation remains a post-MVP architecture goal.

- [x] Create basic project hygiene.
  - Added `README.md`, `CONTRIBUTING.md`, `LICENSE`, `.gitignore`, `CONTEXT.md`, and `docs/adr/`.

- [x] Set scale tiers instead of one giant target.
  - See `dev-plans/scale_tiers.md`.
  - Decision recorded in `docs/adr/0008-scale-by-deployment-tier.md`.

- [x] Confirm the implementation stack.
  - Stack: Go backend and Agent, React + TypeScript UI, PostgreSQL, Redis, NATS, PostgreSQL FTS initially, OIDC, REST API v1, Kubernetes or Podman, OCI containers.
  - Decision recorded in `docs/adr/0009-implementation-stack.md`.

- [x] Confirm Atlas is not bound to AWX or Ansible.
  - AWX and Ansible may inform workflow discovery, but Atlas must work without those layers present.
  - Decision recorded in `docs/adr/0010-no-awx-or-ansible-runtime-dependency.md`.

- [x] Confirm environment context is descriptive, not product-specific.
  - Environment facts may inform MVP requirements, but Atlas must remain portable across Ceph operators.
  - Decision recorded in `docs/adr/0011-environment-context-is-descriptive.md`.

- [x] Define the first Ceph compatibility matrix.
  - Ceph 18 is the MVP primary target.
  - Ceph 16 is migration/read-only compatibility context.
  - Bare-metal Ceph and Rook-managed Ceph are equal first-class Cluster types.
  - See `dev-plans/ceph_compatibility.md`.
  - Decision recorded in `docs/adr/0012-ceph-18-and-cluster-type-parity.md`.

- [x] Research official Ceph 18 and Rook APIs before provider design.
  - See `dev-plans/provider_api_research.md`.
  - Key result: provider interfaces should be organized around Atlas intents, with Ceph Dashboard REST API, Ceph command API, Rook CRDs, Kubernetes API, Prometheus, and Atlas Agents hidden behind typed provider methods.

- [x] Define initial provider contracts.
  - See `dev-plans/provider_contracts.md`.
  - Key result: scaffold read-only providers first, with fake, bare-metal, and Rook implementations shaped around Atlas intents.

- [x] Define local development topology.
  - See `dev-plans/local_development_topology.md`.
  - Key result: start with fake providers and PostgreSQL; keep real Ceph/Rook validation optional and explicit; defer Redis, NATS, real agents, and mutations until needed.

- [x] Define initial repository layout.
  - See `dev-plans/repository_layout.md`.
  - Key result: Go backend and Agent under `cmd/` and `internal/`, React UI under `web/`, contracts under `api/`, migrations under `db/`, fake fixtures under `dev/fixtures/`, and deployment assets under `deploy/`.

- [x] Define MVP test strategy.
  - See `dev-plans/mvp_test_strategy.md`.
  - Key result: test fake providers, fixtures, normalized read models, API behavior, migrations, and workflow state before real provider validation or mutations.

- [x] Define security review checklist for privileged operations.
  - See `dev-plans/security_review_checklist.md`.
  - Key result: no mutating Agent operation may be implemented, tested against a real cluster, or enabled until operation definition, RBAC, policy, approval, audit, idempotency, Agent trust, secret handling, testing, rollout, and break-glass gates are satisfied.

---

# 3. Remaining Gates Before Code

All pre-code gates are complete.

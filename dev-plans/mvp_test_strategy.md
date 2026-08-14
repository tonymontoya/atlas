# Atlas MVP Test Strategy
**Version:** 0.1 (Draft)  
**Status:** Pre-development Design  
**Audience:** Engineering, Architecture, Contributors  
**Project:** Atlas

---

# 1. Purpose

This document defines the MVP test strategy before implementation begins.

The first test strategy should support a careful scaffold: fake providers, read-only flows, PostgreSQL persistence, REST API v1, and a UI shell. It should not require production access, real Ceph, real Rook, Redis, NATS, AWX, Ansible, or mutating Atlas Agent operations.

---

# 2. Testing Principles

- Test the Atlas domain model independently from provider transports.
- Test provider contracts with fake providers from day one.
- Test read-only flows before mutating workflows.
- Test normalized Atlas outputs, not raw upstream payload shapes.
- Keep tests deterministic and safe by default.
- Keep production endpoints and credentials out of tests.
- Make the fast test path the default path.
- Treat fixtures as test assets with review standards.
- Add mutation tests only after the security review checklist and Agent mutation contracts exist.

---

# 3. Test Tiers

## Tier 0: Static Checks

Purpose:

Catch formatting, lint, generated-code drift, and fixture hygiene problems quickly.

Should include:

- Go formatting
- Go vet/lint once selected
- TypeScript lint once UI exists
- fixture schema validation
- secret-pattern scan for fixtures
- OpenAPI validation once API contracts exist

Should not require:

- PostgreSQL
- Ceph
- Rook
- Prometheus
- Redis
- NATS

Target command:

```text
make lint
```

## Tier 1: Unit Tests

Purpose:

Validate domain behavior, provider normalization, policy decisions, config parsing, and API mapping without external services.

Should include:

- domain object validation
- Case lifecycle rules
- inventory normalization
- provider error normalization
- fake provider scenario loading
- RBAC decision units once implemented
- policy decision units once implemented
- workflow state transitions once implemented
- API handler units with fake dependencies

Should not require:

- PostgreSQL unless a package explicitly uses a test database
- Ceph
- Rook
- Prometheus
- Redis
- NATS

Target command:

```text
make test
```

## Tier 2: Local Integration Tests

Purpose:

Validate Atlas API, PostgreSQL persistence, migrations, fake providers, and read-only sync flows together.

Should include:

- migration up/down or migration apply validation
- API health endpoint
- cluster registration using fake provider mode
- health sync using fake providers
- inventory sync using fake providers
- OSD-down fake scenario creates or updates expected state
- Prometheus alert fixture creates Case input when alert-to-Case is implemented
- API response contract checks
- OIDC bearer-token verification (ephemeral keys; valid, expired, wrong
  audience, bad signature, missing subject, key rotation)
- authenticated manual Case writes (create, transition with closed-terminal
  conflicts, assignment, notes) including 401/400/404/409 error envelopes

Should require:

- PostgreSQL
- Atlas API test process
- fake providers

Should not require:

- real Ceph
- real Rook
- real Prometheus
- Redis
- NATS
- Atlas Agent mutation provider

Target command:

```text
make integration
```

## Tier 3: Contract Tests

Purpose:

Ensure all provider implementations obey the same Atlas-facing contracts.

Should include:

- `CephReadProvider` contract tests
- `CephSafetyProvider` read-only decision contract tests
- `RookDeploymentProvider` contract tests
- `AgentEvidenceProvider` fake/read-only contract tests
- `ObservabilityProvider` contract tests

Every provider contract test should check:

- success case
- unavailable provider
- unauthorized provider
- malformed response
- partial response where relevant
- unsupported cluster type where relevant
- normalized output shape
- no raw upstream payload leakage into domain objects

Initial required implementations:

- fake providers only

Later implementations:

- bare-metal Ceph read provider
- Rook read/deployment provider
- Prometheus observability provider
- Atlas Agent evidence provider

Target command:

```text
make provider-contract-test
```

## Tier 4: UI Tests

Purpose:

Validate that the UI can consume public REST API v1 and display fake-provider data without business logic in the browser.

Should include:

- component tests for key views
- API client tests with mocked REST responses
- fleet dashboard smoke test
- Case list/detail smoke test once implemented
- inventory view smoke test once implemented
- error-state rendering
- empty-state rendering

Should not require:

- production endpoints
- real Ceph
- real Rook

Target command:

```text
make web-test
```

## Tier 5: Optional Real Provider Validation

Purpose:

Validate read providers against non-production Ceph and Rook clusters.

Should include:

- Ceph 18 bare-metal read validation
- Ceph 18 Rook read validation
- Ceph 16 read-only validation where available
- provider timeout/error behavior
- Dashboard API availability diagnostics
- Rook CRD availability diagnostics

Should require explicit configuration:

- Ceph endpoint
- credentials
- Kubernetes context where applicable
- non-production target confirmation

Should not run by default.

Target command:

```text
make provider-validate
```

## Tier 6: Controlled Mutation Validation

Purpose:

Validate typed Agent mutations in a non-production lab only after later gates exist.

Required before this tier exists:

- security review checklist
- Agent mutation contract
- RBAC enforcement
- policy enforcement
- approval flow
- immutable audit records
- durable workflow state
- operation idempotency design
- lab-only target enforcement

Initial MVP scaffold should not implement this tier.

---

# 4. Fixture Strategy

Fixtures are first-class test inputs.

Required first fixtures:

- healthy Ceph 18 bare-metal cluster
- degraded Ceph 18 bare-metal cluster with one OSD down
- healthy Ceph 18 Rook cluster
- degraded Ceph 18 Rook cluster with one OSD down
- Ceph 16 read-only cluster
- provider unavailable
- provider unauthorized
- malformed provider response
- partial host/device inventory
- Prometheus OSD-down alert
- normalized Cluster identity
- normalized Health
- normalized OSD inventory
- normalized Host/Storage Device inventory
- normalized Case input from alert

All required first fixtures exist as of v0.3.0: Ceph read scenarios and
error directives under `dev/fixtures/ceph/`, Prometheus alert scenarios
under `dev/fixtures/prometheus/`, and normalized examples under
`dev/fixtures/normalized/` (including `case-input/osd-down-alert.json`,
pinned by a golden test in `internal/casedetection`).

Fixture rules:

- scrub all sensitive values
- avoid production endpoints
- avoid real credentials
- include provenance notes when derived from real payloads
- prefer small focused payloads over giant captured blobs
- keep raw upstream examples separate from normalized examples
- validate fixture JSON/YAML syntax in static checks

---

# 5. Database Test Strategy

PostgreSQL is the durable persistence target.

MVP tests should cover:

- migration application from an empty database
- migration idempotency expectations
- schema constraints for core objects
- basic seed data load for local fake-provider mode
- transaction behavior for sync writes
- append-only audit table behavior once audit exists
- durable workflow state persistence once workflows exist

Do not introduce Redis or NATS test dependencies until code paths require them.

---

# 6. API Test Strategy

REST API v1 tests should cover:

- health endpoint
- version endpoint if added
- cluster registration using fake providers
- list/get Cluster
- list/get Host
- list/get OSD
- list/get Case once Cases are implemented
- provider error mapping to HTTP responses
- authorization failures once RBAC is implemented
- OpenAPI contract conformance once OpenAPI exists

API tests should use fake providers by default.

---

# 7. Workflow Test Strategy

MVP workflow testing should start with read-only and state-machine behavior.

Before mutating workflows:

- Case creation from alert fixture
- Case creation from OSD-down fake provider scenario
- Workflow Instance creation
- Job state transitions
- resume after process restart
- safety check result attached as evidence
- approval-required state represented but not bypassed

After mutation gates:

- typed operation dispatch
- idempotency
- audit before and after execution
- rollback/escalation path

The first scaffold should not include mutating workflow execution.

---

# 8. Security Test Strategy

Before the security review checklist exists, MVP tests should still enforce safe defaults:

- mutating providers disabled by default
- fake provider mode by default
- no arbitrary command execution path
- no AWX/Ansible execution path
- no production endpoints in committed config
- fixture secret scan

After RBAC and audit exist:

- unauthorized users cannot execute privileged operations
- read-only access differs from operator access
- audit records are immutable
- approval decisions are attributable

---

# 9. CI Expectations

Initial CI should run:

```text
make lint
make test
make fixtures-check
```

Once PostgreSQL integration tests exist, CI should also run:

```text
make integration
```

CI should not require:

- production access
- Ceph
- Rook
- Prometheus
- Redis
- NATS
- AWX
- Ansible
- Atlas Agent mutation provider

---

# 10. Definition Of Test-Ready Scaffold

The first scaffold is test-ready when:

- `make test` runs Go unit tests
- fake providers have contract tests
- fixture validation exists
- database migrations can be applied locally
- API health endpoint has a test
- provider error normalization has tests
- no test requires production access
- no test requires real Ceph/Rook
- no test requires Redis/NATS
- no mutating operation test exists without security gates

---

# 11. Open Questions

- ~~Which Go lint tool should be selected?~~ Resolved: golangci-lint v2
  (standard linters, `.golangci.yml`, CI job) plus `go vet` in `make lint`.
- ~~Which TypeScript test runner should be selected with the UI scaffold?~~
  Resolved: vitest (already in use). TypeScript linting uses ESLint with
  typescript-eslint; `make web-lint` runs it.
- ~~Should provider contract tests be shared test suites that every implementation imports?~~
  Resolved: `internal/providers/contracttest` exports shared suites
  (`RunReadProviderSuite` and `RunObservabilityProviderSuite`) that every
  provider implementation wires into its own tests via a scenario factory.
  Run with `make provider-contract-test`.
- Should fixture schema be JSON Schema, Go validation code, or both?
  Provisional convention in place: fake-provider fixtures may carry an
  error-directive envelope (`{"error": {"class": ..., "message": ...}}`)
  validated in Go by the fake provider; `provider-malformed/*.json` are
  deliberately invalid JSON, which any future fixture syntax check must
  account for.
- Raw upstream payload leakage checks are not meaningfully testable against
  the fake provider (its fixtures are already normalized); they become a
  contract-suite requirement when a raw-payload provider implementation
  exists.
- Should migration tests use disposable local PostgreSQL containers or a developer-provided PostgreSQL instance?
- Should CI use GitLab services for PostgreSQL?

---

# 12. Near-Term Recommendation

Scaffold tests in this order:

1. Go unit test harness
2. fixture validation
3. fake provider contract tests
4. provider error normalization tests
5. API health endpoint test
6. PostgreSQL migration test
7. basic UI test harness

Only after these are stable should Atlas add real Ceph/Rook provider validation.

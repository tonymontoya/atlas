# Atlas Security Review Checklist
**Version:** 0.1 (Draft)  
**Status:** Pre-development Security Gate  
**Audience:** Engineering, Architecture, Security, Contributors  
**Project:** Atlas

---

# 1. Purpose

This checklist defines the review gate for privileged Atlas operations.

No mutating Atlas Agent operation should be implemented, tested against a real cluster, or enabled until this checklist is satisfied for that operation.

This document does not block read-only scaffolding with fake providers.

---

# 2. Scope

This checklist applies to:

- Atlas Agent mutation operations
- privileged Agent evidence collection
- Ceph operations that change cluster state
- Rook/Kubernetes operations that change Ceph deployment state
- host, device, power, firmware, or OOB operations
- break-glass workflow design

This checklist does not apply to:

- fake provider read paths
- read-only API health checks
- read-only fixture parsing
- read-only UI development
- local Mode 0/Mode 1 development with fake providers

---

# 3. Non-Negotiable Rules

- No arbitrary shell execution.
- No SSH proxying.
- No generic remote command API.
- No mandatory AWX-backed execution.
- No mandatory Ansible-backed execution.
- No production mutation from local development mode.
- No mutating operation without an operation-specific type.
- No mutating operation without RBAC, policy, approval, audit, and idempotency review.
- No mutating operation may rely only on UI-side checks.
- Unsafe operations are rejected, not merely warned, unless a break-glass workflow is explicitly approved.

---

# 4. Operation Definition Gate

Every privileged operation must define:

- operation name
- operation type identifier
- purpose
- target cluster type support
- target scope
- required inputs
- forbidden inputs
- expected preconditions
- expected postconditions
- timeout behavior
- retry behavior
- idempotency behavior
- rollback or escalation behavior
- verification steps
- failure states
- audit fields
- required permissions
- required approvals
- safety checks
- supported Ceph/Rook versions
- explicit unsupported versions or cluster types

The operation must be expressible without arbitrary command strings.

---

# 5. Authorization Gate

Before execution, Atlas must verify:

- authenticated user identity
- user role
- user permissions
- target scope
- cluster type
- operation permission
- approval authority if approval is required
- break-glass authority if break-glass is requested

Authorization must be enforced server-side.

The Atlas Agent must receive enough context to verify that the request is authorized, scoped, and tied to a Workflow Instance.

---

# 6. Policy And Safety Gate

Before execution, Atlas must evaluate:

- cluster health
- quorum safety where relevant
- replication safety where relevant
- capacity safety where relevant
- recovery/backfill state where relevant
- maintenance window rules
- stale maintenance flags
- target object current state
- conflicting active workflows
- version support
- provider support
- required approvals

Safety results must be stored as evidence on the Workflow Instance or Case.

Safety decisions are point-in-time. Long-running workflows must re-check safety before mutation.

---

# 7. Approval Gate

Approval-required operations must record:

- approver identity
- approver authority
- requested operation
- target scope
- safety evidence reviewed
- approval decision
- approval reason or comment
- timestamp
- expiration where applicable

Approval must be bound to the exact operation type, target, and workflow context. A broad approval must not authorize a different operation or target.

---

# 8. Audit Gate

Every privileged operation must create immutable audit records for:

- request received
- authorization decision
- policy/safety decision
- approval decision if applicable
- dispatch to Agent
- Agent acceptance or rejection
- operation result
- verification result
- failure or escalation

Audit records must include:

- user identity
- target scope
- operation type
- typed inputs or redacted input summary
- Workflow Instance
- Job ID
- Approval ID where applicable
- Agent identity
- source IP or source context where available
- timestamp
- result
- audit correlation ID

Audit records must not contain secrets.

---

# 9. Agent Trust Gate

Every Atlas Agent must have:

- unique identity
- mutual TLS certificate
- signed registration
- rotatable credentials
- scoped authorization
- heartbeat/status reporting

Every Agent request must include:

- operation type
- typed inputs
- target scope
- user identity
- Workflow Instance
- Job ID
- Approval context where required
- idempotency key where practical
- audit correlation ID
- request timestamp or nonce to reduce replay risk

Agents must reject:

- unknown operation types
- malformed inputs
- missing authorization context
- missing workflow context
- expired or replayed requests where detectable
- unsupported target scope
- unsupported local environment
- any request for arbitrary shell execution

---

# 10. Idempotency Gate

Each mutating operation must define:

- whether it is idempotent
- idempotency key format
- what state is checked before retry
- what state is written before execution
- what state is written after execution
- how duplicate dispatch is detected
- how partial success is represented
- how operator intervention is requested

Destructive steps must not execute twice for the same idempotency key.

---

# 11. Secrets And Sensitive Data Gate

Privileged operations must not:

- log secrets
- store raw credentials in audit records
- store private keys in fixtures
- expose secrets in UI responses
- include credentials in provider errors
- commit real endpoints or credentials to the repo

Secrets must come from approved runtime secret sources, not committed config.

Evidence bundles must redact sensitive values.

---

# 12. Testing Gate

Before lab mutation validation, tests must cover:

- unauthorized user rejected
- unsupported scope rejected
- unsupported cluster type rejected
- unsupported Ceph/Rook version rejected
- missing approval rejected
- stale approval rejected where expiration applies
- safety rejection
- Agent rejection
- duplicate idempotency key does not repeat destructive work
- audit records written for success
- audit records written for failure
- secrets are redacted
- fake provider mutation-disabled default

Before production enablement, tests must also cover:

- real non-production lab validation
- failure during execution
- partial success
- verification failure
- retry behavior
- operator escalation behavior

---

# 13. Rollout Gate

Every mutating operation must define rollout posture:

- disabled by default
- fake-only first
- lab-only second
- limited pilot third
- production enablement last

Production enablement requires:

- operation-specific review
- documented rollback/escalation
- owner sign-off
- security sign-off where required
- observability for operation outcomes
- runbook for failed or partial execution

---

# 14. Break-Glass Gate

Break-glass workflows are exceptional.

They require:

- explicit break-glass operation type
- elevated authorization
- reason
- time-bound scope
- stronger audit
- notification
- post-action review

Break-glass must not become a generic shell or SSH proxy.

---

# 15. Review Record

Every reviewed privileged operation should have a review record with:

- operation type
- reviewer
- review date
- checklist result
- open risks
- accepted risks
- required follow-up
- production enablement status

Review records may live beside operation design docs until an implementation location exists.

---

# 16. Initial Operation Candidates

Read-only or evidence operations may be considered first:

- collect host evidence
- collect device evidence
- validate replacement device
- collect SMART evidence

Mutating operations remain deferred:

- replace OSD execution
- OSD destroy/delete
- CRUSH changes
- pool creation/deletion
- cluster expansion
- cluster upgrade execution
- host reboot/power operations

---

# 17. Scaffold Implication

The first code scaffold may include:

- provider contracts
- fake providers
- mutation-disabled configuration
- audit package skeleton
- RBAC package skeleton
- policy package skeleton
- Agent package placeholder

The first code scaffold must not include:

- real mutating Agent operations
- shell command dispatch
- SSH proxying
- AWX/Ansible execution
- production mutation configuration

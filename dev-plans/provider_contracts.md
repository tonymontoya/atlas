# Atlas Provider Contracts
**Version:** 0.1 (Draft)  
**Status:** Pre-development Design  
**Audience:** Engineering, Architecture, Product, Contributors  
**Project:** Atlas

---

# 1. Purpose

This document defines the initial provider boundaries for Atlas.

The goal is to keep Atlas domain logic stable while allowing different execution and discovery paths for bare-metal Ceph, Rook-managed Ceph, Prometheus, and Atlas Agents.

This is still design work. It should guide repository layout and scaffolding, but it is not source code.

---

# 2. Design Principles

- Providers expose Atlas intents, not upstream transport details.
- Bare-metal Ceph and Rook-managed Ceph are equal first-class Cluster types.
- Ceph remains the storage source of truth.
- Rook is a Ceph deployment and management path, not a separate storage domain.
- Prometheus provides observability context, not inventory truth.
- Atlas Agents provide evidence and mutation through typed operations.
- AWX and Ansible may inform migration, but are not runtime dependencies.
- Read-only providers should be scaffolded before mutating providers.
- Mutating providers require RBAC, policy, approval, audit, and idempotency design before implementation.

---

# 3. Provider Families

## CephReadProvider

Reads Ceph state and normalizes it into Atlas domain objects.

Primary responsibilities:

- Cluster identity
- health
- capacity
- daemon inventory
- host inventory
- OSD inventory
- pool visibility
- CRUSH visibility
- RGW visibility where available

Expected implementations:

- `BareMetalCephReadProvider`
- `RookCephReadProvider`
- `FakeCephReadProvider`

## CephSafetyProvider

Evaluates whether a proposed Ceph operation is safe to prepare or execute.

Primary responsibilities:

- replication safety
- capacity safety
- quorum safety
- OSD delete/destroy safety
- recovery/backfill risk
- health-state gates
- maintenance flag awareness

Expected implementations:

- `BareMetalCephSafetyProvider`
- `RookCephSafetyProvider`
- `FakeCephSafetyProvider`

## RookDeploymentProvider

Reads Rook and Kubernetes deployment context for Rook-managed Ceph clusters.

Primary responsibilities:

- `CephCluster` status
- Rook-managed cluster mode
- Rook Ceph version image
- Rook dashboard exposure
- Rook Prometheus exposure
- Rook CRD desired state for pools, object stores, filesystems, and users where relevant
- Kubernetes workload context for Ceph daemons

Expected implementations:

- `KubernetesRookDeploymentProvider`
- `FakeRookDeploymentProvider`

## AgentEvidenceProvider

Collects host, device, and local environment evidence through Atlas Agents.

Primary responsibilities:

- host facts
- device facts
- SMART evidence
- replacement-device validation
- local daemon evidence
- hardware evidence where safe and non-mutating

Expected implementations:

- `AtlasAgentEvidenceProvider`
- `FakeAgentEvidenceProvider`

## AgentMutationProvider

Executes strongly typed privileged operations through Atlas Agents.

Primary responsibilities:

- execute approved typed operations
- enforce operation shape and target scope
- pass user, workflow, approval, and audit correlation context
- report durable operation status
- support idempotency where practical

Expected implementations:

- `AtlasAgentMutationProvider`
- `FakeAgentMutationProvider`

This provider is not part of the first read-only scaffold.

## ObservabilityProvider

Reads metrics, alerts, and SLO context.

Primary responsibilities:

- alert-to-Case inputs
- current alert context
- SLO/error-budget context
- capacity and recovery metric enrichment
- RGW/object-storage signal context

Expected implementations:

- `PrometheusObservabilityProvider`
- `FakeObservabilityProvider`

---

# 4. Contract Format

Every provider method should define:

- purpose
- read-only or mutating classification
- input
- output
- error classes
- idempotency expectation
- audit relevance
- supported MVP cluster types
- upstream interface candidates

Provider methods should return normalized Atlas concepts. They should not leak raw Ceph Dashboard, Ceph CLI, Kubernetes, Rook, or Prometheus response bodies as domain objects.

Raw upstream payloads may be stored as evidence when useful, but they must be clearly separated from normalized state.

---

# 5. Shared Error Classes

Provider methods should normalize failures into a small set of error classes.

| Error Class | Meaning | Example |
|---|---|---|
| `Unavailable` | Provider endpoint cannot be reached. | Ceph Dashboard unreachable. |
| `Unauthorized` | Credentials are missing or insufficient. | OIDC token lacks Ceph API access. |
| `Unsupported` | Provider cannot support the requested method for this cluster. | Rook-only method on bare-metal cluster. |
| `VersionUnsupported` | Ceph or Rook version is outside validated support. | Ceph 16 mutating operation requested. |
| `NotFound` | Requested object does not exist. | OSD ID no longer exists. |
| `Conflict` | Observed state changed while work was planned. | OSD state changed between precheck and execution. |
| `Unsafe` | Safety provider rejected the operation. | Cluster lacks recovery headroom. |
| `Partial` | Some data was collected but not all. | Host inventory missing for one host. |
| `MalformedResponse` | Upstream response could not be parsed safely. | Unexpected Dashboard API payload. |
| `Timeout` | Provider exceeded configured deadline. | Prometheus query timed out. |

Note on `Partial`: the Atlas read model currently represents `Partial` as an
error with no partial payload; the provider aborts the collection rather than
returning partial data. A partial-result shape (data plus error) may be
introduced when a consumer needs it.

---

# 6. CephReadProvider Methods

## ClusterIdentity

Purpose:

Return stable identity for a Ceph Cluster.

Classification:

Read-only.

Input:

- cluster connection reference

Output:

- Ceph FSID
- display name if available
- Ceph version summary if available
- provider-reported cluster type hint if available

Error classes:

- `Unavailable`
- `Unauthorized`
- `MalformedResponse`

Idempotency:

Read-only and safe to retry.

Audit relevance:

Operational audit not required. Access logging and sync history are useful.

Supported MVP cluster types:

- Ceph 18 bare-metal
- Ceph 18 Rook
- Ceph 16 read-only

Upstream interface candidates:

- Ceph Dashboard `/api/health/get_cluster_fsid`
- Ceph Dashboard `/api/summary`
- Ceph command/API fallback
- Rook `CephCluster` identity plus Ceph identity check

## Health

Purpose:

Return normalized cluster health.

Classification:

Read-only.

Input:

- cluster identity

Output:

- health status
- health checks
- severity
- summary text
- observed timestamp

Error classes:

- `Unavailable`
- `Unauthorized`
- `MalformedResponse`
- `Partial`

Idempotency:

Read-only and safe to retry.

Audit relevance:

No privileged audit required. Health observations should be part of sync history and Case evidence when used to open or update Cases.

Supported MVP cluster types:

- Ceph 18 bare-metal
- Ceph 18 Rook
- Ceph 16 read-only

Upstream interface candidates:

- Ceph Dashboard `/api/summary`
- Ceph Dashboard `/api/health/full`
- Ceph Dashboard `/api/health/minimal`
- Ceph command/API fallback
- Rook `CephCluster` status as deployment context

## Capacity

Purpose:

Return normalized cluster and pool capacity context.

Classification:

Read-only.

Input:

- cluster identity

Output:

- raw capacity
- used capacity
- available capacity
- usage ratio
- pool-level capacity where available
- near-full/full status where available

Error classes:

- `Unavailable`
- `Unauthorized`
- `MalformedResponse`
- `Partial`

Idempotency:

Read-only and safe to retry.

Audit relevance:

No privileged audit required. Capacity observations should be retained as Case evidence when they influence safety or planning.

Supported MVP cluster types:

- Ceph 18 bare-metal
- Ceph 18 Rook
- Ceph 16 read-only

Upstream interface candidates:

- Ceph Dashboard `/api/health/get_cluster_capacity`
- pool endpoints
- Ceph command/API fallback
- Prometheus enrichment

## Daemons

Purpose:

Return normalized Ceph daemon inventory.

Classification:

Read-only.

Input:

- cluster identity

Output:

- daemon ID
- daemon type
- host
- status
- version where available
- container/workload context where applicable

Error classes:

- `Unavailable`
- `Unauthorized`
- `MalformedResponse`
- `Partial`

Idempotency:

Read-only and safe to retry.

Audit relevance:

No privileged audit required. Daemon state may become Case evidence.

Supported MVP cluster types:

- Ceph 18 bare-metal
- Ceph 18 Rook
- Ceph 16 read-only

Upstream interface candidates:

- Ceph Dashboard `/api/daemon`
- host daemon endpoints
- Ceph command/API fallback
- Kubernetes workloads for Rook context

## Hosts

Purpose:

Return normalized host inventory known to Ceph.

Classification:

Read-only.

Input:

- cluster identity

Output:

- host identity
- address metadata where available
- daemon membership
- device visibility status
- Rook/Kubernetes node context where applicable

Error classes:

- `Unavailable`
- `Unauthorized`
- `MalformedResponse`
- `Partial`

Idempotency:

Read-only and safe to retry.

Audit relevance:

No privileged audit required. Host inventory sync should be recorded.

Supported MVP cluster types:

- Ceph 18 bare-metal
- Ceph 18 Rook
- Ceph 16 read-only

Upstream interface candidates:

- Ceph Dashboard host endpoints
- Ceph command/API fallback
- Kubernetes nodes and Rook context

## HostDevices

Purpose:

Return normalized device inventory associated with a Host.

Classification:

Read-only.

Input:

- cluster identity
- host identity

Output:

- storage device identity
- serial number where available
- device path where available
- health evidence where available
- OSD relationship where available
- evidence completeness

Error classes:

- `Unavailable`
- `Unauthorized`
- `NotFound`
- `MalformedResponse`
- `Partial`

Idempotency:

Read-only and safe to retry.

Audit relevance:

No privileged audit required unless collection requires privileged Agent evidence. Evidence used for replacement workflows should be attached to the Case.

Supported MVP cluster types:

- Ceph 18 bare-metal
- Ceph 18 Rook
- Ceph 16 read-only when available

Upstream interface candidates:

- Ceph Dashboard host device/inventory/SMART endpoints
- Atlas Agent evidence provider
- Rook discovery where configured

## OSDs

Purpose:

Return normalized OSD inventory and current OSD state.

Classification:

Read-only.

Input:

- cluster identity

Output:

- OSD ID
- host
- up/down state
- in/out state
- weight
- device relationship where available
- version where available
- Rook workload context where applicable

Error classes:

- `Unavailable`
- `Unauthorized`
- `MalformedResponse`
- `Partial`

Idempotency:

Read-only and safe to retry.

Audit relevance:

No privileged audit required. OSD observations can create or update Cases.

Supported MVP cluster types:

- Ceph 18 bare-metal
- Ceph 18 Rook
- Ceph 16 read-only

Upstream interface candidates:

- Ceph Dashboard OSD endpoints
- Ceph command/API fallback
- Kubernetes workloads for Rook context

## Pools

Purpose:

Return normalized pool inventory.

Classification:

Read-only.

Input:

- cluster identity

Output:

- pool name
- pool ID where available
- pool type
- size/min-size where available
- PG count where available
- usage where available
- CRUSH rule relationship where available

Error classes:

- `Unavailable`
- `Unauthorized`
- `MalformedResponse`
- `Partial`

Idempotency:

Read-only and safe to retry.

Audit relevance:

No privileged audit required.

Supported MVP cluster types:

- Ceph 18 bare-metal
- Ceph 18 Rook
- Ceph 16 read-only

Upstream interface candidates:

- Ceph Dashboard pool endpoints
- Ceph command/API fallback
- Rook `CephBlockPool` desired state where applicable

---

# 7. CephSafetyProvider Methods

## CheckOperationSafety

Purpose:

Evaluate whether an operation may proceed, requires approval, should be delayed, or must be rejected.

Classification:

Read-only decision support. It does not mutate Ceph.

Input:

- operation type
- target cluster
- target object
- requested time
- workflow context
- current health/capacity/inventory evidence

Output:

- decision: `Allow`, `Reject`, `RequireApproval`, `Delay`
- reasons
- evidence references
- required approvals
- recommended recheck interval if delayed

Error classes:

- `Unavailable`
- `Unsupported`
- `VersionUnsupported`
- `Unsafe`
- `Partial`

Idempotency:

Read-only and safe to retry. Results are point-in-time and must not be reused indefinitely.

Audit relevance:

Safety decisions that affect workflow progress should be attached to the Workflow Instance and Case Timeline. Rejections and approvals are audit-relevant.

Supported MVP cluster types:

- Ceph 18 bare-metal
- Ceph 18 Rook
- Ceph 16 read-only for decision support only

Upstream interface candidates:

- Ceph Dashboard safety endpoints such as `safe_to_delete` and `safe_to_destroy`
- Ceph MON command/API safety checks
- normalized Atlas health/capacity/inventory state
- Rook deployment context

---

# 8. RookDeploymentProvider Methods

## ClusterDeploymentStatus

Purpose:

Return Rook/Kubernetes deployment context for a Rook-managed Ceph Cluster.

Classification:

Read-only.

Input:

- Kubernetes cluster reference
- namespace
- Rook `CephCluster` name

Output:

- Rook cluster phase/status
- Ceph image/version configuration
- cluster mode
- dashboard configuration
- external-cluster flag where applicable
- relevant condition summaries

Error classes:

- `Unavailable`
- `Unauthorized`
- `NotFound`
- `Unsupported`
- `MalformedResponse`

Idempotency:

Read-only and safe to retry.

Audit relevance:

No privileged audit required. Deployment state can become Case evidence.

Supported MVP cluster types:

- Ceph 18 Rook
- Ceph 16 Rook if present and read-only

Upstream interface candidates:

- Kubernetes API
- Rook `CephCluster` CRD

## RookResources

Purpose:

Return relevant Rook CRD desired state for Ceph resources.

Classification:

Read-only.

Input:

- Kubernetes cluster reference
- namespace
- resource filters

Output:

- block pools
- object stores
- filesystems
- clients
- object-store users
- relevant status/conditions where available

Error classes:

- `Unavailable`
- `Unauthorized`
- `NotFound`
- `Unsupported`
- `MalformedResponse`
- `Partial`

Idempotency:

Read-only and safe to retry.

Audit relevance:

No privileged audit required.

Supported MVP cluster types:

- Ceph 18 Rook
- Ceph 16 Rook if present and read-only

Upstream interface candidates:

- Kubernetes API
- Rook CRDs

---

# 9. AgentEvidenceProvider Methods

## CollectHostEvidence

Purpose:

Collect host-local evidence through an authenticated Atlas Agent.

Classification:

Read-only, but privileged.

Input:

- host identity
- evidence request type
- workflow or Case context where applicable

Output:

- evidence bundle ID
- normalized host facts
- raw evidence references where retained
- collection completeness

Error classes:

- `Unavailable`
- `Unauthorized`
- `NotFound`
- `Timeout`
- `Partial`

Idempotency:

Safe to retry. Each collection creates a new timestamped evidence bundle.

Audit relevance:

Audit required because an authenticated Agent performs privileged local collection.

Supported MVP cluster types:

- Ceph 18 bare-metal
- Ceph 18 Rook when host evidence is needed
- Ceph 16 read-only when explicitly enabled

Upstream interface candidates:

- Atlas Agent typed evidence operation

## ValidateReplacementDevice

Purpose:

Verify that a replacement Storage Device matches the requested target and appears safe to use.

Classification:

Read-only, but privileged.

Input:

- host identity
- expected serial number
- target OSD context where available
- workflow context

Output:

- validation status
- matched device identity
- SMART summary where available
- reasons
- evidence bundle ID

Error classes:

- `Unavailable`
- `Unauthorized`
- `NotFound`
- `Conflict`
- `Unsafe`
- `Timeout`
- `Partial`

Idempotency:

Safe to retry. Results are point-in-time evidence.

Audit relevance:

Audit required. This evidence may gate a mutating replacement workflow.

Supported MVP cluster types:

- Ceph 18 bare-metal
- Ceph 18 Rook where direct device replacement applies
- Ceph 16 read-only by exception only

Upstream interface candidates:

- Atlas Agent typed evidence operation

---

# 10. AgentMutationProvider Methods

## ExecuteTypedOperation

Purpose:

Execute one approved privileged operation through an Atlas Agent.

Classification:

Mutating.

Input:

- operation type
- typed operation inputs
- target scope
- user identity
- Workflow Instance
- Job ID
- Approval context where required
- idempotency key where practical
- audit correlation ID

Output:

- operation status
- started/completed timestamps
- result payload
- verification hints
- error class if failed

Error classes:

- `Unavailable`
- `Unauthorized`
- `Unsupported`
- `VersionUnsupported`
- `Conflict`
- `Unsafe`
- `Timeout`

Idempotency:

Required where practical. The same idempotency key must not execute a destructive step twice.

Audit relevance:

Full immutable audit required before and after execution.

Supported MVP cluster types:

- Ceph 18 bare-metal after validation
- Ceph 18 Rook after validation
- Ceph 16 mutating operations deferred unless explicitly validated

Upstream interface candidates:

- Atlas Agent typed operation provider

Implementation status:

Deferred until read providers, RBAC, policy, approval, audit, durable workflow state, and security review checklist exist.

---

# 11. ObservabilityProvider Methods

## CurrentAlerts

Purpose:

Return current alert context for a Cluster or target object.

Classification:

Read-only.

Input:

- cluster identity
- optional target scope
- alert filters

Output:

- alert name
- severity
- labels
- annotations
- start time
- current state
- source reference

Error classes:

- `Unavailable`
- `Unauthorized`
- `Timeout`
- `MalformedResponse`

Idempotency:

Read-only and safe to retry.

Audit relevance:

No privileged audit required. Alerts that create or update Cases should be referenced as Case evidence.

Supported MVP cluster types:

- Ceph 18 bare-metal
- Ceph 18 Rook
- Ceph 16 read-only

Upstream interface candidates:

- Prometheus
- Alertmanager if selected later
- Ceph Dashboard Prometheus endpoints where useful

---

# 12. Read-Only MVP Flow Order

The first scaffold should prove read paths in this order:

1. ClusterIdentity
2. Health
3. Capacity
4. Daemons
5. OSDs
6. Hosts
7. HostDevices
8. Pools
9. CurrentAlerts

Each flow should support:

- fake provider implementation
- bare-metal provider implementation
- Rook provider implementation where applicable
- normalized Atlas output
- sync history
- error normalization

---

# 13. Explicit Deferrals

Do not implement these until later design gates are complete:

- Replace OSD mutation steps
- OSD destroy/delete execution
- CRUSH changes
- pool creation or deletion
- cluster expansion
- cluster upgrade execution
- host reboot or power operations
- arbitrary command execution
- AWX-backed or Ansible-backed execution
- Rook toolbox shell execution as a core pathway

These may be researched as future workflows, but they should not shape the first code scaffold.

---

# 14. Acceptance Questions

Before scaffolding code, this document should let the team answer:

- Can bare-metal and Rook share one Atlas domain model?
- Which read interfaces are stable enough to scaffold first?
- Which provider methods need fake implementations from day one?
- Which evidence collections require audit even though they are read-only?
- Which provider outputs become Case evidence?
- What must never be provider-specific?
- Which provider methods are intentionally unsupported for Ceph 16?

Current answers:

- Yes, bare-metal and Rook share one Atlas domain model.
- Read interfaces are stable enough to scaffold first if wrapped behind providers.
- Fake providers are required from day one.
- Agent evidence collection requires audit because it is privileged even when read-only.
- Health, alert, OSD, host, device, and safety outputs may become Case evidence.
- RBAC, policy, audit, Case, Workflow, Timeline, and domain objects must not be provider-specific.
- Ceph 16 mutating operations are deferred unless explicitly validated.

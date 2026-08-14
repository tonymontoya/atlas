# Atlas Ceph Compatibility Matrix
**Version:** 0.1 (Draft)  
**Status:** Pre-development Compatibility Document  
**Audience:** Engineering, Architecture, Product, Contributors  
**Project:** Atlas

---

# 1. Purpose

This document defines the initial Ceph compatibility posture for Atlas MVP.

Decision reference: `docs/adr/0012-ceph-18-and-cluster-type-parity.md`.

---

# 2. MVP Version Targets

| Ceph Version | MVP Posture | Notes |
|---|---|---|
| Ceph 18 | Primary target | First-class target for inventory, health, Cases, workflows, policy checks, and Agent operations. |
| Ceph 16 | Compatibility target | Important during migration. Start with read-only inventory, health, and reporting unless a mutating operation is explicitly validated. |

Ceph versions outside this table are not rejected permanently, but they are not MVP validation targets.

---

# 3. Cluster Types

| Cluster Type | MVP Posture | Notes |
|---|---|---|
| Bare-metal Ceph | First-class | Direct Ceph provider path for non-Rook clusters. |
| Rook-managed Ceph | First-class | Kubernetes/Rook provider path for clusters managed through Rook. |

Bare-metal and Rook-managed clusters must share the same Atlas domain model and user-facing workflow semantics.

Rook support means Atlas understands Kubernetes/Rook as a Ceph deployment and management path. It does not mean Atlas treats Kubernetes as the storage system of record; Ceph remains the storage system of record.

---

# 4. Provider Boundary

Atlas should model Ceph operations by intent, then route implementation through provider interfaces.

Examples:

| Intent | Bare-metal Provider | Rook Provider |
|---|---|---|
| Collect cluster health | supported Ceph API or CLI fallback | Rook toolbox/API path or Kubernetes-mediated Ceph access |
| Synchronize OSD inventory | supported Ceph API or CLI fallback | Rook/Kubernetes plus Ceph data where needed |
| Collect device evidence | host-local typed Agent operation | Kubernetes/Rook-aware typed Agent operation |
| Replace OSD | typed Agent operation, validated before mutation | typed Agent operation, validated before mutation |

Provider implementations may differ, but RBAC, policy, approval, audit, Case, Workflow, and Timeline behavior must remain consistent.

---

# 5. MVP Capability Matrix

| Capability | Ceph 18 Bare Metal | Ceph 18 Rook | Ceph 16 Bare Metal | Ceph 16 Rook |
|---|---|---|---|---|
| Cluster registration | Required | Required | Read-only compatible | Read-only compatible if present |
| Health sync | Required | Required | Required | Required if present |
| Inventory sync | Required | Required | Required | Required if present |
| OSD failure Case creation | Required | Required | Required | Required if present |
| Replace OSD Workflow | Required after validation | Required after validation | Deferred unless validated | Deferred unless validated |
| Upgrade readiness report | Required | Required | Useful for migration | Useful for migration if present |
| RGW signal visibility | Useful | Useful | Useful | Useful if present |

---

# 6. Interface Preference

Atlas should use:

1. supported Ceph APIs
2. supported Ceph CLI where APIs are unavailable
3. Kubernetes/Rook APIs for Rook deployment context
4. direct host or device operations only through typed Atlas Agent operations

Atlas should not use:

- unsupported Ceph internals as stable contracts
- arbitrary shell execution
- SSH proxying
- mandatory AWX-backed execution
- mandatory Ansible-backed execution

---

# 7. Open Validation Work

- Identify exact Ceph 18 minor versions used by first pilot clusters.
- Identify exact Ceph 16 minor versions still present during migration.
- Map each MVP operation to its supported Ceph API, CLI fallback, or Rook/Kubernetes provider path.
- Define test fixtures for Ceph 18 bare-metal and Ceph 18 Rook clusters.
- Decide whether Ceph 16 mutating operations are explicitly unsupported or supported only by exception.

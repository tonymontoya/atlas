# Atlas Ceph Compatibility Matrix
**Version:** 0.2  
**Status:** Revised (2026-09-01) — Ceph 16 retired, 19/20 staged  
**Audience:** Engineering, Architecture, Product, Contributors  
**Project:** Atlas

---

# 1. Purpose

This document defines the Ceph compatibility posture for Atlas.

Decision reference: `docs/adr/0012-ceph-18-and-cluster-type-parity.md`
(including the 2026-09-01 amendment on the version floor and breadth).

---

# 2. Version Targets

**Support floor: Ceph 18 and newer.** Older versions are not validation
targets and are not rejected permanently, but nothing guarantees behavior
on them.

| Ceph Version | Posture | Notes |
|---|---|---|
| Ceph 18 (Reef) | Primary target | First-class target for inventory, health, Cases, workflows, policy checks, and Agent operations through v0.1.0. |
| Ceph 19 | Read compatibility (from v0.0.11) | Read paths validated against version-shaped Dashboard fixtures; mutating operations validate with v0.3.0. |
| Ceph 20 | Read compatibility (from v0.0.11) | Read paths validated against version-shaped Dashboard fixtures; mutating operations validate with v0.3.0. |

Ceph 16 (Pacific) was a pre-development read-only migration target. It was
retired on 2026-09-01: the migration context is obsolete, the support floor
is Ceph 18, and the `pacific-readonly` fixture was removed.

The primary Ceph version is re-pinned from fleet reality at v0.3.0, when
Rook-managed clusters (which run newer Ceph) become first-class and Ceph
19/20 mutation validation lands.

---

# 3. Cluster Types

| Cluster Type | Posture | Notes |
|---|---|---|
| Bare-metal Ceph | First-class | Direct Ceph provider path for non-Rook clusters. |
| Rook-managed Ceph | First-class (provider lands v0.3.0) | Kubernetes/Rook provider path for clusters managed through Rook. |

Bare-metal and Rook-managed clusters must share the same Atlas domain model and user-facing workflow semantics.

Rook support means Atlas understands Kubernetes/Rook as a Ceph deployment and management path. It does not mean Atlas treats Kubernetes as the storage system of record; Ceph remains the storage system of record.

---

# 4. Provider Boundary

Atlas should model Ceph operations by intent, then route implementation
through provider interfaces.

Examples:

| Intent | Bare-metal Provider | Rook Provider |
|---|---|---|
| Collect cluster health | supported Ceph API or CLI fallback | Rook toolbox/API path or Kubernetes-mediated Ceph access |
| Synchronize OSD inventory | supported Ceph API or CLI fallback | Rook/Kubernetes plus Ceph data where needed |
| Collect device evidence | host-local typed Agent operation | Kubernetes/Rook-aware typed Agent operation |
| Replace OSD | typed Agent operation, validated before mutation | typed Agent operation, validated before mutation |

Provider implementations may differ, but RBAC, policy, approval, audit, Case, Workflow, and Timeline behavior must remain consistent.

Between-release Dashboard REST API drift is absorbed inside the provider
package behind the shared provider contract (ADR-0023); widening the
validated read matrix to Ceph 19/20 (v0.0.11) means adding
version-shaped fixtures and read contract coverage, not new contracts.

---

# 5. Capability Matrix

| Capability | Ceph 18 | Ceph 19 | Ceph 20 |
|---|---|---|---|
| Cluster registration | Required | Supported | Supported |
| Health sync | Required | Validated v0.0.11 | Validated v0.0.11 |
| Inventory sync | Required | Validated v0.0.11 | Validated v0.0.11 |
| OSD failure Case creation | Required | Via validated reads | Via validated reads |
| Replace OSD Workflow | Required after validation | Mutation-validated v0.3.0 | Mutation-validated v0.3.0 |
| Upgrade readiness report | Required | Useful | Useful |
| RGW signal visibility | Useful | Useful | Useful |

Rook-managed variants of every capability arrive with the v0.3.0 Rook
milestone and carry the same per-version validation as bare-metal.

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
- Identify which Ceph 19 and 20 minors the pilot fleets run, to shape the
  v0.0.11 fixture set.
- Map each mutating operation to its supported Ceph API, CLI fallback, or
  Rook/Kubernetes provider path per version (v0.3.0 line).
- Define test fixtures for Ceph 19 and Ceph 20 Dashboard shapes (v0.0.11).
- Re-pin the primary Ceph version from fleet reality at v0.3.0.

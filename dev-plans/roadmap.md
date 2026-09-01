# Atlas Roadmap
**Version:** 0.1  
**Status:** Accepted (2026-08-19)  
**Audience:** Engineering, Product, Contributors  
**Project:** Atlas  

---

# 1. Purpose

This is the canonical version ladder from the current line to Atlas 0.1.0 and
the prioritized direction after it. `dev-plans/prd.md` defines the long-term
product; `dev-plans/mvp.md` defines the v0.1.0 scope; this file sequences the
work. The README summarizes this file; where they disagree, this file wins.

---

# 2. The Realignment

Atlas 0.1.0 is the first production-usable single-zone release. It carries the
scope the PRD originally called "MVP"; the PRD's former "Version 1"
enterprise vision (fleet scale, broad integrations, search, dashboards,
federation) is the post-0.1.0 direction.

**Renumber, 2026-09-01:** the first seven releases were originally
published as `v0.1.0` through `v0.6.1`, with this document targeting
`v1.0` as the first production-usable release. They were renumbered down
to `v0.0.1` through `v0.0.7` — before the project had any users or
published binaries — because the work was not far enough along to justify
consuming the `0.x` minor line. The first production-usable target moved
from `1.0` to `v0.1.0`, development milestones live in `0.0.x` patches,
and the intermediate rungs were remapped (`v0.7` → `v0.0.8`, `v0.8` →
`v0.0.9`, `v0.9` → `v0.0.10`). The Go module proxy retains cached copies
of the old tags; those version strings will never be reused.

Ground rules for the ladder:

- Safety before mutation: RBAC, policy, and Audit Events land before the
  first real Ceph mutation executes.
- Fake-first: every capability is proven on fake providers and local stacks
  before it touches a real cluster.
- Each `0.0.x` patch is a coherent development milestone; breaking changes
  are allowed and described in the release notes.
- From `v0.1.0`, minors carry the stability commitment: patches are always
  safe, breaking changes land only at minor boundaries (`v0.2.0`,
  `v0.3.0`, ...) with migration notes, and deprecations are flagged one
  minor ahead. `1.0` later marks maturity, not a rule change.

---

# 3. Version Ladder

## v0.0.6 — The Real Ceph Read Provider line (complete)

v0.0.6 shipped the read-only Ceph Dashboard provider; v0.0.7 shipped the
internal-architecture hardening pass (app-level error taxonomy,
guarded-transition store helpers, one Actor type, typed inventory status,
dashtest provider-welding helpers) with the alert-eval fail-fast behavior
change documented in its release notes.

## v0.0.8 — Registered Reads — Current line

Real clusters report in through enrolled Atlas Agents (ADR-0025):

- Cluster Registration and Enrollment for bare-metal Ceph
- Read-only Atlas Agent: collects over the Dashboard REST API inside the
  cluster's trust domain and pushes observations to Atlas
- Real alert ingestion: live alerts create Cases automatically
- Removal of the control-plane pull path (`ATLAS_PROVIDER_MODE=ceph`)
- No mutation

## v0.0.9 — Safety Chain

The guardrails before the first real mutation:

- Hierarchical RBAC (Organization → Zone → Datacenter → Cluster → Host)
- Policy evaluation: approval requirements, maintenance windows, safety
  checks
- Immutable Audit Events for privileged operations

## v0.0.10 — Real Mutations

Real mutation through the typed-operation model, dispatched to the Agents
enrolled since v0.0.8:

- Mutual TLS hardening, credential rotation, and revocation for the Agent channel
- Typed, approved, idempotent Job execution against real Ceph
- Replace OSD executing real operations end to end, with recovery
  monitoring and verification

## v0.0.11 — Read Matrix Widening

Ceph version breadth on the read path, ahead of Ship:

- Ceph 19 and Ceph 20 Dashboard fixtures alongside the Ceph 18 (Reef) set
- Read contract coverage per version; between-release Dashboard endpoint
  drift stays absorbed behind the provider contract (ADR-0023)
- Read-only: mutation validation for Ceph 19/20 stays post-Ship (v0.3.0)

## v0.1.0 — Ship

Usable by a stranger:

- Published container images and a production-shaped single-zone
  deployment path
- Bootstrap runbook: OIDC issuer, first admin, cluster registration,
  Agent enrollment
- User documentation
- Security review pass (`dev-plans/security_review_checklist.md`)
- PostgreSQL backup/restore note
- Upgrade path from v0.0.11
- No new features

v0.1.0 acceptance criteria live in `dev-plans/mvp.md`.

---

# 4. Post-v0.1.0 Direction

Versioned milestones after Ship, in priority order:

## v0.2.0 — Federation

Global control plane, multi-zone synchronization, cross-zone workflows —
first: Atlas's first target customer operates 50+ clusters across 15
datacenters in 3 global zones and will not adopt until multi-zone,
multi-control-plane features exist. Each regional Atlas deployment stays
single-zone per ADR-0001; federation links them.

## v0.3.0 — Rook and Version Breadth

- Rook-managed Ceph as an equal first-class cluster type (v0.1.0 ships
  bare-metal first)
- Ceph 19/20 mutation validation rides the Rook work (Rook fleets run
  newer Ceph); the primary Ceph version is re-pinned from fleet reality
  at this milestone

## After v0.3.0

1. Chat notifications (for example, Slack)
2. Enterprise line: NetBox as a required inventory source,
   OpenSearch/Splunk contextual log links, external ticket tracker work
   request creation, scheduler and maintenance windows, richer policy
   language, additional workflows (Drain Host, Restart Daemon, Create Pool,
   Expand Cluster, Cluster Upgrade, Host Maintenance, Cluster Health
   Investigation), fleet-wide reporting, unified search

---

# 5. Tracker

GitHub milestones mirror this ladder (`v0.0.8` through `v0.1.0`, then
`v0.2.0`, `v0.3.0`). Each past release has a closed epic issue linking its
release notes. Forward work is ticketed against these milestones; deferred
items stay in this file, not the tracker.

# Atlas Roadmap
**Version:** 0.1  
**Status:** Accepted (2026-08-19)  
**Audience:** Engineering, Product, Contributors  
**Project:** Atlas  

---

# 1. Purpose

This is the canonical version ladder from the current line to Atlas 1.0.0 and
the prioritized direction after it. `dev-plans/prd.md` defines the long-term
product; `dev-plans/mvp.md` defines the v1.0 scope; this file sequences the
work. The README summarizes this file; where they disagree, this file wins.

---

# 2. The Realignment

Atlas 1.0 is the first production-usable single-zone release. It carries the
scope the PRD originally called "MVP"; the PRD's former "Version 1"
enterprise vision (fleet scale, broad integrations, search, dashboards,
federation) is the post-1.0 direction.

Ground rules for the ladder:

- Safety before mutation: RBAC, policy, and Audit Events land before the
  first real Ceph mutation executes.
- Fake-first: every capability is proven on fake providers and local stacks
  before it touches a real cluster.
- Each 0.x minor is a coherent development milestone; breaking changes are
  allowed and described in the release notes. Stability commitments begin
  at 1.0.0.

---

# 3. Version Ladder

## v0.6.x — Current line

Finish the internal architecture candidates (app-level error taxonomy,
guarded-transition store helpers, one Actor type, typed inventory status,
dashtest provider-welding helpers) and ship the v0.6.1 release notes,
including the alert-eval fail-fast behavior change.

## v0.7 — Real Reads

Point the working read loop at real clusters:

- Cluster registration for bare-metal Ceph over the Dashboard REST API
- API read source against a live cluster (`ATLAS_READ_SOURCE=ceph`)
- Real Prometheus alert source: live alerts create Cases automatically
- No mutation

## v0.8 — Safety Chain

The guardrails before the first real mutation:

- Hierarchical RBAC (Organization → Zone → Datacenter → Cluster → Host)
- Policy evaluation: approval requirements, maintenance windows, safety
  checks
- Immutable Audit Events for privileged operations

## v0.9 — Real Agent

Real mutation through the typed-operation model:

- Atlas Agent service with mutual TLS
- Typed, approved, idempotent Job execution against real Ceph
- Replace OSD executing real operations end to end, with recovery
  monitoring and verification

## v1.0 — Ship

Usable by a stranger:

- Published container images and a production-shaped single-zone
  deployment path
- Bootstrap runbook: OIDC issuer, first admin, cluster registration,
  Agent enrollment
- User documentation
- Security review pass (`dev-plans/security_review_checklist.md`)
- PostgreSQL backup/restore note
- Upgrade path from v0.9
- No new features

v1.0 acceptance criteria live in `dev-plans/mvp.md`.

---

# 4. Post-1.0 Direction

In priority order:

1. Rook-managed Ceph as an equal first-class cluster type (v1.0 ships
   bare-metal first)
2. Chat notifications (for example, Slack)
3. Enterprise line: NetBox as a required inventory source,
   OpenSearch/Splunk contextual log links, external ticket tracker work
   request creation, scheduler and maintenance windows, richer policy
   language, additional workflows (Drain Host, Restart Daemon, Create Pool,
   Expand Cluster, Cluster Upgrade, Host Maintenance, Cluster Health
   Investigation), fleet-wide reporting, unified search
4. Federation: global control plane, multi-zone synchronization, cross-zone
   workflows — last

---

# 5. Tracker

GitHub milestones mirror this ladder (`v0.6.1`, `v0.7`, `v0.8`, `v0.9`,
`v1.0`). Each past release has a closed epic issue linking its release
notes. Forward work is ticketed against these milestones; deferred items
stay in this file, not the tracker.

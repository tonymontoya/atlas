# Atlas v0.1.0 Scope
**Version:** 0.2 (Draft)  
**Status:** v0.1.0 Scope Document — realigned 2026-08-19; this milestone was formerly called "MVP". See `dev-plans/roadmap.md` for the ladder to 0.1.0.  
**Audience:** Engineering, Architecture, Product, Contributors  
**Project:** Atlas

---

# 1. Purpose

This document defines the scope of Atlas 0.1.0 — the first production-usable, single-zone release, and the first release carrying stability commitments.

v0.1.0 should prove that Atlas can safely turn Ceph operational intent into governed, auditable work. It should not attempt to deliver the broader enterprise platform, which follows post-0.1.0.

---

# 2. v0.1.0 Thesis

Atlas 0.1.0 is a single-zone operations platform for one or more Ceph clusters in one operational boundary.

v0.1.0 must prove the core Atlas loop:

1. observe Ceph inventory and health
2. create or update a Case
3. evaluate RBAC and Policy
4. request Approval when required
5. execute a strongly typed Workflow through an Atlas Agent
6. record Audit Events and Timeline Events
7. verify the result

---

# 3. v0.1.0 In Scope

## Platform

- Single-zone Atlas deployment
- One Organization
- One active Zone
- One or more Datacenters
- One or more Ceph Clusters
- REST API v1
- Web UI consuming REST API v1
- PostgreSQL persistence

Rook-managed Ceph remains an equal first-class cluster type in the data
model, but its provider lands post-0.1.0 as the first follow-on slice (see
`dev-plans/roadmap.md`).

## Inventory

- Cluster registration
- Cluster type: bare-metal Ceph
- Host inventory
- Storage Device inventory
- OSD inventory
- MON and MGR visibility
- Pool visibility
- automatic synchronization from Ceph
- preservation of historical Storage Device to OSD relationships

## Operations

- manual Case creation
- automatic Case creation from Ceph health or OSD failure events
- Case assignment
- Case comments
- Case Timeline Events
- Case status lifecycle
- one initial Workflow: Replace OSD
- durable Workflow Instance and Job state
- human Task support for hardware replacement
- Approval support

## Security

- OIDC authentication
- Compatibility with any OIDC identity provider (for example, Okta)
- hierarchical RBAC across Organization, Zone, Datacenter, Cluster, and Host
- immutable Audit Events for privileged operations
- least-privilege Atlas Agent operations
- mutual TLS between Atlas and Atlas Agents

## Integrations

- Ceph (bare-metal): required
- OIDC: required
- Prometheus: required for health and alert context, including a live alert source

Deferred to post-0.1.0: Rook-managed Ceph, chat notifications, NetBox, the
external ticket tracker, and OpenSearch/Splunk (see `dev-plans/roadmap.md`).

---

# 4. v0.1.0 Out of Scope

- Global Control Plane
- multi-zone synchronization
- cross-zone workflows
- WAN conflict resolution
- global dashboards
- Rook-managed Ceph provider (first post-0.1.0 slice)
- chat notifications
- arbitrary shell execution
- SSH proxying
- generic remote execution
- workflow marketplace
- plugin SDK
- natural language search
- AI-assisted summaries
- mobile experience
- ServiceNow, Teams, PagerDuty, or email integrations
- full cluster upgrade automation
- capacity forecasting
- predictive hardware failure analysis

---

# 5. First Workflow: Replace OSD

Replace OSD is the v0.1.0 tracer-bullet workflow because it exercises the main Atlas concepts without requiring federation.

The workflow should include:

- Case creation from an OSD failure
- evidence collection
- prechecks for cluster health, replication safety, and capacity
- Approval gate when policy requires it
- human Task for hardware replacement
- Agent-executed OSD operation steps
- recovery monitoring
- verification
- Case closure
- Audit Events for every privileged action
- Timeline Events for user-facing progress

v0.1.0 may support dry-run execution before real mutation if needed to validate the safety model.

---

# 6. v0.1.0 Acceptance Criteria

Atlas 0.1.0 is complete when:

- an operator can log in through OIDC
- a Ceph cluster can be registered
- inventory sync creates Clusters, Hosts, Storage Devices, OSDs, MONs, MGRs, and Pools
- a real Prometheus alert for a failed OSD can create a Case automatically
- an operator can create and assign a Case manually
- RBAC prevents unauthorized users from executing privileged actions
- policy can require Approval before a Workflow continues
- the Replace OSD Workflow can run through durable Workflow Instance and Job states
- an Atlas Agent executes only typed, approved operations
- privileged operations create immutable Audit Events
- Case Timeline Events show user-facing progress
- the system can restart without losing Workflow progress

---

# 7. Post-v0.1.0 Candidates

These remain important but follow v0.1.0:

- Rook-managed Ceph provider
- chat notifications
- NetBox as a required inventory source
- OpenSearch/Splunk contextual log links
- An external ticket tracker as a required integration
- additional workflows: Drain Host, Restart Daemon, Create Pool, Expand Cluster, Cluster Upgrade
- scheduler and maintenance windows beyond the needs of Replace OSD
- richer policy language
- fleet-wide reporting
- federation and global dashboards

---

# 8. Implementation Guardrails

- Prefer supported Ceph APIs before CLI.
- Wrap CLI usage behind typed provider interfaces.
- Treat bare-metal Ceph and Rook-managed Ceph as equal first-class Cluster types.
- Keep environment-specific names, tools, repositories, channels, and workflows out of core product concepts.
- Keep business logic out of the UI.
- Treat Workflow state as durable.
- Treat Event records as durable.
- Do not add generic remote execution to Atlas Agents.
- Do not make AWX or Ansible a runtime dependency.
- Existing AWX jobs, Ansible playbooks, and runbooks may be used only as discovery and migration inputs.
- Keep v0.1.0 data models compatible with future federation, but do not implement federation early.

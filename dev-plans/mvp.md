# Atlas MVP Scope
**Version:** 0.1 (Draft)  
**Status:** Pre-development Scope Document  
**Audience:** Engineering, Architecture, Product, Contributors  
**Project:** Atlas

---

# 1. Purpose

This document defines the first implementation slice for Atlas.

The MVP should prove that Atlas can safely turn Ceph operational intent into governed, auditable work. It should not attempt to deliver the full V1 enterprise platform.

---

# 2. MVP Thesis

Atlas MVP is a single-zone operations platform for one or more Ceph clusters in one operational boundary.

The MVP must prove the core Atlas loop:

1. observe Ceph inventory and health
2. create or update a Case
3. evaluate RBAC and Policy
4. request Approval when required
5. execute a strongly typed Workflow through an Atlas Agent
6. record Audit Events and Timeline Events
7. verify the result

---

# 3. MVP In Scope

## Platform

- Single-zone Atlas deployment
- One Organization
- One active Zone
- One or more Datacenters
- One or more Ceph Clusters
- REST API v1
- Web UI consuming REST API v1
- PostgreSQL persistence

## Inventory

- Cluster registration
- Cluster type: bare-metal Ceph
- Cluster type: Rook-managed Ceph
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

- Ceph: required
- OIDC: required
- Prometheus: required for health and alert context
- Chat notifications (for example, Slack): required for notifications only
- NetBox: optional read-only sync during MVP
- External ticket tracker (for example, Jira): optional external work request creation
- OpenSearch/Splunk: deferred to V1 unless needed for a pilot

---

# 4. MVP Out of Scope

- Global Control Plane
- multi-zone synchronization
- cross-zone workflows
- WAN conflict resolution
- global dashboards
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

Replace OSD is the MVP tracer-bullet workflow because it exercises the main Atlas concepts without requiring federation.

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

The MVP may support dry-run execution before real mutation if needed to validate the safety model.

---

# 6. MVP Acceptance Criteria

MVP is complete when:

- an operator can log in through OIDC
- a Ceph cluster can be registered
- inventory sync creates Clusters, Hosts, Storage Devices, OSDs, MONs, MGRs, and Pools
- a failed OSD can create a Case automatically
- an operator can create and assign a Case manually
- RBAC prevents unauthorized users from executing privileged actions
- policy can require Approval before a Workflow continues
- the Replace OSD Workflow can run through durable Workflow Instance and Job states
- an Atlas Agent executes only typed, approved operations
- privileged operations create immutable Audit Events
- Case Timeline Events show user-facing progress
- Chat notifications are sent for Case and Approval changes
- the system can restart without losing Workflow progress

---

# 7. Non-MVP V1 Candidates

These remain important for V1 but should follow the MVP:

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
- Keep MVP data models compatible with future federation, but do not implement federation early.

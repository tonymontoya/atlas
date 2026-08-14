# Atlas High-Level Architecture (HLD)
**Version:** 0.1 (Draft)
**Status:** High-Level Design
**Audience:** Software Architects, Senior Engineers, Contributors
**Project:** Atlas

---

# 1. Purpose

This document defines the high-level architecture for Atlas.

It intentionally does **not** describe implementation details.

Instead it defines:

- System boundaries
- Major components
- Trust boundaries
- Deployment topology
- Data ownership
- Scalability goals
- Failure domains
- Integration points
- Engineering principles

This document is considered the authoritative architecture reference for Atlas.

---

# 2. Architectural Goals

Atlas shall:

- Operate Ceph fleets at enterprise scale
- Minimize operational risk
- Eliminate routine SSH usage
- Provide enterprise RBAC
- Operate multiple independent Ceph clusters
- Continue functioning during WAN outages
- Integrate with enterprise tooling
- Remain upstream-friendly to Ceph
- Be horizontally scalable
- Be contributor-friendly

---

# 3. Architectural Principles

## 3.1 Ceph First

Atlas does not replace Ceph.

Atlas orchestrates Ceph.

---

## 3.2 API First

Every Atlas capability must be available through an API.

The Web UI consumes the same APIs.

---

## 3.3 Federated

Atlas is deployed per Zone.

A global control plane coordinates Zones.

Zones continue functioning independently if isolated.

---

## 3.4 Event Driven

Everything meaningful produces an event.

Examples:

- Cluster degraded
- OSD failed
- Workflow started
- Case opened
- Approval granted

---

## 3.5 Case Driven

Operators manage Cases.

Not alerts.

---

## 3.6 Policy Controlled

Every action passes through policy evaluation.

---

# 4. System Context

```
                     +----------------------+
                     |      Operators       |
                     +----------+-----------+
                                |
                     Web UI / CLI / API
                                |
                     +----------v-----------+
                     |  Global Atlas Plane  |
                     +----------+-----------+
                                |
              ----------------------------------------
              |                  |                   |
        Zone Atlas         Zone Atlas         Zone Atlas
              |                  |                   |
        Ceph Clusters      Ceph Clusters      Ceph Clusters
              |                  |                   |
        Atlas Agents       Atlas Agents       Atlas Agents
```

---

# 5. Deployment Model

Atlas consists of three logical tiers.

The MVP implements only a single Zone Atlas deployment plus Atlas Agents. The Global Control Plane is part of the long-term architecture and is not required for MVP.

## Global Control Plane

Responsibilities:

- Identity federation
- Global RBAC
- Fleet inventory
- Cross-zone reporting
- Cross-zone workflows
- Global audit
- Policy distribution
- Global dashboards

The Global Control Plane does **not** directly execute Ceph operations.

---

## Regional (Zone) Atlas

Each Zone contains an independent Atlas deployment.

Responsibilities:

- Local operations
- Local workflows
- Local cases
- Local event processing
- Agent communication
- Local cache
- Regional API

Zones continue operating if disconnected.

MVP constraint:

- one active Zone
- no cross-zone workflows
- no global dashboards
- no WAN synchronization conflict resolution
- data models must remain compatible with future federation

---

## Atlas Agents

Atlas Agents execute privileged operations.

Agents reside close to Ceph clusters.

Agents communicate outbound to the Zone Atlas.

Operators never communicate directly with Agents.

Atlas Agents are the Atlas execution boundary.

Atlas does not require AWX, Ansible, or any external workflow runner to execute core operations. Existing AWX jobs, Ansible playbooks, and runbooks may inform operation design, but they are not part of the required runtime architecture.

---

# 6. Component Overview

Atlas is a modular monolith composed of well-defined internal modules.

```
Atlas

├── API Gateway
├── Authentication
├── RBAC
├── Policy Engine
├── Case Manager
├── Workflow Engine
├── Event Engine
├── Inventory Service
├── Fleet Service
├── Scheduler
├── Notification Service
├── Audit Service
├── Integration Framework
├── Ceph Provider
├── Search
└── UI
```

Each module owns its own domain model.

---

# 7. Authentication

Supported Day One

- Generic OIDC
- Okta (example OIDC-compatible provider)

Future

- LDAP
- SAML

Atlas never stores passwords.

Authentication is delegated.

---

# 8. Authorization

RBAC is hierarchical.

```
Organization

    ↓

Zone

    ↓

Datacenter

    ↓

Cluster

    ↓

Host
```

Permissions inherit downward.

---

# 9. Trust Boundaries

```
User

↓

Atlas UI

↓

Authentication

↓

Authorization

↓

Policy

↓

Workflow

↓

Atlas Agent

↓

Ceph
```

No user ever communicates directly with Agents.

---

# 10. Atlas Agent

Purpose:

Safely execute privileged operations.

Responsibilities:

- Execute approved operations
- Collect inventory
- Gather diagnostics
- Query Ceph
- Stream events
- Report status

Non-goals:

- Shell access
- SSH proxy
- Generic remote execution

---

# 11. Agent Security

Every Agent possesses:

- Unique identity
- Mutual TLS certificate
- Signed registration
- Rotatable credentials

Every request includes:

- User identity
- Workflow
- Approval context
- Authorization token

Agents verify all requests.

Agents execute only strongly typed operations.

Examples:

- RestartDaemon
- CreateOSD
- RemoveOSD
- CollectDiagnostics

Agents do not expose:

- arbitrary shell execution
- SSH proxying
- generic remote execution
- mandatory AWX-backed or Ansible-backed execution paths

Every Agent operation must include:

- operation type
- typed inputs
- target scope
- user identity
- Workflow Instance
- Approval context where required
- idempotency key where practical
- audit correlation identifier

---

# 12. Ceph Integration

Preferred order:

## Primary

Supported Ceph APIs

Examples:

- Dashboard API
- Manager APIs

---

## Secondary

Supported CLI

Only when APIs are unavailable.

---

## Future

Optional Atlas mgr module.

Not required.

---

# 13. Inventory Model

Infrastructure

```
Organization

↓

Zone

↓

Datacenter

↓

Cluster

↓

Host

↓

Device

↓

OSD
```

Atlas metadata extends Ceph metadata.

---

# 14. Event Engine

Everything produces events.

Operational events are durable records before they trigger automation. A message bus may distribute events, but the authoritative event history must survive broker restarts and support replay or reconciliation.

Examples

Infrastructure

- Host discovered
- Device removed

Ceph

- OSD down
- Pool full
- Recovery started

Workflow

- Started
- Waiting
- Completed

Cases

- Created
- Assigned
- Closed

Audit

- Login
- Approval
- Policy violation

---

# 15. Case Management

Cases are long-lived operational records.

Lifecycle

```
Detected

↓

Triaged

↓

Planned

↓

Waiting

↓

Executing

↓

Verification

↓

Resolved

↓

Closed
```

Cases own:

- Workflows
- Timeline
- Evidence
- External ticket links
- Comments
- Attachments

---

# 16. Workflow Engine

Workflows execute operational procedures.

Workflow Instances and Jobs are durable state machines. They must resume after process restarts and must not rely on in-memory execution state.

State Machine

```
Created

↓

Prechecks

↓

Policy Evaluation

↓

Approval

↓

Scheduled

↓

Executing

↓

Verification

↓

Completed
```

Failures transition into recovery states.

Each Job has:

- stable identity
- typed operation
- target
- inputs
- current state
- retry policy
- terminal result
- audit correlation

---

# 17. Policy Engine

Policy evaluates:

- RBAC
- Maintenance windows
- Cluster health
- Safety rules
- Scheduling
- Approvals

Policy determines:

Allow

Reject

Require Approval

Delay

Escalate

---

# 18. Scheduler

Scheduler manages:

- Future execution
- Maintenance windows
- Retry logic
- Recurring jobs
- Deferred workflows

---

# 19. Fleet Service

Maintains:

- Organizations
- Zones
- Datacenters
- Clusters

Supports:

- Rollup health
- Capacity summaries
- Fleet search
- Fleet reporting

---

# 20. Inventory Service

Integrates with:

- Ceph
- NetBox

Maintains:

- Hosts
- Devices
- OSD mappings

---

# 21. Notification Service

Day One

- Chat notifications (for example, Slack)

Future

- Email
- Teams
- PagerDuty

Notifications originate from Cases.

Not raw alerts.

---

# 22. Search

Unified search across:

- Clusters
- Hosts
- Devices
- OSDs
- Cases
- Workflows
- Policies

Future:

Natural language search.

---

# 23. Audit Service

Every operation records:

- Timestamp
- User
- Approval
- Workflow
- Target
- Result
- Source IP
- Zone

Audit is append-only.

---

# 24. Integration Framework

Built-in integrations

- Ceph
- OIDC-compatible identity providers (for example, Okta)
- Prometheus
- NetBox
- Chat notifications (for example, Slack)
- OpenSearch
- Splunk

Optional adapters

- Jira
- ServiceNow
- Teams

---

# 25. Persistence

Atlas owns:

- Cases
- Workflows
- Policies
- RBAC
- Audit
- Scheduling
- Metadata

Atlas does **not** own:

- Ceph configuration
- Metrics
- Logs
- Inventory source of truth

---

# 26. External Systems

| Domain | System |
|---------|--------|
| Storage | Ceph |
| Identity | OIDC-compatible IdP (for example, Okta) |
| Metrics | Prometheus |
| Logs | OpenSearch / Splunk |
| Inventory | NetBox |
| Collaboration | Chat (for example, Slack) |
| ITSM | External tracker (for example, Jira; optional) |

---

# 27. Failure Model

## Zone Isolation

Regional Atlas continues functioning.

Synchronization resumes later.

---

## Global Control Plane Failure

Regional Atlas remains fully operational.

Global dashboards unavailable.

No interruption to local operations.

---

## Ceph Cluster Failure

Cases generated.

Workflows paused where appropriate.

Notifications issued.

---

## Agent Failure

Agent marked unavailable.

No operations executed.

Health warnings generated.

---

# 28. Scalability Targets

Initial design goals are organized into tiers in `dev-plans/scale_tiers.md`.

The largest targets remain long-term enterprise federation goals, not MVP acceptance criteria.

Enterprise federation targets:

| Metric | Target |
|---------|---------|
| Zones | 50+ |
| Datacenters | 500+ |
| Clusters | 500+ |
| Hosts | 100,000+ |
| Devices | 2,000,000+ |
| OSDs | 1,000,000+ |
| Concurrent Users | 5,000 |
| Concurrent Workflows | 50,000 |
| Cases | Unlimited practical retention |

The architecture should scale by adding additional regional Atlas deployments rather than vertically scaling a single instance.

---

# 29. Technology Recommendations

| Component | Recommendation |
|----------|----------------|
| Backend | Go |
| UI | React + TypeScript |
| API | REST (v1), GraphQL evaluation later |
| Database | PostgreSQL |
| Cache | Redis |
| Messaging | NATS |
| Search | PostgreSQL FTS initially |
| Authentication | OIDC |
| Deployment | Kubernetes or Podman |
| Packaging | OCI Containers |

These are recommendations and may evolve.

---

# 30. Engineering Principles

- Keep modules loosely coupled.
- Favor composition over inheritance.
- Every public API is versioned.
- All operations are idempotent where practical.
- Every workflow is resumable.
- Every action is auditable.
- Every module is independently testable.
- No hidden state.
- No business logic in the UI.
- Minimize Ceph-version-specific assumptions.

---

# 31. Future Architecture

Potential future enhancements:

- Optional Atlas ceph-mgr module
- AI-assisted operational analysis
- Predictive hardware failure modeling
- Workflow marketplace
- Plugin SDK
- Read-only disaster recovery control plane
- Multi-organization tenancy
- Public SDKs (Go, Python)

These are intentionally excluded from the initial implementation.

---

# 32. Summary

Atlas is architected as a federated, event-driven, case-centric operations platform for Ceph.

It complements the Ceph ecosystem by providing enterprise operational capabilities while respecting Ceph as the authoritative storage platform.

The architecture emphasizes:

- Safety over convenience
- Policy over ad hoc execution
- Cases over alerts
- Workflows over manual procedures
- Federation over centralized dependency
- Integration over replacement
- Operational excellence over feature breadth

This architecture is intended to support organizations operating from a single laboratory cluster to globally distributed, multi-petabyte Ceph fleets while remaining approachable for the open-source community.

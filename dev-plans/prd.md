# Atlas Product Requirements Document (PRD)
**Version:** 0.1 (Draft)  
**Status:** Product Requirements Document  
**Audience:** Engineering, Architecture, UX, Contributors  
**Project:** Atlas

---

# 1. Purpose

This document defines the functional and non-functional requirements for Atlas Version 1.

The purpose of this document is to define **what** Atlas must do.

Implementation details belong in architecture documents.

The first implementation slice is defined separately in `dev-plans/mvp.md`. MVP is intentionally narrower than the full Version 1 vision and starts with a single-zone deployment.

---

# 1.1 MVP Relationship

Atlas MVP shall prove the core operational loop before implementing the full enterprise platform:

- observe Ceph
- create or update Cases
- evaluate RBAC and Policy
- request Approval when required
- execute a strongly typed Workflow through an Atlas Agent
- record Audit Events and Timeline Events
- verify the result

Federation, global dashboards, cross-zone workflows, and mandatory enterprise integrations beyond the MVP set are deferred until after the single-zone operating model is proven.

---

# 2. Vision

Atlas is an open-source operations platform for Ceph that enables organizations to safely operate fleets of Ceph clusters through policy-driven automation, enterprise RBAC, operational workflows, case management, and multi-cluster visibility.

---

# 3. Problem Statement

Large enterprise Ceph deployments suffer from several operational challenges:

- Operations require extensive CLI knowledge.
- Engineers often require SSH and elevated privileges.
- Multi-cluster operations are manual.
- Operational runbooks exist as documentation instead of executable workflows.
- Enterprise RBAC is limited.
- Operational history is fragmented.
- Integrations between Ceph and enterprise tooling are largely custom-built.
- Storage teams spend excessive time coordinating routine maintenance.

Atlas addresses these challenges while remaining complementary to Ceph.

---

# 4. Product Goals

Atlas Version 1 shall:

- Provide a secure operational interface for Ceph.
- Eliminate routine SSH usage.
- Support enterprise RBAC.
- Manage fleets of Ceph clusters.
- Execute repeatable operational workflows.
- Track operational work through Cases.
- Integrate with enterprise identity and observability systems.
- Provide complete auditability.

---

# 5. Success Criteria

Atlas V1 is considered successful when organizations can:

- Perform routine Ceph administration without SSH.
- Execute common maintenance using standardized workflows.
- Delegate operations using RBAC instead of shared credentials.
- Track operational work using Cases.
- Operate multiple clusters from a unified interface.
- Integrate Atlas into existing enterprise ecosystems.

---

# 6. Personas

## Storage Operator

Responsibilities:

- Respond to alerts
- Replace failed OSDs
- Monitor recovery
- Execute maintenance

Primary Goal:

Operate clusters safely.

---

## Storage Architect

Responsibilities:

- Capacity planning
- Cluster expansion
- CRUSH design
- Pool configuration

Primary Goal:

Maintain platform health.

---

## Storage Administrator

Responsibilities:

- User administration
- RBAC
- Policies
- Fleet configuration

Primary Goal:

Govern Atlas.

---

## Platform Engineer

Responsibilities:

- Integrations
- APIs
- Automation
- Enterprise tooling

Primary Goal:

Connect Atlas to the enterprise ecosystem.

---

## Auditor

Responsibilities:

- Compliance
- Reporting
- Review

Primary Goal:

Understand what happened.

---

# 7. Product Scope

## In Scope

- Multi-cluster management
- Fleet inventory
- Cases
- Workflows
- RBAC
- Authentication
- Audit
- Policies
- Chat notification integration
- NetBox integration
- Prometheus integration
- OpenSearch/Splunk integration
- External ticket tracker integration
- Ceph operations

---

## Out of Scope

- Linux provisioning
- Operating system management
- BIOS management
- BMC management
- Firmware updates
- General configuration management
- Replacing Grafana
- Replacing Prometheus
- Replacing Ceph Dashboard

---

# 8. Functional Requirements

---

# FR-100 Fleet Management

Atlas shall support management of multiple Ceph clusters.

Atlas shall organize clusters by:

- Organization
- Zone
- Datacenter

Atlas shall provide roll-up health.

---

# FR-110 Cluster Inventory

Atlas shall maintain inventory of:

- Clusters
- Hosts
- Devices
- OSDs
- MONs
- MGRs
- MDSs
- Pools
- Filesystems

Inventory shall synchronize automatically.

---

# FR-120 Device Mapping

Atlas shall distinguish:

Storage Device

and

OSD.

The system shall preserve historical relationships.

---

# FR-200 Cases

Atlas shall provide a built-in Case Management system.

Cases represent operational work.

Cases shall support:

- Assignment
- Comments
- Timeline
- Attachments
- Evidence
- Status
- Related workflows

---

# FR-210 Case Creation

Cases may be created from:

- Operators
- Ceph events
- Prometheus alerts
- API requests
- Scheduled maintenance
- External integrations

---

# FR-220 Case Lifecycle

Supported states:

- Detected
- Triaged
- Planned
- Waiting
- Executing
- Verification
- Resolved
- Closed

---

# FR-300 Workflows

Atlas shall provide reusable workflow templates.

Examples:

- Replace OSD
- Drain Host
- Upgrade Cluster
- Create Pool
- Expand Cluster

---

# FR-310 Workflow Engine

Workflow engine shall support:

- Preconditions
- Variables
- Retries
- Approval gates
- Wait states
- Scheduling
- Verification
- Rollback hooks

---

# FR-320 Long Running Workflows

Workflows shall support waits lasting:

Minutes

Hours

Days

Weeks

---

# FR-330 Human Tasks

Workflow execution may pause while awaiting:

Hardware replacement

Approval

External ticket completion

Operator confirmation

---

# FR-400 Policies

Atlas shall support policy evaluation.

Policies include:

- Approval requirements
- Maintenance windows
- Safety checks
- Scheduling

---

# FR-410 Safety Validation

Atlas shall validate operations before execution.

Examples:

- Quorum safety
- Replication safety
- Cluster health
- Capacity

Unsafe operations shall fail safely.

---

# FR-500 Authentication

Atlas shall support:

- OIDC
- Any OIDC-compatible identity provider (for example, Okta)

Authentication is delegated.

---

# FR-510 Authorization

Atlas shall support hierarchical RBAC.

Scopes:

Organization

Zone

Datacenter

Cluster

Host

---

# FR-520 Audit

Every privileged operation shall generate immutable audit records.

---

# FR-600 Notifications

Atlas shall integrate with a chat notification provider.

The chat notification integration (for example, Slack) supports:

- Notifications
- Workflow approvals
- Case updates
- Interactive commands (future)

---

# FR-700 Ceph Integration

Atlas shall use:

1. Supported APIs
2. Supported CLI

Future mgr module optional.

---

# FR-710 Prometheus

Atlas shall consume:

- Metrics
- Alert context

Atlas shall not replace Prometheus.

---

# FR-720 Logs

Atlas shall link to:

- OpenSearch
- Splunk

Atlas shall not become a log platform.

---

# FR-730 NetBox

Atlas shall synchronize:

- Host inventory
- Datacenter metadata
- Rack information (where available)

---

# FR-740 External Ticket Tracker

Atlas shall support optional synchronization.

Atlas remains the operational system of record.

An external ticket tracker (for example, Jira) supports:

- External coordination
- Hardware requests
- Maintenance tickets

---

# FR-800 Fleet Dashboard

The landing page shall answer:

- What requires attention?
- What work is assigned to me?
- What approvals are waiting?
- What maintenance is scheduled?

---

# FR-900 Search

Unified search across:

- Cases
- Clusters
- Devices
- Hosts
- OSDs
- Policies
- Workflows

---

# 9. Non-Functional Requirements

---

## Security

- Mutual TLS
- Immutable audit
- No shared credentials
- No SSH requirement
- Least privilege

---

## Availability

Regional Atlas continues functioning during WAN outages.

---

## Scalability

Target support:

- Hundreds of clusters
- Hundreds of thousands of hosts
- Millions of OSDs

---

## Performance

Dashboard load:

< 3 seconds

API latency:

< 250 ms P95

Search:

< 2 seconds

---

## Reliability

Every workflow is resumable.

---

## Testability

Every module shall support automated testing.

---

# 10. User Journeys

## Failed Drive

1. OSD failure detected
2. Case created
3. Evidence collected
4. External work request created
5. Hardware replaced
6. Workflow resumes
7. OSD recreated
8. Recovery monitored
9. Case closed

---

## Cluster Upgrade

1. Upgrade Case
2. Compatibility checks
3. Approval
4. Schedule
5. Execute
6. Verify
7. Close

---

## Capacity Expansion

1. Capacity threshold exceeded
2. Atlas forecasts exhaustion
3. Planning Case created
4. Hardware ordered
5. Cluster expanded
6. Verification
7. Close

---

# 11. Integrations

## MVP Mandatory

- Ceph
- OIDC
- Prometheus
- Chat notifications (for example, Slack)

Identity-provider support is delivered through OIDC compatibility; Okta is one compatible example.

## MVP Optional

- NetBox
- External ticket tracker (for example, Jira)

## V1 Candidates After MVP

- NetBox as required inventory synchronization
- OpenSearch/Splunk contextual log links
- External ticket tracker work request creation (for example, Jira)
- additional notification and ITSM providers

---

# 12. Acceptance Criteria

MVP is complete when:

✓ Operator logs in using OIDC

✓ Fleet inventory synchronizes automatically

✓ RBAC controls access

✓ Cases can be created manually and automatically

✓ Workflows execute through Atlas Agents

✓ Every operation is audited

✓ Chat notifications function

✓ Prometheus metrics are visible

✓ Regional Atlas functions as a single-zone deployment

✓ Workflows resume after process restart

✓ Atlas Agents execute only typed, approved operations

Post-MVP V1 is complete when:

✓ NetBox inventory synchronization works when enabled

✓ OpenSearch/Splunk integration provides contextual log access when enabled

✓ The external ticket tracker integration creates external work requests when enabled

✓ Regional Atlas deployments continue functioning during WAN isolation

---

# 13. Risks

- Ceph API compatibility across releases
- Federated synchronization complexity
- Long-running workflow reliability
- Privileged agent security
- Large-scale inventory synchronization
- Enterprise identity edge cases

---

# 14. Assumptions

- Ceph remains the storage system of record.
- NetBox remains the infrastructure inventory source.
- Prometheus remains the metrics source.
- OpenSearch/Splunk remain log sources.
- An OIDC-compatible identity provider provides authentication.
- Atlas is the operational control plane.

---

# 15. Version 1 Deliverables

## Platform

- Single-zone regional operation
- Agent framework
- REST API
- Web UI

Post-MVP:

- Federated architecture
- Global Control Plane
- cross-zone synchronization

---

## Security

- OIDC
- Any OIDC-compatible identity provider (for example, Okta)
- RBAC
- Audit

---

## Core

- Fleet
- Cases
- Workflows
- Policies
- Scheduler

---

## Integrations

- Ceph
- Prometheus
- Chat notifications (for example, Slack)

Post-MVP:

- NetBox
- OpenSearch/Splunk
- External ticket tracker (for example, Jira)

---

## Operations

Initial workflow library:

- Replace OSD
- Drain Host
- Restart Daemon
- Create Pool
- Expand Cluster
- Cluster Upgrade
- Host Maintenance
- Cluster Health Investigation

---

# 16. Post-V1 Roadmap

Potential future capabilities:

- Optional ceph-mgr module
- AI-assisted operational summaries
- Predictive hardware failure analysis
- Workflow SDK
- Plugin marketplace
- Mobile experience
- Advanced reporting
- Cross-cluster maintenance planning
- Advanced chat bot interactions
- Public automation SDKs

These items are intentionally excluded from the Version 1 commitment.

---

# 17. Definition of Done

A feature is complete only when it includes:

- Functional implementation
- Unit tests
- Integration tests
- API documentation
- User documentation
- Audit coverage
- RBAC enforcement
- Observability (metrics and logs)
- Security review
- UX review

---

# 18. Guiding Principle

Atlas should always optimize for **operational excellence** over feature count.

Every feature should answer one or more of these questions:

- Does it make Ceph safer to operate?
- Does it reduce manual effort?
- Does it improve operational consistency?
- Does it reduce time to resolution?
- Does it improve visibility?
- Does it simplify enterprise adoption?

If the answer to all of these questions is **no**, the feature should not be included in Atlas.

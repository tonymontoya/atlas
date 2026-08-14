# Atlas Product Vision & Principles
**Version:** 0.1 (Draft)
**Status:** Foundational Design Document
**Audience:** Engineering, Architecture, Product, Contributors
**Project:** Atlas
**License:** Apache 2.0 (Planned)

---

# 1. Vision

Atlas is an open-source operations platform for Ceph that enables organizations to safely operate large-scale storage environments through policy-driven automation, multi-cluster fleet management, enterprise-grade RBAC, operational workflows, and intelligent case management.

Atlas complements Ceph by focusing on **how organizations operate Ceph**, rather than how Ceph itself stores data.

---

# 2. Mission

Provide the best operational experience for managing Ceph at enterprise scale while remaining a good citizen of the upstream Ceph ecosystem.

Atlas should reduce operational complexity, improve safety, minimize manual effort, and allow storage teams to confidently operate fleets containing thousands of servers and tens of thousands of OSDs.

---

# 3. Goals

Atlas exists to solve problems that Ceph intentionally does not.

Specifically:

- Enterprise RBAC
- Fleet management
- Multi-cluster operations
- Operational automation
- Case management
- Maintenance planning
- Policy enforcement
- Approval workflows
- Operational history
- Capacity intelligence
- Enterprise integrations

---

# 4. Non-Goals

Atlas is **not** intended to replace:

- Ceph Dashboard
- ceph-mgr
- MON
- MGR
- OSD
- MDS
- RADOS Gateway
- Prometheus
- Grafana
- OpenSearch
- Splunk
- Linux configuration management
- Ansible
- AWX
- Puppet
- Chef
- Terraform

Atlas operates Ceph.

It does not become a general infrastructure management platform.

---

# 5. Product Philosophy

Atlas is built around several core philosophies.

## 5.1 Operations Platform

Atlas is not a dashboard.

Dashboards display information.

Atlas enables operators to safely perform work.

---

## 5.2 Case Driven

Alerts identify problems.

Cases represent operational work.

Atlas is centered around operational cases rather than alert streams.

---

## 5.3 Event Driven

Atlas continuously observes Ceph environments.

Events generate operational awareness.

Policies determine whether Atlas:

- recommends an action
- prepares a workflow
- requests approval
- performs automation

---

## 5.4 Human-Centered Operations

Operators should not memorize Ceph commands.

Instead they perform operations such as:

- Replace Failed OSD
- Expand Cluster
- Drain Host
- Upgrade Cluster
- Create Pool

Atlas translates these into safe execution plans.

---

## 5.5 Safety First

Every operation should be validated before execution.

Atlas should prevent operators from accidentally damaging production storage.

Whenever possible:

Unsafe operations are refused rather than merely warned about.

Break-glass workflows may override safeguards when explicitly authorized.

---

## 5.6 Attention Driven UX

Users should immediately understand:

- What needs attention
- What work is assigned
- What approvals are pending
- What maintenance is scheduled

Navigation should never be the primary interaction model.

---

## 5.7 Enterprise Friendly

Atlas should integrate with existing enterprise systems instead of replacing them.

Examples include:

- Okta
- Prometheus
- NetBox
- OpenSearch
- Splunk
- Slack
- Jira

---

## 5.8 Upstream Friendly

Atlas should consume supported Ceph interfaces whenever possible.

Atlas should avoid depending on unsupported internal Ceph implementation details.

Future upstream contributions should remain possible.

---

# 6. Scope

Atlas manages the lifecycle of Ceph.

Atlas does not manage the lifecycle of Linux.

---

## Atlas Owns

- Cluster operations
- Fleet operations
- Pool management
- OSD lifecycle
- MON lifecycle
- MGR lifecycle
- MDS lifecycle
- RGW lifecycle
- CRUSH management
- Ceph upgrades
- Ceph configuration
- Operational workflows
- Cases
- Maintenance
- Audit
- RBAC
- Capacity planning

---

## Atlas Does Not Own

- Operating system installation
- Network configuration
- DNS
- NTP
- BIOS
- BMC
- Firmware
- Linux users
- General package management
- Configuration management

These systems may be integrated into Atlas workflows.

---

# 7. Core Principles

## Principle 1

No operator should require SSH access or root privileges to perform routine Ceph operations.

---

## Principle 2

All privileged operations occur through authenticated Atlas Agents.

---

## Principle 3

Every action is attributable.

There are no anonymous operations.

---

## Principle 4

Every privileged action is auditable.

---

## Principle 5

Every destructive operation should have safety validation.

---

## Principle 6

Atlas should expose operational intent instead of implementation details.

Users should click:

Replace Failed OSD

instead of:

ceph osd out

---

## Principle 7

Automation should be deterministic and repeatable.

---

## Principle 8

Humans approve.

Software executes.

---

# 8. Security Principles

Security is a first-class design objective.

Atlas exists partly to eliminate widespread SSH and root access requirements.

## Authentication

MVP:

- OIDC

Identity-provider support is delivered through OIDC compatibility; Okta is one compatible example.

Post-MVP:

- additional generic OAuth2 provider hardening
- LDAP
- SAML

Additional providers may be added later.

---

## Authorization

Atlas RBAC determines:

Who may:

- View
- Approve
- Execute
- Schedule
- Administer

Permissions are scoped to organizational hierarchy.

---

## Least Privilege

Atlas Agents execute only approved operations.

Agents should never expose arbitrary shell access.

Operations should be strongly typed.

Examples:

- RestartDaemon
- CreateOSD
- RemoveOSD
- UpgradeCluster

NOT

ExecuteShell()

---

## Audit

Every action records:

- User
- Time
- Source
- Reason
- Workflow
- Approval chain
- Results

Audit history is immutable.

---

# 9. Architectural Principles

## Federated

Atlas is a federated platform.

Each Zone contains a regional Atlas deployment.

A global control plane coordinates fleet-wide operations.

---

## Regional Autonomy

Loss of WAN connectivity must not prevent local cluster operations.

Regional deployments continue functioning independently.

State synchronizes after connectivity returns.

---

## Modular Monolith

Atlas is developed as a modular monolith.

Benefits:

- Simpler deployment
- Easier development
- Strong module boundaries
- Future extensibility

---

## API First

Everything Atlas performs should be available through public APIs.

The UI consumes the same APIs as external clients.

---

## Ceph API First

Preferred integration order:

1. Supported Ceph APIs
2. Supported Ceph CLI
3. Optional future Atlas ceph-mgr module

---

# 10. Organizational Model

Atlas understands organizational hierarchy.

```
Organization
    │
    └── Zone
          │
          └── Datacenter
                 │
                 └── Cluster
                        │
                        └── Host
                               │
                               ├── Storage Device
                               │
                               └── Ceph Daemon
```

RBAC inherits naturally throughout this hierarchy.

---

# 11. Core Objects

Atlas defines several first-class objects.

## Infrastructure

- Organization
- Zone
- Datacenter
- Cluster
- Host
- Device

## Ceph

- MON
- MGR
- OSD
- MDS
- Pool
- Filesystem
- CRUSH
- PG
- RBD
- RGW

## Operations

- Case
- Workflow
- Approval
- Maintenance Window
- Policy
- Job
- Audit Event

---

# 12. Cases

Cases are central to Atlas.

Cases represent operational work.

Examples:

- Failed Drive
- Cluster Upgrade
- Capacity Expansion
- Hardware Failure
- Cluster Investigation
- Planned Maintenance

Cases may exist for minutes or months.

Cases may include:

- Workflows
- External ticket links
- Evidence
- Timeline
- Comments
- Attachments
- Audit history

---

# 13. Workflows

Workflows execute operational procedures.

Examples:

- Replace OSD
- Drain Host
- Upgrade Cluster
- Expand Cluster
- Add OSD
- Create Pool

Every workflow supports:

- Preconditions
- Safety validation
- Approval
- Execution
- Verification
- Audit

---

# 14. Integration Priorities

Atlas MVP requires native support for:

## Identity

- OIDC

Identity-provider support is delivered through OIDC compatibility; Okta is one compatible example.

## Storage

- Ceph

## Observability

- Prometheus

## Communications

- Chat notifications (for example, Slack)

Post-MVP integrations include:

- NetBox
- External ticket tracking (for example, Jira)
- OpenSearch
- Splunk
- ServiceNow
- Teams
- Email
- PagerDuty

---

# 15. Relationship with External Systems

Atlas is the operational control plane.

External systems remain authoritative for their domains.

| Domain | System of Record |
|----------|------------------|
| Storage | Ceph |
| Metrics | Prometheus |
| Logs | OpenSearch / Splunk |
| Identity | OIDC-compatible IdP (for example, Okta) |
| Inventory | NetBox |
| External Tickets | External tracker (for example, Jira; optional) |
| Operations | Atlas |

---

# 16. Success Metrics

Atlas is successful when:

- Operators rarely require SSH access.
- Routine storage operations become repeatable workflows.
- Enterprise RBAC replaces shared administrative accounts.
- Fleet-wide operations can be safely executed from a single interface.
- Mean Time To Recovery (MTTR) decreases.
- Manual operational effort decreases.
- Operational consistency increases.
- Organizations trust Atlas as the primary operational interface for Ceph.

---

# 17. Guiding Principle

> Atlas is not another Ceph dashboard.

> Atlas is the operational control plane that allows organizations to safely run Ceph at enterprise scale.

Everything in Atlas should reinforce this vision.

If a feature does not improve operational safety, consistency, visibility, or efficiency, it does not belong in Atlas.

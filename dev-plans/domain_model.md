# Atlas Domain Model & Ubiquitous Language
**Version:** 0.1 (Draft)  
**Status:** Foundational Design Document  
**Audience:** Engineering, Architecture, Contributors  
**Project:** Atlas

---

# 1. Purpose

This document establishes the canonical language used throughout Atlas.

Every engineer, contributor, document, API, and UI should use these definitions consistently.

This document intentionally defines **concepts**, not implementation.

The purpose is to eliminate ambiguity.

---

# 2. Design Philosophy

Atlas has three categories of objects:

1. Infrastructure
2. Ceph
3. Operations

Infrastructure describes *where* Ceph exists.

Ceph describes *what* Ceph is.

Operations describe *how* people operate Ceph.

---

# 3. Domain Overview

```
Organization
    │
    ▼
Zone
    │
    ▼
Datacenter
    │
    ▼
Cluster
    │
    ▼
Host
    │
    ├──────────────┐
    ▼              ▼
Storage Device   Ceph Daemon
                     │
                     ▼
                Operational Case
                     │
                     ▼
                 Workflow(s)
                     │
                     ▼
                 Job(s)
```

---

# 4. Infrastructure Domain

## Organization

The highest administrative boundary.

An Organization owns:

- Zones
- Policies
- Users
- Roles

Examples

Example Organization

Acme Corporation

---

## Zone

A geographic or logical operating region.

Examples

US-West

US-East

Europe

Japan

A Zone contains:

- Datacenters
- Regional Atlas deployment
- Regional Cases
- Regional Policies

---

## Datacenter

A physical facility.

Contains:

- Clusters
- Hosts

Atlas stores operational metadata about Datacenters.

Ceph does not.

---

## Cluster

A single Ceph cluster.

Identified by:

- FSID
- Name

A Cluster contains:

- Hosts
- Daemons
- Pools
- OSDs
- CRUSH hierarchy
- Filesystems

A Cluster is the primary operational boundary.

---

## Host

A physical or virtual machine participating in a Ceph cluster.

Hosts own:

- Storage Devices
- Daemons

Hosts may participate in only one Cluster.

---

## Storage Device

A physical storage medium.

Examples

- NVMe
- SSD
- HDD

A Storage Device exists independently of an OSD.

This distinction is intentional.

Storage Devices have identity.

OSDs are software.

---

# 5. Ceph Domain

Atlas intentionally mirrors Ceph terminology.

---

## OSD

A Ceph Object Storage Daemon.

Attributes include:

- OSD ID
- Weight
- Status
- Device
- Host

An OSD is associated with exactly one Storage Device.

Over time:

One Storage Device may host multiple OSD identities.

---

## MON

Monitor daemon.

Responsible for cluster consensus.

---

## MGR

Manager daemon.

Provides operational APIs.

Primary Atlas integration point.

---

## MDS

Metadata Server.

Used for CephFS.

---

## RGW

RADOS Gateway.

Represents object storage services.

---

## Pool

Logical storage pool.

Contains PGs.

---

## PG

Placement Group.

Atlas treats PGs primarily as health indicators.

Atlas does not attempt to manage PG internals.

---

## Filesystem

CephFS instance.

---

## CRUSH Map

Cluster placement topology.

Atlas understands and visualizes it.

Atlas never replaces it.

---

## CRUSH Rule

Placement policy.

---

## Ceph Configuration

Represents cluster configuration.

Atlas may modify configuration.

Ceph remains authoritative.

---

# 6. Operations Domain

This is Atlas's primary domain.

---

## Case

The central operational object.

Definition:

A Case represents a piece of operational work from detection through closure.

Cases may last:

- Minutes
- Hours
- Days
- Weeks
- Months

Examples

Replace Drive

Upgrade Cluster

Capacity Expansion

Customer Investigation

Maintenance

Cases own:

- Workflow
- Timeline
- Evidence
- Audit
- Approvals
- External tickets

---

## Workflow

A reusable operational procedure.

Examples

Replace OSD

Upgrade Cluster

Create Pool

Drain Host

Every Workflow has:

- Inputs
- Preconditions
- Execution plan
- Verification
- Completion criteria

A Workflow is reusable.

A Case is not.

---

## Workflow Instance

An execution of a Workflow.

Example

Workflow Template

Replace OSD

↓

Workflow Instance

Replace OSD #347

Cluster Alpha

OSD 12

Started 15:43

---

## Job

A single executable step.

Examples

Mark OSD Out

Restart Daemon

Collect SMART

Update CRUSH

A Workflow contains many Jobs.

---

## Task

A unit of human work.

Examples

Replace Drive

Approve Maintenance

Verify Rack

Tasks may be assigned.

Tasks may exist without automation.

---

## Approval

Authorization to continue execution.

Approvals are explicit domain objects.

They include:

Approver

Reason

Timestamp

Decision

Comments

---

## Policy

A declarative operational rule.

Examples

No upgrades Friday night.

Require approval for CRUSH changes.

Never reduce replication below three.

Policies influence workflows.

Policies never directly execute work.

---

## Maintenance Window

A scheduled period during which specific operations may execute.

---

## Schedule

Defines future execution.

Used by:

Cases

Workflows

Maintenance

Automation

---

## Audit Event

Immutable record of an action.

Examples

Login

Approval

Policy Violation

Workflow Started

Workflow Completed

---

## Timeline Event

A chronological operational event.

Timeline Events are user-facing.

Audit Events are compliance records.

These are intentionally different concepts.

---

# 7. Identity Domain

## User

Authenticated human.

Authentication delegated to OIDC.

Atlas stores:

Identity

Preferences

Permissions

Assignments

---

## Role

Collection of permissions.

Examples

Viewer

Operator

Storage Engineer

Administrator

Auditor

---

## Permission

Smallest authorization unit.

Examples

ViewCluster

RestartDaemon

CreatePool

UpgradeCluster

ApproveWorkflow

---

## Scope

Defines where a Permission applies.

Examples

Organization

Zone

Datacenter

Cluster

Host

---

# 8. Automation Domain

## Event

Something that occurred.

Examples

OSD Down

Pool Full

Workflow Completed

Events are immutable.

---

## Trigger

Maps Events into Actions.

Examples

OSD Down

↓

Create Case

---

## Action

Automated response.

Examples

Create Jira

Send Slack

Open Case

Launch Workflow

---

## Rule

Connects:

Event

Trigger

Action

---

## Atlas Agent

A privileged Atlas component that executes strongly typed, approved operations close to Ceph clusters.

Atlas Agents are not:

- SSH proxies
- remote shells
- generic command runners

---

# 9. External Systems

Atlas integrates with external systems.

They remain authoritative.

---

## Identity Provider

Examples

Okta

Azure AD

OIDC

---

## Inventory Provider

NetBox

---

## Metrics Provider

Prometheus

---

## Log Provider

OpenSearch

Splunk

---

## Ticket Provider

Jira

ServiceNow

---

## Notification Provider

Slack

Teams

---

# 10. Relationships

```
Organization

contains

Zones

Zone

contains

Datacenters

Datacenter

contains

Clusters

Cluster

contains

Hosts

Host

contains

Storage Devices

Host

runs

Daemons

Storage Device

backs

OSD

Case

owns

Workflow Instances

Workflow Instance

contains

Jobs

Workflow Instance

generates

Audit Events

Workflow Instance

updates

Timeline Events

Policy

governs

Workflow Instance

Approval

authorizes

Workflow Instance
```

---

# 11. Lifecycle Models

## Case

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

---

## Workflow

```
Created

↓

Prechecks

↓

Policy

↓

Approval

↓

Execution

↓

Verification

↓

Completed
```

---

## Job

```
Pending

↓

Running

↓

Succeeded

or

Failed

or

Cancelled
```

---

# 12. Naming Conventions

Engineering should use:

Case

NOT

Incident

unless referring to an actual operational incident.

---

Workflow

NOT

Playbook

---

Job

NOT

Command

---

Device

NOT

Disk

unless referring to physical media.

---

Timeline Event

NOT

Audit Event

These are distinct.

---

Cluster

Always means

One Ceph FSID.

---

Host

Always means

One operating system instance.

---

# 13. Ubiquitous Language

The following words have precise meanings.

| Word | Meaning |
|--------|---------|
| Case | Long-lived operational work |
| Workflow | Reusable procedure |
| Workflow Instance | One execution |
| Job | Executable step |
| Task | Human work |
| Policy | Declarative rule |
| Approval | Authorization |
| Event | Something happened |
| Trigger | React to Event |
| Action | Automation |
| Timeline | Operational history |
| Audit | Compliance history |
| Device | Physical storage |
| OSD | Ceph daemon |
| Cluster | Single Ceph FSID |

These definitions are normative.

---

# 14. Guiding Principle

Every new feature should introduce as few new domain concepts as possible.

If a feature cannot be described using the existing ubiquitous language, engineers should first determine whether:

1. An existing domain object already models the concept.
2. The feature is introducing an unnecessary abstraction.
3. The domain model should be intentionally extended through an Architecture Decision Record (ADR).

A shared language is one of Atlas's most important architectural assets. Preserving its clarity is essential to keeping the codebase understandable as the project and contributor community grow.

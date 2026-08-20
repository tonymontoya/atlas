# Atlas

Atlas is an operations platform for Ceph. Its language distinguishes infrastructure, Ceph objects, and operational work so APIs, UI, docs, and code describe the same concepts consistently.

## Infrastructure

**Organization**:
The highest administrative boundary in Atlas.
_Avoid_: Tenant, company account

**Zone**:
A geographic or logical operating region containing a single regional Atlas deployment in the long-term architecture.
_Avoid_: Region, site group

**Datacenter**:
A physical facility that contains clusters and hosts.
_Avoid_: Site, facility

**Cluster**:
One Ceph cluster identified by a single Ceph FSID.
_Avoid_: Fleet, environment

**Host**:
One operating system instance participating in a Ceph cluster.
_Avoid_: Server, node, machine

**Storage Device**:
A physical storage medium that may back one or more historical OSD identities over time.
_Avoid_: Disk, drive, volume

## Ceph

**OSD**:
A Ceph Object Storage Daemon associated with exactly one Storage Device at a point in time.
_Avoid_: Disk, storage node

**Ceph Daemon**:
A Ceph service process such as MON, MGR, MDS, OSD, or RGW.
_Avoid_: Service, process

**Pool**:
A Ceph logical storage pool.
_Avoid_: Bucket, volume

## Operations

**Operator**:
A human user of Atlas who performs operational work, distinct from system and Atlas Agent actors.
_Avoid_: User account, admin

**Actor**:
The attribution identity behind an action: an Operator, the Atlas system, or the Atlas Agent.
_Avoid_: User account, principal

**Case**:
A long-lived record of operational work from detection through closure.
_Avoid_: Ticket, incident

**Assignment**:
The Operator currently responsible for a Case.
_Avoid_: Owner, ticket holder

**Case Note**:
A durable, addressable annotation on a Case, recorded as a Timeline Event.
_Avoid_: Comment, message

**Workflow**:
A reusable operational procedure.
_Avoid_: Playbook, runbook

**Workflow Instance**:
One execution of a Workflow for a specific Case or target.
_Avoid_: Run, execution

**Job**:
A single executable step inside a Workflow Instance.
_Avoid_: Command, script

**Task**:
A unit of human work that may be assigned and may exist without automation.
_Avoid_: Job, ticket

**Workflow Gate**:
A point in a Workflow Instance where execution pauses until an Approval or Operator action allows it to continue.
_Avoid_: Checkpoint, barrier

**Approval**:
An explicit authorization decision allowing a Workflow Instance to continue past a Workflow Gate.
_Avoid_: Sign-off

**Policy**:
A declarative operational rule that influences whether work is allowed, rejected, delayed, escalated, or requires approval.
_Avoid_: Script, automation

**Maintenance Window**:
A scheduled period during which specific operations may execute.
_Avoid_: Change window

**Audit Event**:
An immutable compliance and security record proving that an action occurred.
_Avoid_: Timeline entry, log line

**Timeline Event**:
A user-facing chronological event showing operational progress on a Case or Workflow Instance.
_Avoid_: Audit event

**Alert**:
A normalized condition report from an observability source such as Prometheus.
_Avoid_: Incident, alarm

**Alert Fingerprint**:
A stable identity for an alert condition (alert name plus non-context labels), used to deduplicate automatic Case creation.
_Avoid_: Alert ID, hash key

**Cluster Registration**:
The Operator act of defining a Cluster in Atlas and obtaining Enrollment instructions for its Atlas Agent.
_Avoid_: Cluster onboarding, connection setup

**Enrollment**:
The process by which an Atlas Agent establishes a durable, trusted identity with Atlas for a registered Cluster.
_Avoid_: Pairing, join, agent registration

**Atlas Agent**:
A privileged component in a Cluster's trust domain that collects cluster state and executes strongly typed, approved operations.
_Avoid_: SSH proxy, remote shell, worker node

# Atlas Environment Context
**Version:** 0.2 (Draft)  
**Status:** Descriptive Context Document  
**Audience:** Engineering, Architecture, Product, Contributors  
**Project:** Atlas  

---

# 1. Purpose

This document describes an example operating environment that Atlas is
designed to serve.

This document is descriptive, not normative. Its purpose is to reveal realistic
operational constraints and portability requirements. Atlas must remain a
general Ceph operations platform and must not hard-code any one environment's
names, tools, repositories, channels, or topology into core product behavior.

Decision reference: `docs/adr/0011-environment-context-is-descriptive.md`.

---

# 2. Example Environment Summary

The example environment includes:

- dozens of Ceph clusters
- tens of datacenters
- multiple global regions
- Ceph 16 still present during migration
- active upgrade work toward Ceph 18
- bare-metal Ceph clusters on conventional Linux hosts
- Rook-managed Ceph clusters
- block storage through RBD-oriented families
- object storage through RGW/S3-compatible families
- Prometheus, SLO, and alerting modernization work
- chat-based operational visibility
- hardware, Storage Device, host, and networking operational load
- existing AWX/Ansible automation that must be treated as migration/discovery input only

---

# 3. Portability Rules

Atlas must not assume:

- specific internal storage-family names
- a specific external ticket tracker project
- a specific chat channel
- AWX or Ansible availability
- a specific Git provider group or repository layout
- one Linux distribution as the only host operating model
- Rook as the only Kubernetes operating model
- one company's datacenter naming scheme
- one identity provider beyond OIDC compatibility

Atlas may support environment-specific configuration through adapters, labels, metadata, and deployment configuration.

---

# 4. Product Implications

## Cluster Types

Bare-metal Ceph and Rook-managed Ceph are equal first-class Cluster types.

Atlas should model both as Ceph clusters. Rook is a management and deployment path for Ceph, not a replacement for the Ceph domain model.

Atlas should expose consistent concepts across both:

- Cluster
- Host
- Storage Device
- OSD
- MON
- MGR
- Pool
- Case
- Workflow
- Audit Event
- Timeline Event

Implementation may differ behind provider interfaces.

## Ceph Versions

Ceph 18 is the primary MVP target.

Ceph 16 remains important as migration context. MVP support for Ceph 16 should focus on read-only inventory, health, and reporting unless specific mutating operations are explicitly validated.

## Operations

The example environment's operational pressure points suggest Atlas should prioritize:

- inventory and health synchronization
- OSD failure and replacement workflows
- upgrade readiness reporting
- alert-to-Case creation
- capacity and recovery context
- RGW/object-storage signal visibility
- hardware and host evidence collection
- maintenance state awareness

## Existing Automation

Existing AWX jobs, Ansible playbooks, scripts, and runbooks are useful for:

- workflow discovery
- operational-risk classification
- precheck design
- verification design
- migration planning

They are not required runtime dependencies.

---

# 5. Design Tension

A deployment should feel native to its operating environment without making Atlas proprietary to it.

The practical rule:

If a concept is true for Ceph operators generally, it belongs in the core model.

If a concept is true only for one organization, it belongs in configuration, metadata, an adapter, or documentation.

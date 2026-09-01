# Ceph 18 Target And Cluster Type Parity

Atlas MVP will target Ceph 18 as the primary supported Ceph version. Bare-metal Ceph clusters and Rook-managed Ceph clusters are equal first-class Cluster types, not primary and secondary modes.

**Consequences**

Provider interfaces must separate Ceph operation semantics from deployment mechanism. Bare-metal and Rook implementations may use different access paths, but the Atlas domain model, API, RBAC, audit, policy, Case, and Workflow behavior should remain consistent across both cluster types.

**Amendment, 2026-09-01 (version floor and breadth):** Ceph 18 remains the
primary target through v0.1.0, but the support floor is now Ceph 18 and
newer: the Ceph 16 read-only migration target is retired — its migration
context is obsolete and the `pacific-readonly` fixture is removed. Ceph 19
and 20 join as read-compatibility targets at v0.0.11 (Dashboard fixtures
plus read contract coverage) and reach mutation validation with the Rook
milestone (v0.3.0), where the primary version is re-pinned from fleet
reality. Cluster-type parity is unchanged.

# Ceph 18 Target And Cluster Type Parity

Atlas MVP will target Ceph 18 as the primary supported Ceph version. Bare-metal Ceph clusters and Rook-managed Ceph clusters are equal first-class Cluster types, not primary and secondary modes.

**Consequences**

Provider interfaces must separate Ceph operation semantics from deployment mechanism. Bare-metal and Rook implementations may use different access paths, but the Atlas domain model, API, RBAC, audit, policy, Case, and Workflow behavior should remain consistent across both cluster types.

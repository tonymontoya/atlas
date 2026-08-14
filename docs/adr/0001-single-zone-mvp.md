# Single-Zone MVP

Atlas will begin with a single-zone MVP instead of implementing the Global Control Plane and Regional Atlas federation from day one. This lets the first implementation prove the core operating loop: inventory, cases, RBAC, audit, policy, workflows, and agent execution inside one operational boundary. Federation remains an architectural goal, but MVP code should not require cross-zone synchronization to function.

**Consequences**

The MVP may model `Organization`, `Zone`, and `Datacenter`, but only one active Zone is required. APIs and persistence should avoid assumptions that prevent future federation, but global dashboards, cross-zone workflows, and WAN conflict resolution are not MVP requirements.

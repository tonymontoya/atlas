# Approval Records Bound to Workflow Instance Gates

An Approval is a durable record bound to a specific Workflow Instance and the gate it is paused at, satisfying the security review checklist's approval gate. The record captures approver identity as an OIDC subject with a display-name snapshot (the assignment pattern from manual Case writes), the Workflow Instance and gate, an optional reason, and a timestamp. One approval advances the instance past its current gate; it authorizes nothing else. Approval expiration is deferred until approvals guard real mutation.

Before RBAC exists, any authenticated Operator may approve, mirroring the v0.4.0 decision that all authenticated operators may perform manual writes. Approval authority becomes a permission when RBAC lands; the record shape does not change.

**Consequences**

Approving a gate other than the instance's current one is rejected server-side. A second approval of an already-passed gate is an idempotent no-op, following the same-assignee precedent. For the Replace OSD MVP there is exactly one gate (before mutation), so per-gate and per-instance granularity coincide; per-gate is the durable shape. Whether approval authorization evidence becomes the first Audit Event use case remains a separate decision.

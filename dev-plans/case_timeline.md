# Case Timeline Read Model
**Version:** 0.1 (Draft)
**Status:** Read model implemented (`GET /api/v1/cases/{id}/timeline`, seeded
events); `case_detected` write side implemented in v0.3.0
(`internal/casedetection` writes it when an alert creates a Case). Remaining
event types are still design-only.
**Audience:** Engineering, Architecture, Product, Contributors
**Project:** Atlas

---

# 1. Purpose

This document sketches the initial Case Timeline read model before implementation.

The goal is to make Case progress visible to operators without confusing user-facing operational history with immutable compliance audit.

This is a design document only. It does not define a storage migration, API implementation, or UI implementation.

---

# 2. Boundary

A Timeline Event is a user-facing chronological event on a Case or Workflow Instance.

An Audit Event is an immutable compliance and security record proving that an action occurred.

The two concepts may reference the same operational moment, but they serve different readers:

- Timeline Events help operators understand what happened and what is next.
- Audit Events help auditors, security reviewers, and incident reviewers prove who did what, when, and under which authorization context.

Timeline Events should be durable enough to survive restarts and support Case history, but the Case Timeline is not the audit ledger.

---

# 3. Initial API Shape

The proposed read-only endpoint is:

```http
GET /api/v1/cases/{id}/timeline
```

The response should be ordered oldest to newest unless the API contract later adds explicit ordering parameters.

Initial response shape:

```json
[
  {
    "id": 1,
    "caseId": 42,
    "type": "case_detected",
    "message": "OSD down case detected from Prometheus alert context.",
    "occurredAt": "2026-08-13T12:00:00Z",
    "actor": {
      "type": "system",
      "displayName": "Atlas"
    },
    "payload": {
      "source": "prometheus",
      "clusterFsid": "00000000-0000-4000-8000-000000000101"
    }
  }
]
```

Fields:

| Field | Purpose |
|---|---|
| `id` | Stable Timeline Event identifier. |
| `caseId` | Case that owns the Timeline Event. |
| `type` | Machine-readable event type. |
| `message` | Short user-facing summary suitable for the Case detail view. |
| `occurredAt` | Time the event occurred. |
| `actor` | Human or system actor to display, when known. |
| `payload` | Type-specific structured context for future UI rendering. |

The API should not expose raw upstream Ceph, Rook, Prometheus, or Kubernetes payloads as Timeline Event payloads. Raw upstream data may be stored as evidence later, but Timeline Events should carry normalized Atlas context.

---

# 4. Initial Event Types

## `case_detected`

Records that Atlas detected a Case-worthy condition.

Suggested payload:

```json
{
  "source": "prometheus",
  "clusterFsid": "00000000-0000-4000-8000-000000000101",
  "signal": "OSD_DOWN"
}
```

## `case_triaged`

Records that a Case moved from initial detection into operator triage.

Suggested payload:

```json
{
  "previousStatus": "detected",
  "newStatus": "triaged"
}
```

## `case_status_changed`

Records a visible Case lifecycle transition.

Suggested payload:

```json
{
  "previousStatus": "triaged",
  "newStatus": "closed"
}
```

## `case_note_added`

Records that a human-visible note was added to the Case.

Suggested payload:

```json
{
  "noteId": 17
}
```

Timeline payloads should reference note identifiers rather than duplicating full note bodies.

## `workflow_attached`

Records that a Workflow Instance was attached to the Case.

Suggested payload:

```json
{
  "workflowId": "replace-osd",
  "workflowInstanceId": 101
}
```

## `workflow_state_changed`

Records a visible Workflow Instance lifecycle transition.

Suggested payload:

```json
{
  "workflowInstanceId": 101,
  "previousState": "waiting_for_approval",
  "newState": "running"
}
```

---

# 5. Actor Shape

The initial actor shape should be display-oriented and avoid committing to the final identity/RBAC schema too early.

```json
{
  "type": "user",
  "id": "user-123",
  "displayName": "Storage Operator"
}
```

Allowed initial actor types:

- `system`
- `user`
- `atlas_agent`
- `provider`

Actor details should be treated as display context, not authorization evidence. Authorization evidence belongs in Audit Events.

---

# 6. Implementation Deferrals

This design intentionally defers:

- PostgreSQL schema and migration design.
- OpenAPI contract changes.
- Go domain model implementation.
- Case detail UI timeline rendering.
- Case note storage and mutation APIs.
- Workflow Instance and Job state implementation.
- Audit Event storage and compliance export behavior.
- RBAC, policy, approval, and privileged operation audit infrastructure.

The next implementation slice should remain read-only and fake-provider friendly.

---

# 7. Open Questions

- Should the initial endpoint support pagination immediately, or wait until Cases have many Timeline Events?
- Should Workflow Instance timelines be exposed only through Case timelines at first, or also through a later `GET /api/v1/workflow-instances/{id}/timeline` endpoint?
- Should Timeline Events have severity or tone metadata, or should the UI derive that from event type?

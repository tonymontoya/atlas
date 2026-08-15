# Workflow Instance and Job State Models

Workflow Instances are durable state machines with states: `pending`, `running`, `waiting_for_approval`, `waiting_for_operator`, and the terminal `succeeded`, `failed`, `cancelled`. Jobs advance `pending` → `dispatched` → `succeeded` or `failed`, with retry meaning a transition back to `pending` governed by the Workflow definition's retry policy. Cancellation is an instance-level terminal only; individual Jobs are not skipped or cancelled independently in the MVP. Terminal states do not revert, mirroring the terminal-closed precedent for Cases.

`waiting_for_operator` pauses the instance at a human Task — the hardware-replacement step of Replace OSD — using the same pause/resume machinery as the approval gate.

**Consequences**

The fake agent adapter can answer synchronously, so Jobs need no in-flight `running` state yet; introducing a real agent's in-flight window is a future ADR. Visible instance transitions emit `workflow_state_changed` Timeline Events on the owning Case. Retry and resumability are constrained to this shape: a restart resumes instances from their durable state, and destructive steps must not execute twice for the same idempotency key.

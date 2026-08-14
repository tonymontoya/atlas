# Durable Workflow State

Workflow Instances and Jobs will be persisted as durable state machines. Atlas workflows may wait for minutes, days, or weeks, so execution state cannot live only in process memory or transient queue messages.

**Consequences**

Workflow execution must be resumable after process restarts. Jobs need stable identifiers, terminal states, retry policy, idempotency keys where practical, and clear linkage to Case Timeline Events and Audit Events.

# Timeline Events Are Not Audit Events

Atlas will model Timeline Events and Audit Events as separate concepts. Timeline Events are user-facing operational history for Cases and Workflow Instances. Audit Events are immutable compliance and security records proving actions, actors, authorization context, and outcomes.

**Consequences**

A single operational moment may produce both a Timeline Event and an Audit Event, but neither record should be treated as a substitute for the other. Timeline APIs can optimize for operator readability and Case progress, while audit storage and APIs must optimize for immutability, authorization evidence, retention, and reviewability.

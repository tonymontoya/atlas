# Atlas Agents Execute Typed Operations Only

Atlas Agents will execute only strongly typed, approved operations such as `RestartDaemon`, `CreateOSD`, or `CollectDiagnostics`. They will not expose arbitrary shell execution, SSH proxying, or generic remote command APIs. This preserves the core safety promise: operators express operational intent and Atlas executes a constrained, auditable plan.

**Consequences**

Every agent operation needs a stable operation type, explicit inputs, authorization context, audit correlation, idempotency behavior, and validation rules. Adding a generic escape hatch is a security model change and requires a new ADR.

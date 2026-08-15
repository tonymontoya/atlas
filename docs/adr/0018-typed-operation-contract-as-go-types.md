# Typed Operation Contract as Go Types with a Registry

The agent operation contract is expressed in Go: each operation such as `CollectHostEvidence` or `DestroyOSD` is a struct with JSON-tagged, validated fields implementing a small `Operation` interface (`OperationType()` and `Validate()`). A registry maps operation type strings to operation structs, and a single shared request envelope carries workflow instance id, Job id, actor identity, approval context, idempotency key, and audit correlation id alongside the typed parameters. Dispatching is: look up the operation type, unmarshal into the typed struct, validate, forward the envelope. Atlas Agents reject unknown operation types and malformed inputs, per ADR-0003 and the security review checklist.

**Consequences**

Job definitions, the dispatcher, and agent adapters share one compile-time-checked decode path; a definition referencing a nonexistent operation or field is a compile error rather than a runtime surprise. Non-Go agents are not blocked forever: JSON Schemas can be generated from the structs if a second implementation language appears. Adding an operation means adding a Go type and registering it — reviewed code, not configuration.

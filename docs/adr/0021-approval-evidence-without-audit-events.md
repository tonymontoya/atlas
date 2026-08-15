# Approval Records Are Their Own Authorization Evidence; Audit Events Stay Deferred

Approvals in the workflow skeleton do not create Audit Events. The Approval record itself — approver subject, display-name snapshot, gate binding, timestamp, reason — is the authorization evidence, satisfying the security review checklist's approval gate without prematurely designing the audit ledger. The record is immutable once written at the store level (no updates, no deletes) so the future audit ledger can reference it as evidence.

**Consequences**

Atlas remains without Audit Event storage until real mutation needs it (ADR-0013 keeps Timeline and Audit distinct; the ledger's correlation and export semantics stay undesigned until a slice exercises them). Timeline Events still show gate openings as `workflow_state_changed`. When Audit Events arrive, approvals and fake-agent dispatch are candidates for backfill tooling, but no schema commitment is made now.

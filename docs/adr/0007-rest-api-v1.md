# REST API v1

Atlas will expose a versioned REST API for v1, with the Web UI consuming the same API as external clients. GraphQL may be evaluated later, but the initial product needs simple operational resources, auditability, and stable integration points more than flexible client-driven queries.

**Consequences**

Public API paths should be versioned. UI behavior must not depend on private backend capabilities that are unavailable to API clients.

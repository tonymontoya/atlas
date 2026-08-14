# Bearer JWT Identity with a Separate Dev Issuer

Atlas authenticates public API write requests with bearer JWTs verified against an OIDC issuer's JWKS. The API is a verifier only — it never issues, refreshes, or stores tokens — which keeps the v0.4.0 surface stateless and identical for UI, CLI, and future agents. Tokens are short-lived (on the order of 15 minutes); there are no refresh tokens and no server-side revocation in v0.4.0 because no privileged operations exist yet (revocation stakes rise when they do, per `dev-plans/security_review_checklist.md`). The web UI keeps tokens in memory rather than persistent browser storage and re-authenticates on expiry.

Local development uses a separate dev issuer command that generates an RSA keypair at startup (never committed — this is a public repository), serves JWKS plus a token endpoint, and prints a usable dev token. The API therefore exercises the real verification code path in dev; there is no static dev-token bypass forking the auth path. It runs only in dev stacks, gated by configuration.

Scope boundaries for v0.4.0: reads stay unauthenticated (the inventory/case read surface is unchanged); any authenticated operator may create or edit Cases manually — RBAC, policy, approvals, and Audit Events stay deferred until the privileged-operations milestone that their MVP criteria actually bind to; internal pipelines (inventory sync, alert evaluation) remain identity-free system-actor commands.

**Alternatives considered**: opaque server-side sessions with a session table (revocable and familiar to ops tools, but adds session storage, cookie/CSRF handling, and a second auth mechanism to keep aligned with the OIDC criterion; rejected for v0.4.0); accepting a static well-known dev token in fake mode (simplest for dev, but the default local path would never exercise real signature verification — a hidden-hazard pattern AGENTS.md forbids; rejected); an optional Keycloak compose profile (closest to real OIDC redirect flows; deferred as an opt-in profile for later UI login-flow work).

**Consequences**

- No logout or revocation in v0.4.0; token expiry is the only session end. Revisit when privileged operations or session revocation requirements arrive.
- A stolen bearer token is valid until expiry; in-memory-only UI storage limits browser persistence but XSS remains the standing mitigation owner.
- Clock skew between issuer and API must stay within the verification leeway or tokens fail near expiry.
- JWKS keys are fetched at first use and cached by key ID; an unknown `kid` (rotation) triggers one re-fetch per request. There is no negative caching, so sustained requests with unknown kids can amplify fetches, and a rotation that drops still-valid keys invalidates outstanding tokens. Acceptable while the dev issuer is the only expected issuer; revisit before real IdPs.
- `GET /healthz` and all v0.4.0-existing read endpoints remain public; the first protected surface is the manual Case write API plus `GET /api/v1/me`.

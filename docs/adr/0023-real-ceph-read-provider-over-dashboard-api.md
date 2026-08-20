# Real Ceph Read Provider over the Dashboard REST API

> Note (2026-08-20): the credential-custody and control-plane-sync aspects of
> this ADR are superseded by ADR-0025 — the Dashboard REST reading approach
> survives, relocated into the enrolled Atlas Agent. Endpoint, auth, error,
> and testing decisions below remain authoritative for the provider code.

Atlas's first real (non-fake) provider implements `CephReadProvider` for bare-metal Ceph 18 (Reef) by reading the Ceph Dashboard REST API under `/api`, selected per ADR-0006 (supported interfaces first) and the research in `dev-plans/provider_api_research.md`, which recommends the Dashboard API as the first-class read source. Every response is normalized to Atlas domain types inside the provider package (`internal/providers/ceph`), so the Dashboard's between-release endpoint drift is contained behind the existing provider contract; the MON command API and CLI are not used for reads and remain candidates for typed operations later. Rook-managed clusters keep their own future provider per ADR-0012 parity; the Dashboard transport may turn out to serve that provider too, but Rook discovery and design stay out of this decision.

Authentication is username and password for a dedicated, read-only Dashboard user. The provider logs in once, caches the resulting JWT in-process, and re-authenticates once on a 401 before failing. A static token was rejected because Dashboard JWTs expire, which would strand unattended sync runs until a human re-mints (and minting requires a login anyway); least privilege comes from the Dashboard user's role, not the credential type. Credentials exist only in the explicit `ATLAS_PROVIDER_MODE=ceph` opt-in path — never in ordinary local development paths (ADR-0011; `dev-plans/local_development_topology.md`) and never in this repository.

Error normalization is pinned as: context cancellation/deadline → `Timeout`; transport failures (connect refused, DNS, TLS) and HTTP 5xx → `Unavailable`; HTTP 401 after the re-auth attempt and HTTP 403 → `Unauthorized`; HTTP 404 on host-scoped lookups → `NotFound`; an undecodable body or shape violation → `MalformedResponse`. The `Partial` class is not produced by this provider in v0.6.0 — no Dashboard read legitimately returns partial data — so the shared contract suite must allow a provider to omit that scenario; the class stays in the contract for providers that can produce it.

Testing never touches real Ceph: all unit, contract, and wiring tests run against an in-process fake Dashboard (httptest) serving Dashboard-shaped JSON, and the provider runs the shared `contracttest` suite like any other implementation. Response-shape assumptions pinned from Reef documentation rather than a live cluster are documented in-code at the decode sites. Validation against a real cluster is explicit, optional, and manual, per `dev-plans/mvp_test_strategy.md` and `dev-plans/pre_development_checklist.md`.

**Alternatives considered**: wrapping `ceph` CLI JSON output (ADR-0006's second choice; requires shell/SSH reach into a monitor host, conflicting with the no-shell-proxy posture; deferred); the MON command API directly (valuable for future typed operations but heavier auth/proxy design with no inventory breadth advantage; deferred); Prometheus as the read source (metrics are not inventory truth; alerting already has its own `ObservabilityProvider` seam); static bearer token (breaks unattended syncs on expiry; rejected).

**Consequences**

- Clusters must have the Dashboard mgr module enabled and reachable; clusters without it cannot be read by this provider in v0.6.0.
- Endpoint drift is absorbed in one package, but shapes pinned from docs may need correction when first exercised against a live Reef cluster; those sites are marked.
- Atlas holds a long-lived Dashboard credential in environment configuration; secret storage and rotation design is deferred until real-deployment work (security checklist).
- Local development and CI gain no new dependency: fake mode remains the default everywhere, and the dev stack is unchanged.
- Reads still flow exclusively through inventory sync snapshots (ADR-0014); pointing the live API read path (`ATLAS_READ_SOURCE`) at a real cluster is deferred to a later slice.
- The shared provider contract suite gains the ability to omit the `Partial` scenario for providers that cannot produce it.

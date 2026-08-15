# In-Process Fake Agent Adapter Behind an Interface

The workflow dispatcher depends on an `AgentAdapter` interface. The only v0.5.0 implementation is a fake adapter: an in-process Go type selected by configuration (mirroring `ATLAS_PROVIDER_MODE=fake`), with deterministic scenario-driven outcomes including a failure scenario that exercises retry and idempotency-key dedup. Requests still round-trip through the real JSON encode/decode of the typed-operation envelope so the wire contract is exercised honestly. The fake adapter never touches real Ceph.

A network-speaking agent (HTTP, mTLS, registration, heartbeat — security checklist §9) is deferred until the real Atlas Agent design; that adapter will slot behind the same interface.

**Consequences**

The fake loop can run fully in CI and local Docker without trust-model design, while the contract, durable state machine, retry, and restart-resume behavior all get exercised. Agent identity and transport security remain undesigned and unblocked. Mutation stays disabled everywhere real; only the fake adapter simulates execution.

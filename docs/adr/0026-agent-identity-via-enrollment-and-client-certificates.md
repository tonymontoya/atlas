# Agent Identity Is an Atlas-Issued Client Certificate, Enrolled with a One-Time Credential

Registering a Cluster (ADR-0025) issues a single-use Enrollment Credential, returned once in the registration response and never stored in plaintext. The Atlas Agent generates its key pair locally and submits a CSR plus the Enrollment Credential to the enrollment endpoint; Atlas verifies and burns the credential, then issues the Agent a client certificate from an Atlas-held certificate authority. Every subsequent Agent connection to Atlas authenticates with that certificate over mutual TLS. Private keys never leave the Agent host; a stolen Enrollment Credential is worthless after first use (or expiry); revocation is Atlas rejecting the certificate, and rotation is certificate re-issue — no credential ever sits in the control plane.

Bearer tokens were rejected because a copyable secret granting standing access reintroduces exactly the weakness that motivated ADR-0025: proof-of-possession is required, so a replayed or exfiltrated authenticator must fail. Operator-generated pre-shared certificates were rejected for manual key-handling ergonomics that invite misuse and do not scale to a fleet.

Go standard library (`crypto/x509`) covers the whole mechanism. v0.7 keeps lifetimes long and renewal manual; v0.9 hardens the same mechanism (shorter lifetimes, automated renewal, revocation lists) rather than replacing it, since operation dispatch joins this identity.

**Consequences**

- Atlas runs an internal CA; its key is control-plane configuration with its own protection story (security checklist; single-zone MVP keeps it on the Atlas host).
- Enrollment Credentials are single-use, short-lived, and stored only in hashed/one-time form if persisted at all.
- The dev stack and CI never exercise real certificates end to end beyond an in-process test CA; the fake-provider paths are unchanged.
- An Agent's certificate maps to exactly one registered Cluster, so Atlas attributes pushed observations to clusters by certificate identity, not by payload claims.

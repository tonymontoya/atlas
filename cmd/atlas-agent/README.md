# Atlas Agent

`atlas-agent` runs inside a Cluster's trust domain (ADR-0025, ADR-0026).
On first start it enrolls with Atlas — a locally generated Ed25519 key,
a CSR, and the Cluster's one-time Enrollment Credential — and persists
the issued client certificate under `ATLAS_AGENT_STATE_DIR`
(`certificate.pem` + `key.pem`). It then collects full inventory
batches from the local Ceph Dashboard and pushes them to
`POST /api/v1/agent/observations` over mutual TLS on an internal
ticker.

The Agent is read-only by construction in v0.0.8: it has no dispatch or
command surface, and Dashboard credentials never leave the Agent.

## Configuration (agent-local `ATLAS_AGENT_*` environment)

| Variable | Default | Purpose |
|---|---|---|
| `ATLAS_AGENT_ATLAS_URL` | required | Atlas API base URL; must be `https` (ingestion requires mutual TLS) |
| `ATLAS_AGENT_ATLAS_CA_PATH` | system roots | PEM CA bundle verifying the Atlas serving certificate |
| `ATLAS_AGENT_ATLAS_INSECURE_TLS` | `false` | dev-only: skip Atlas certificate verification |
| `ATLAS_AGENT_ENROLLMENT_CREDENTIAL` | unset | one-time credential from the Cluster's registration; needed for first enrollment and renewal |
| `ATLAS_AGENT_ENROLLMENT_CREDENTIAL_FILE` | unset | file carrying the same one-time credential (container deployments); mutually exclusive with the inline form |
| `ATLAS_AGENT_STATE_DIR` | `atlas-agent-state` | where the certificate chain and private key persist |
| `ATLAS_AGENT_COLLECT_INTERVAL` | `60s` | collection ticker; must be positive |
| `ATLAS_AGENT_RETRY_INITIAL` / `ATLAS_AGENT_RETRY_MAX` | `1s` / `30s` | exponential backoff bounds after transient failures |
| `ATLAS_AGENT_DASHBOARD_URL` | required | Ceph Dashboard base URL |
| `ATLAS_AGENT_DASHBOARD_USER` / `ATLAS_AGENT_DASHBOARD_PASSWORD` | required | read-only Dashboard credentials; never sent to Atlas |
| `ATLAS_AGENT_DASHBOARD_CLUSTER_NAME` | `ceph` | cluster name reported in observation batches |
| `ATLAS_AGENT_DASHBOARD_INSECURE_TLS` | `false` | skip Dashboard certificate verification |

## Running

```sh
# one collect-and-push cycle (deterministic CI runs)
atlas-agent -once

# daemon: collect on the ticker until shutdown
atlas-agent
```

Transient failures (network errors, HTTP 429, 5xx) retry with backoff;
permanent ones (invalid credential, revoked certificate, FSID
conflict) stop the Agent for an operator. An expired certificate
requires re-registration in Atlas and a fresh Enrollment Credential
(renewal is re-enrollment, ADR-0026).

Mutating Agent operations remain gated by
`dev-plans/security_review_checklist.md`.

# Development Assets

Local development assets live here.

Default development uses fake providers and must not require production Ceph, Rook, AWX, Ansible, Redis, or NATS.

## Local PostgreSQL

The local Docker stack starts PostgreSQL only. Atlas API still runs directly on
the host in fake-provider mode.

```sh
make db-up
make db-migrate
make dev-fake
```

Persist one fake-provider inventory observation with:

```sh
make db-sync-fake
```

Run one fake-provider alert evaluation (creates a Case from a firing alert,
deduplicated across reruns) with:

```sh
make db-alert-eval-fake
```

Run the API against PostgreSQL-backed reads after syncing:

```sh
ATLAS_READ_SOURCE=postgres make dev-fake
```

Run PostgreSQL-backed integration tests with:

```sh
make db-test
```

The default local database URL is:

```text
postgres://atlas:atlas_dev@127.0.0.1:15432/atlas?sslmode=disable
```

Stop the database with:

```sh
make db-down
```

## Full dev stack with the enrolled Agent loop

`make dev-stack-up` (profile `stack`) runs everything above plus the
whole enrolled Agent loop with nothing real except the software itself —
only Ceph is fake:

- `dashboard` — the fixture-backed fake Ceph Dashboard
  (`cmd/atlas-dev-dashboard` over `internal/providers/ceph/dashfake`,
  the same handler the provider tests exercise through `dashtest`).
- An ephemeral dev CA generated inside the API container at every start
  (`cmd/atlas-dev-ca`): a fresh authority plus an API serving
  certificate on the `atlas-dev-ca` volume, so the in-memory enrollment
  authority always matches the files the Agent trusts. No real CA
  anywhere.
- The API serves an additional TLS listener (`:8443`, in-network only,
  not published) with client-certificate verification, while the
  published HTTP port keeps Operator and web UI traffic.
- `agent-bootstrap` — retires the previous bring-up's rows through the
  Operator API (the live FSID holder and any dormant same-name
  registration), creates a fresh Cluster registration with a dev-issuer
  bearer token, and writes the one-time Enrollment Credential to a file
  for the Agent.
- `atlas-agent` — the real Agent binary: enrolls with the bootstrap's
  credential, collects from the fake Dashboard every 10s, and pushes
  over mutual TLS. Its state directory is ephemeral by design: each
  bring-up mints a fresh CA and a fresh registration, so the Agent
  enrolls anew.

Bring-up semantics: the loop is provisioned per full bring-up
(`make dev-stack-up` after a down, or every `make dev-stack-check` run,
which downs the stack on exit). Restarting individual services
mid-bring-up can break the loop until the next full down/up — the
one-time credential is burned and the Agent deliberately stops on
permanent errors instead of retrying them.

`make dev-stack-check` asserts the full register → enroll → push → read
loop, the seeded fake scenarios, the workflow loop, and the
authentication rejections, and stays green across reruns against the
persistent PostgreSQL volume.

Stop the full stack with:

```sh
make dev-stack-down
```

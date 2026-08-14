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

# Fixtures

Fake provider fixtures live here.

Rules:

- scrub sensitive values
- avoid production endpoints
- avoid real credentials
- keep raw upstream-like payloads separate from normalized examples

## Ceph fake scenarios

Each scenario under `ceph/<scenario>/` holds one JSON file per read method:
`cluster_identity.json`, `health.json`, `osds.json`, `hosts.json`,
`devices.json`, `daemons.json`, `pools.json`.

`devices.json` holds every scenario's Storage Devices (each row names its
Host); the provider filters per Host for HostDevices reads. `osdId` is the
currently backing OSD identity where one exists; absence means the device is
not backing an OSD. A Storage Device's serial number is its normalized
identity — OSD identities may change over a device's lifetime while the
serial persists.

Cluster-state scenarios (all read methods return normalized data):

- `reef-healthy-baremetal` / `reef-osd-down-baremetal` (Ceph 18, bare-metal)
- `reef-healthy-rook` / `reef-osd-down-rook` (Ceph 18, Rook-managed)
- `pacific-readonly` (Ceph 16, read-only)

Error scenarios simulate upstream failure modes instead of cluster states:

- `provider-unauthorized`: every fixture is an error directive (see below).
- `provider-malformed`: every fixture is deliberately invalid JSON; the fake
  provider normalizes this to `MalformedResponse`.
- `provider-partial`: every fixture is a `Partial` error directive describing
  what the simulated upstream could not collect.
- a missing scenario directory exercises `Unavailable`.

## Prometheus fake scenarios

Each scenario under `prometheus/<scenario>/` holds `alerts.json`: an array of
normalized alerts (name, severity, labels, annotations, startedAt, state,
source). Alert states are `firing`, `pending`, or `resolved`.

Cluster-state scenarios:

- `osd-down-alert`: one firing `CephOSDDown` warning against the
  `reef-baremetal-osd-down` cluster (Ceph 18, bare-metal).

Error scenarios simulate observability-source failure modes:

- `provider-unauthorized`: an error directive (see below).
- `provider-malformed`: deliberately wrong-shaped JSON; the fake provider
  normalizes this to `MalformedResponse`.
- a missing scenario directory exercises `Unavailable`.

### Error directives

A fixture may be an error envelope instead of normalized data:

```json
{
  "error": {
    "class": "Unauthorized",
    "message": "simulated upstream rejected provider credentials"
  }
}
```

The fake provider returns an `apperr.Error` (the Atlas error taxonomy,
ADR-0024) of the given class. `class` must
be one of the shared error classes in `dev-plans/provider_contracts.md` §5;
anything else normalizes to `MalformedResponse`. A `Partial` directive means
the simulated upstream could not finish collection; the current read model
represents this as an error with no partial payload.

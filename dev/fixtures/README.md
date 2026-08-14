# Fixtures

Fake provider fixtures live here.

Rules:

- scrub sensitive values
- avoid production endpoints
- avoid real credentials
- keep raw upstream-like payloads separate from normalized examples

## Ceph fake scenarios

Each scenario under `ceph/<scenario>/` holds one JSON file per read method:
`cluster_identity.json`, `health.json`, `osds.json`.

Error scenarios simulate upstream failure modes instead of cluster states:

- `provider-unauthorized`: every fixture is an error directive (see below).
- `provider-malformed`: every fixture is deliberately invalid JSON; the fake
  provider normalizes this to `MalformedResponse`.
- `provider-partial`: every fixture is a `Partial` error directive describing
  what the simulated upstream could not collect.
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

The fake provider returns a `ProviderError` of the given class. `class` must
be one of the shared error classes in `dev-plans/provider_contracts.md` §5;
anything else normalizes to `MalformedResponse`. A `Partial` directive means
the simulated upstream could not finish collection; the current read model
represents this as an error with no partial payload.

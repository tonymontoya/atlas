# Inventory History Through Per-Snapshot Observations

Atlas persists Hosts, Storage Devices, Ceph Daemons, and Pools as per-snapshot observation rows keyed to `inventory_snapshots` (natural keys: Host name, device serial per Host, daemon type plus name, pool ID), in the same pattern as `osd_observations`. Storage Device identity in the read model is the serial number plus Host; no durable entity tables with surrogate IDs are introduced yet.

Historical Storage Device to OSD relationships are preserved by the observation rows themselves: each snapshot records which OSD identity a device backs, and the `storage_device_osd_history` view aggregates first/last observed timestamps per (Cluster, device, OSD identity) so a device that backed OSD 3 and later OSD 7 shows both links.

**Alternatives considered**: durable `hosts`/`storage_devices` entity tables with surrogate IDs plus a `device_osd_assignments` link table upserted per sync. Deferred: identity management and upsert semantics are not needed by any MVP read, and entity IDs become load-bearing only when workflows, Atlas Agent operations, or multi-cluster addressing need stable references. Introducing them later is additive.

**Consequences**

- Current-state reads use `cluster_current_*` views (latest snapshot wins), consistent with health and OSD reads.
- History queries scan observations; indexes on (cluster, serial) keep them cheap at MVP scale.
- Renaming a Host or reassigning a serial starts a new observation identity rather than updating an entity; acceptable for a read model.

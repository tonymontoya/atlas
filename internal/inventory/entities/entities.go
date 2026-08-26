// Package entities declares the list-shaped inventory entities once.
// Every consumer of the cluster-scoped read family — the Postgres read
// model, the single-cluster provider adapter, the API surface, and the
// provider contract tests — derives its per-entity wiring from this
// registry, so adding an entity means adding one declaration entry
// plus the artifacts that cannot be derived (a migration, an OpenAPI
// path, a web page).
//
// Entries are pure metadata: identifiers are compile-time constants
// consumed only by server-side SQL construction, never user input.
// Row scans stay with the consumer that owns the scan target type.
//
// Health is deliberately absent: it is singleton-shaped (one row, a
// JSON checks column, and a view the cluster index joins) rather than
// list-shaped, so it stays hand-written at each consumer.
//
// Failure convention for a missing entry: consumers that construct at
// process start (the singlecluster adapter, the API route table)
// panic; test-only consumers (the contract suite) report the gap as a
// test failure. Either way the omission surfaces before an entity can
// be served half-wired.
package entities

// Entity declares one list-shaped inventory read: how the read model
// names it, which view serves it, the columns and ordering of that
// view, the message reported when a known cluster's latest snapshot
// lacks the entity, and the name of the store read method that serves
// it (used by completeness tests to keep the registry and its
// consumers from drifting).
type Entity struct {
	// Noun is the route noun the entity is addressed by, e.g. "osds".
	Noun string
	// StoreMethod is the PostgresStore method serving this entity,
	// e.g. "ClusterOSDs".
	StoreMethod string
	// View is the read-model view name serving latest-snapshot rows.
	View string
	// Columns are the view columns, in scan order.
	Columns string
	// OrderBy is the view ordering for the entity's rows.
	OrderBy string
	// NotFound is the message reported when the cluster exists but
	// its latest snapshot has no rows for the entity.
	NotFound string
}

// OSDs declares the OSD inventory read.
var OSDs = Entity{
	Noun:        "osds",
	StoreMethod: "ClusterOSDs",
	View:        "cluster_current_osds",
	Columns:     "osd_id, host, osd_up, osd_in, device",
	OrderBy:     "osd_id",
	NotFound:    "current OSD inventory not found",
}

// Hosts declares the host inventory read.
var Hosts = Entity{
	Noun:        "hosts",
	StoreMethod: "ClusterHosts",
	View:        "cluster_current_hosts",
	Columns:     "host_name, address",
	OrderBy:     "host_name",
	NotFound:    "current host inventory not found",
}

// StorageDevices declares the Storage Device inventory read.
var StorageDevices = Entity{
	Noun:        "storage-devices",
	StoreMethod: "ClusterStorageDevices",
	View:        "cluster_current_storage_devices",
	Columns:     "host_name, serial, device_type, device_path, device_health, osd_id",
	OrderBy:     "host_name, serial",
	NotFound:    "current storage device inventory not found",
}

// Daemons declares the Ceph Daemon inventory read.
var Daemons = Entity{
	Noun:        "daemons",
	StoreMethod: "ClusterDaemons",
	View:        "cluster_current_daemons",
	Columns:     "daemon_type, daemon_name, host_name, status, ceph_version",
	OrderBy:     "daemon_type, daemon_name",
	NotFound:    "current Ceph Daemon inventory not found",
}

// Pools declares the Pool inventory read.
var Pools = Entity{
	Noun:        "pools",
	StoreMethod: "ClusterPools",
	View:        "cluster_current_pools",
	Columns:     "pool_id, name, pool_type, size, min_size",
	OrderBy:     "pool_id",
	NotFound:    "current Pool inventory not found",
}

// All is the ordered registry of declared list entities. Consumers
// that serve or cover entities by looping must loop this slice, and
// their completeness tests must fail when an entry lacks its wiring.
var All = []Entity{OSDs, Hosts, StorageDevices, Daemons, Pools}

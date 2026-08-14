# Atlas Provider API Research
**Version:** 0.1 (Draft)  
**Status:** Pre-development Research  
**Audience:** Engineering, Architecture, Product, Contributors  
**Project:** Atlas

---

# 1. Purpose

This document summarizes official Ceph 18 Reef and Rook API/documentation findings that should shape Atlas provider boundaries.

This is research, not final interface design. Its job is to prevent Atlas from binding itself to local runbooks, AWX, Ansible, or unsupported Ceph internals before implementation begins.

---

# 2. Source Scope

Primary sources reviewed:

- Ceph Reef RESTful API documentation: `https://docs.ceph.com/en/reef/mgr/ceph_api/`
- Ceph Reef MON Command API documentation: `https://docs.ceph.com/en/reef/api/mon_command_api/`
- Ceph Dashboard documentation: `https://docs.ceph.com/en/latest/mgr/dashboard/`
- Ceph release documentation for Reef: `https://docs.ceph.com/en/latest/releases/reef/`
- Rook CephCluster CRD documentation: `https://rook.io/docs/rook/latest/CRDs/Cluster/ceph-cluster-crd/`
- Rook CRD specification: `https://rook.io/docs/rook/latest-release/CRDs/specification/`
- Rook Ceph Dashboard documentation: `https://rook.io/docs/rook/latest/Storage-Configuration/Monitoring/ceph-dashboard/`
- Rook toolbox documentation: `https://rook.io/docs/rook/latest-release/Troubleshooting/ceph-toolbox/`
- Rook Prometheus monitoring documentation: `https://www.rook.io/docs/rook/latest-release/Storage-Configuration/Monitoring/ceph-monitoring/`

---

# 3. Findings

## Ceph 18 Reef

Reef is Ceph 18.

The Ceph release index currently shows Reef as an active release line, with `18.2.x` releases. Atlas MVP should target the exact pilot minor versions once known, but `Ceph 18 / Reef` is the correct major-version target.

## Ceph Dashboard REST API

Ceph Reef exposes a RESTful API through the Ceph Dashboard module under the `/api` base path.

Important properties:

- HTTP and JSON API
- JWT authentication
- authorization checks
- explicit endpoint versioning through MIME `Accept` headers
- per-endpoint versioning
- some endpoints are still under active development and may change between Ceph releases

Useful MVP read endpoints include:

- `/api/summary`
- `/api/health/full`
- `/api/health/minimal`
- `/api/health/get_cluster_capacity`
- `/api/health/get_cluster_fsid`
- `/api/daemon`
- `/api/host/{hostname}/daemons`
- `/api/host/{hostname}/devices`
- `/api/host/{hostname}/inventory`
- `/api/host/{hostname}/smart`
- `/api/osd/{svc_id}`
- `/api/osd/{svc_id}/devices`
- `/api/osd/safe_to_delete`
- `/api/osd/safe_to_destroy`
- `/api/pool/{pool_name}/configuration`
- `/api/crush_rule`
- `/api/prometheus/data`
- `/api/prometheus/rules`
- RGW-related `/api/rgw/...` endpoints for object-store visibility

Research implication:

The Dashboard REST API is a good first-class read and validation source, especially for inventory, health, OSD safety checks, daemon inventory, host evidence, pool information, capacity, and RGW visibility. Atlas should still wrap it behind provider interfaces because endpoint stability varies.

## Ceph MON Command API

Ceph documents MON commands with parameters, Ceph module ownership, and required permissions.

Examples relevant to Atlas:

- monitor metadata
- monitor map
- quorum-safe monitor operations such as `mon ok-to-stop`
- monitor removal commands and their `rw` permission requirements
- cluster log access

Research implication:

The MON Command API is important for typed operations, permission modeling, safety prechecks, and CLI/API fallback behavior. Atlas should model any MON command use as a typed provider operation, not an arbitrary command string.

## Rook CephCluster CRD

Rook manages Ceph through Kubernetes CRDs.

The `CephCluster` CRD supports several cluster modes:

- host storage clusters
- PVC-backed storage clusters
- stretched clusters
- external Ceph clusters

The `CephCluster` CRD also carries Ceph version image settings and dashboard settings.

Research implication:

Rook is a first-class management path for Ceph clusters, but Atlas should still model the underlying storage target as a Ceph Cluster. Rook/Kubernetes details belong behind the Rook provider.

## Rook CRD Surface

The Rook CRD specification includes Ceph resources such as:

- `CephCluster`
- `CephBlockPool`
- `CephObjectStore`
- `CephObjectStoreUser`
- `CephFilesystem`
- `CephClient`
- `CephNFS`
- `CephRBDMirror`
- realm, zone, and zone-group object-store resources

Research implication:

Rook provider code should use typed Kubernetes clients for Rook CRDs rather than shelling out to `kubectl`.

## Rook Dashboard

Rook can enable the Ceph Dashboard through the `CephCluster` CRD. Rook creates dashboard services and dashboard credentials. Rook docs also note physical disk visibility in the dashboard depends on Rook-specific manager module and discovery settings.

Research implication:

For Rook clusters, Atlas may still use the Ceph Dashboard API for Ceph-level information when enabled, but it must tolerate clusters where dashboard exposure, credentials, or physical disk discovery are configured differently.

## Rook Toolbox

Rook toolbox supports interactive execution and one-time jobs for Ceph commands. Official docs describe the toolbox as a debugging and testing aid.

Research implication:

Atlas should not use interactive toolbox shells as its core execution model. A Rook provider may use Kubernetes-native jobs or controlled mechanisms for narrow, typed operations if needed, but those must remain typed Atlas operations with audit, approval, and idempotency. The toolbox is useful for understanding access patterns, not as an architectural dependency.

## Rook Prometheus Monitoring

Rook includes built-in metrics collectors/exporters and can integrate with Prometheus. The docs describe dashboard Prometheus endpoint configuration and Prometheus alert rule installation.

Research implication:

Atlas should treat Prometheus as an observability provider for health context, alert-to-Case mapping, and SLO context. Prometheus should not be the source of truth for inventory.

---

# 4. Recommended Provider Shape

Atlas should define provider interfaces around Atlas intents, not around upstream transport details.

Recommended initial providers:

```text
CephProvider
  Health()
  ClusterIdentity()
  Capacity()
  Daemons()
  Hosts()
  HostInventory(host)
  HostDevices(host)
  OSDs()
  OSDDetails(osdID)
  OSDDevices(osdID)
  Pools()
  CrushRules()
  RGWSummary()
  SafetyCheck(operation, target)

RookProvider
  ClusterStatus()
  ClusterVersion()
  ClusterMode()
  RookResources()
  DashboardEndpoint()
  PrometheusEndpoint()
  CephAccess()

AgentProvider
  CollectHostEvidence(host)
  CollectDeviceEvidence(host, device)
  ValidateReplacementDevice(host, serial)
  ExecuteTypedOperation(operation)
```

Provider implementations:

```text
BareMetalCephProvider
  uses Ceph Dashboard REST API where stable enough
  uses supported Ceph command API/CLI fallback behind typed methods
  uses Atlas Agent for host/device evidence and mutation

RookCephProvider
  uses Kubernetes API and Rook CRDs for deployment context
  uses Ceph Dashboard REST API or Ceph access path for Ceph state
  uses Atlas Agent or Kubernetes-native typed jobs for host/device evidence and mutation
```

---

# 5. MVP Capability Mapping

| Atlas Capability | Bare-Metal Ceph 18 | Rook-Managed Ceph 18 | Notes |
|---|---|---|---|
| Cluster registration | Ceph API for FSID/health plus operator-supplied endpoint metadata | Rook `CephCluster` plus Ceph API identity | Store Atlas metadata separately from Ceph. |
| Health sync | `/api/summary`, `/api/health/full`, `/api/health/minimal` | Rook status plus Ceph health endpoints | Normalize into one Atlas health model. |
| Capacity sync | `/api/health/get_cluster_capacity` plus pool data | Ceph capacity plus Rook context | Prometheus can enrich, not own. |
| Daemon inventory | `/api/daemon`, host daemon endpoints | K8s workloads plus Ceph daemon state | Normalize MON/MGR/OSD/MDS/RGW. |
| Host inventory | host inventory/device/smart endpoints plus Agent evidence | Rook/K8s nodes plus Ceph host data and Agent evidence | Do not assume physical disks are visible in dashboard. |
| OSD inventory | OSD endpoints plus command fallback | Ceph OSD state plus Rook OSD pods/deployments | Preserve Storage Device/OSD history. |
| OSD safety checks | `safe_to_delete`, `safe_to_destroy`, command API checks where needed | same Ceph safety checks plus Rook context | Safety checks must be required before mutation. |
| Replace OSD | Atlas Workflow and typed Agent operations | Atlas Workflow and Rook-aware typed provider operations | No AWX/Ansible dependency. |
| Pool visibility | pool and CRUSH endpoints | Ceph pool/CRUSH endpoints plus Rook pool CRDs where applicable | CRDs describe desired state; Ceph confirms actual state. |
| RGW visibility | RGW API endpoints and Prometheus context | `CephObjectStore` CRDs plus RGW/Ceph API and Prometheus context | Keep object-store workflows post-MVP unless selected. |
| Alert-to-Case | Prometheus alert input | Prometheus/Rook alert input | Alerts create Cases; Prometheus is not inventory source. |

---

# 6. Open Questions

- Which exact Ceph 18 minor version or versions will be used for pilot validation?
- Are Ceph Dashboard REST API endpoints enabled and reachable in representative bare-metal clusters?
- Are Ceph Dashboard REST API endpoints enabled and reachable in representative Rook clusters?
- Which Rook versions are present in the first pilot environments?
- For Rook clusters, should Atlas prefer direct Kubernetes API access to Rook CRDs or a constrained in-cluster Atlas Agent?
- Which host/device evidence should come from Ceph Dashboard, Rook discovery, Prometheus, or Atlas Agent?
- Which mutating OSD replacement steps can be expressed through supported Ceph APIs, and which require host or Rook deployment operations?
- Should Atlas expose the Ceph API endpoint status as part of cluster registration diagnostics?

---

# 7. Near-Term Recommendation

Before code scaffolding, define interfaces for:

- Ceph read provider
- Ceph command/safety provider
- Rook deployment provider
- Atlas Agent evidence provider
- Atlas Agent mutation provider

Then scaffold only the read paths first:

1. cluster identity
2. health
3. capacity
4. daemons
5. OSD inventory
6. host/device inventory

Mutating workflows should wait until safety checks, RBAC, audit, approval, and agent operation contracts are implemented.

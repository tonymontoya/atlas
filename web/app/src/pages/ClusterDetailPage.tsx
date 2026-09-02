import React from "react";
import { useParams } from "react-router-dom";
import { Column, Grid, InlineNotification } from "@carbon/react";
import { loadClusterView, type ClusterView } from "../api";
import {
  ClusterNotFound,
  ClusterPageHeader,
} from "../components/ClusterPageHeader";
import {
  DaemonTable,
  HealthChecksTable,
  OSDTable,
  PoolTable,
  StorageDeviceTable,
} from "../components/inventoryTables";
import { OperatorPanel } from "../components/OperatorPanel";
import { ErrorState, MetricTile, StatusTag } from "../components/ui";
import { useOperator } from "../operator";
import { useResource } from "../resources";
import {
  daemonStatusCounts,
  daemonStatusSummary,
  type DaemonStatusCounts,
} from "../inventory";
import { agentLastSeenLabel, agentLastPushLabel, healthStatusLabel } from "../clusters";
import { toneForHealth } from "../tones";

function daemonTileTone(counts: DaemonStatusCounts): "ok" | "warn" | "err" {
  if (counts.running === counts.total) {
    return "ok";
  }
  if (counts.error > 0) {
    return "err";
  }
  return "warn";
}

// The cluster overview (issue #42): summary tiles, health checks, and
// inventory tables, all keyed by the FSID in the route. The cluster's
// Cases and Sync runs live behind the section tabs.
export function ClusterDetailPage() {
  const { fsid } = useParams();
  const { operator, signIn, signOut } = useOperator();

  const view = useResource(
    fsid ? (signal) => loadClusterView(fsid, signal) : null,
    [fsid],
  );

  if (!fsid) {
    return <ErrorState message="No cluster FSID in the route." />;
  }

  if (view.loading && (!view.data || "notFound" in view.data)) {
    return <p className="atlas-empty">Loading cluster…</p>;
  }

  if (view.data && "notFound" in view.data) {
    return <ClusterNotFound fsid={fsid} />;
  }

  if (view.error || !view.data) {
    return <ErrorState message={view.error ?? "No cluster data returned."} />;
  }

  const clusterView: ClusterView = view.data;
  const { cluster, health, osds, hosts, storageDevices, daemons, pools, cases, syncRuns } = clusterView;
  const downOsds = osds.filter((osd) => !osd.up).length;
  const outOsds = osds.filter((osd) => !osd.in).length;
  const daemonCounts = daemonStatusCounts(daemons);

  return (
    <>
      <ClusterPageHeader
        fsid={fsid}
        active="Overview"
        title={cluster.name}
        intro={
          <>
            <span className="atlas-mono">{cluster.fsid}</span> · {cluster.clusterType} ·{" "}
            {cluster.cephVersion ?? "Ceph version unreported"}
          </>
        }
      />

      <OperatorPanel operator={operator} onSignIn={signIn} onSignOut={signOut} />

      {clusterView.casesUnavailable ? (
        <InlineNotification
          kind="warning"
          lowContrast
          title="Cases unavailable"
          subtitle={clusterView.casesUnavailable}
        />
      ) : null}
      {clusterView.syncRunsUnavailable ? (
        <InlineNotification
          kind="warning"
          lowContrast
          title="Sync run history unavailable"
          subtitle={clusterView.syncRunsUnavailable}
        />
      ) : null}

      <Grid fullWidth className="atlas-metrics">
        <Column sm={4} md={4} lg={4}>
          <MetricTile
            label="Health"
            value={healthStatusLabel(cluster.healthStatus)}
            detail={cluster.healthSummary ?? health.summary}
            tone={toneForHealth(cluster.healthStatus)}
          />
        </Column>
        <Column sm={4} md={4} lg={4}>
          <MetricTile
            label="Agent"
            value={agentLastSeenLabel(cluster.agentLastSeen)}
            detail={`last push ${agentLastPushLabel(syncRuns)}`}
          />
        </Column>
        <Column sm={4} md={4} lg={4}>
          <MetricTile
            label="OSDs"
            value={osds.length}
            detail={`${downOsds} down, ${outOsds} out`}
          />
        </Column>
        <Column sm={4} md={4} lg={4}>
          <MetricTile
            label="Hosts"
            value={hosts.length}
            detail={`${storageDevices.length} Storage Devices`}
          />
        </Column>
        <Column sm={4} md={4} lg={4}>
          <MetricTile
            label="Ceph Daemons"
            value={daemons.length}
            detail={daemonStatusSummary(daemonCounts)}
            tone={daemonTileTone(daemonCounts)}
          />
        </Column>
        <Column sm={4} md={4} lg={4}>
          <MetricTile label="Pools" value={pools.length} />
        </Column>
        <Column sm={4} md={4} lg={4}>
          <MetricTile label="Cases" value={cases.length} detail="behind the Cases tab" />
        </Column>
      </Grid>

      <section className="atlas-panel" aria-label="Health checks">
        <div className="atlas-panel-heading-row">
          <h2 className="atlas-panel-heading">Health Checks</h2>
          <StatusTag
            label={healthStatusLabel(health.status)}
            tone={toneForHealth(health.status)}
          />
        </div>
        <HealthChecksTable health={health} />
      </section>

      <section className="atlas-panel" aria-label="OSD inventory">
        <h2 className="atlas-panel-heading">OSD Inventory</h2>
        <OSDTable osds={osds} />
      </section>

      <section className="atlas-panel" aria-label="Storage devices">
        <h2 className="atlas-panel-heading">Storage Devices</h2>
        <StorageDeviceTable devices={storageDevices} />
      </section>

      <section className="atlas-panel" aria-label="Ceph daemons">
        <h2 className="atlas-panel-heading">Ceph Daemons</h2>
        <DaemonTable daemons={daemons} />
      </section>

      <section className="atlas-panel" aria-label="Pools">
        <h2 className="atlas-panel-heading">Pools</h2>
        <PoolTable pools={pools} />
      </section>
    </>
  );
}

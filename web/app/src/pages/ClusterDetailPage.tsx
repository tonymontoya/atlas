import React from "react";
import { Link, useParams } from "react-router-dom";
import { Column, Grid, InlineNotification } from "@carbon/react";
import { loadClusterView, type ClusterView } from "../api";
import { CasesSection } from "../components/cases";
import { OperatorPanel } from "../components/OperatorPanel";
import {
  DaemonTable,
  HealthChecksTable,
  OSDTable,
  PoolTable,
  StorageDeviceTable,
} from "../components/inventoryTables";
import { ErrorState, MetricTile, PageIntro, StatusTag } from "../components/ui";
import { useOperator } from "../operator";
import { errorMessage } from "../format";
import { stoppedDaemonCount } from "../inventory";
import { agentLastSeenLabel, healthStatusLabel } from "../clusters";
import { toneForHealth } from "../tones";

// ClusterDetailPage re-founds the old single-cluster dashboard on the
// cluster-scoped reads: summary, health checks, inventory tables, and the
// cluster's Cases — all keyed by the FSID in the route.
export function ClusterDetailPage() {
  const { fsid } = useParams();
  const [view, setView] = React.useState<ClusterView | null>(null);
  const [notFound, setNotFound] = React.useState(false);
  const [loading, setLoading] = React.useState(true);
  const [error, setError] = React.useState<string | null>(null);
  const [reloadKey, setReloadKey] = React.useState(0);
  const { operator, token, signIn, signOut } = useOperator();

  React.useEffect(() => {
    if (!fsid) {
      return;
    }
    const clusterFSID = fsid;
    const controller = new AbortController();

    async function load() {
      try {
        setLoading(true);
        setError(null);
        setNotFound(false);
        const loaded = await loadClusterView(clusterFSID, controller.signal);
        if ("notFound" in loaded) {
          setView(null);
          setNotFound(true);
        } else {
          setView(loaded);
        }
      } catch (loadError) {
        if (controller.signal.aborted) {
          return;
        }
        setError(errorMessage(loadError));
      } finally {
        if (!controller.signal.aborted) {
          setLoading(false);
        }
      }
    }

    void load();

    return () => controller.abort();
  }, [fsid, reloadKey]);

  if (!fsid) {
    return <ErrorState message="No cluster FSID in the route." />;
  }

  if (loading && !view && !notFound) {
    return <p className="atlas-empty">Loading cluster…</p>;
  }

  if (notFound) {
    return (
      <div>
        <InlineNotification
          kind="error"
          lowContrast
          title="Cluster not found"
          subtitle={`No registered cluster carries the FSID ${fsid}. It may have been deregistered, or the link is stale.`}
        />
        <Link className="atlas-table-link" to="/">
          ← Back to all clusters
        </Link>
      </div>
    );
  }

  if (error || !view) {
    return <ErrorState message={error ?? "No cluster data returned."} />;
  }

  const { cluster, health, osds, hosts, storageDevices, daemons, pools, cases } = view;
  const downOsds = osds.filter((osd) => !osd.up).length;
  const outOsds = osds.filter((osd) => !osd.in).length;
  const stoppedDaemons = stoppedDaemonCount(daemons);

  return (
    <>
      <p>
        <Link to="/" className="atlas-table-link">
          ← All clusters
        </Link>
      </p>
      <h1 className="atlas-page-title">{cluster.name}</h1>
      <PageIntro>
        <span className="atlas-mono">{cluster.fsid}</span> · {cluster.clusterType} ·{" "}
        {cluster.cephVersion ?? "Ceph version unreported"} · agent last seen{" "}
        {agentLastSeenLabel(cluster.agentLastSeen)}
      </PageIntro>

      <OperatorPanel operator={operator} onSignIn={signIn} onSignOut={signOut} />

      <Grid fullWidth className="atlas-metrics">
        <Column sm={4} md={4} lg={4}>
          <MetricTile
            label="Health"
            value={healthStatusLabel(cluster.healthStatus)}
            detail={cluster.healthSummary ?? health.summary}
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
            detail={stoppedDaemons === 0 ? "all running" : `${stoppedDaemons} stopped`}
          />
        </Column>
        <Column sm={4} md={4} lg={4}>
          <MetricTile label="Pools" value={pools.length} />
        </Column>
        <Column sm={4} md={4} lg={4}>
          <MetricTile label="Cases" value={cases.length} detail="recent updates" />
        </Column>
      </Grid>

      <section className="atlas-panel" aria-label="Health checks">
        <div className="atlas-panel-heading-row">
          <h2 className="atlas-panel-heading">Health Checks</h2>
          <StatusTag
            label={health.status.replace("HEALTH_", "")}
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

      <section className="atlas-panel" aria-label="Cases">
        <h2 className="atlas-panel-heading">Cases</h2>
        <CasesSection
          cases={cases}
          casesUnavailable={view.casesUnavailable}
          operator={operator}
          token={token}
          defaultClusterFsid={cluster.fsid ?? undefined}
          onCaseCreated={() => setReloadKey((key) => key + 1)}
        />
      </section>
    </>
  );
}

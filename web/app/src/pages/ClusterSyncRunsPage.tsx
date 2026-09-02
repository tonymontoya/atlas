import React from "react";
import { useParams } from "react-router-dom";
import { Button } from "@carbon/react";
import { listSyncRuns, resolveCluster } from "../api";
import { ClusterNotFound, ClusterPageHeader } from "../components/ClusterPageHeader";
import { SyncRunsTable } from "../components/SyncRunsTable";
import { ErrorState } from "../components/ui";
import { useResource } from "../resources";

// The cluster-scoped Sync runs section (issue #42): the observation
// runs that touched the selected cluster — Agent pushes above all.
// Runs Atlas cannot attribute to a cluster (failed pulls that never
// reached one) stay on the fleet-wide page only.
export function ClusterSyncRunsPage() {
  const { fsid } = useParams();

  const cluster = useResource(
    fsid ? (signal) => resolveCluster(fsid, signal) : null,
    [fsid],
  );
  const runs = useResource(
    fsid && cluster.data ? (signal) => listSyncRuns({ cluster: fsid }, signal) : null,
    [fsid, cluster.data],
  );

  if (!fsid) {
    return <ErrorState message="No cluster FSID in the route." />;
  }

  if (cluster.loading && !cluster.data) {
    return <p className="atlas-empty">Loading cluster…</p>;
  }

  if (cluster.error) {
    return <ErrorState message={cluster.error} />;
  }

  if (!cluster.data) {
    return <ClusterNotFound fsid={fsid} />;
  }

  return (
    <>
      <ClusterPageHeader
        fsid={fsid}
        active="Sync runs"
        title={`Sync runs · ${cluster.data.name}`}
        intro={
          <>
            Observation runs that touched{" "}
            <span className="atlas-mono">{fsid}</span> — Agent pushes above all.
          </>
        }
      />
      <Button size="sm" kind="secondary" disabled={runs.loading} onClick={runs.reload}>
        {runs.loading ? "Loading…" : "Refresh"}
      </Button>
      {runs.error ? <ErrorState message={runs.error} /> : null}
      {runs.data ? <SyncRunsTable runs={runs.data} showCluster={false} /> : null}
    </>
  );
}

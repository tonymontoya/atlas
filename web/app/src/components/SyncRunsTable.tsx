import React from "react";
import { Link } from "react-router-dom";
import type { InventorySyncRun } from "../api";
import { formatDate } from "../format";
import { toneForSyncRunStatus } from "../tones";
import { AtlasTable } from "./tables";
import { StatusTag } from "./ui";

// The one sync-run table, shared by the fleet-wide history page and the
// cluster-scoped page. The scoped view omits the cluster column — the
// route already names the cluster.
export function SyncRunsTable({
  runs,
  showCluster = true,
}: {
  runs: InventorySyncRun[];
  showCluster?: boolean;
}) {
  const columns = [
    { key: "id", header: "Run", render: (run: InventorySyncRun) => `#${run.id}` },
    { key: "provider", header: "Provider", render: (run: InventorySyncRun) => run.provider },
    {
      key: "scenario",
      header: "Scenario",
      render: (run: InventorySyncRun) => run.scenario ?? "—",
    },
    ...(showCluster
      ? [
          {
            key: "cluster",
            header: "Cluster",
            render: (run: InventorySyncRun) =>
              run.clusterFsid ? (
                <Link to={`/clusters/${run.clusterFsid}`}>
                  {run.clusterName ?? run.clusterFsid}
                </Link>
              ) : (
                "—"
              ),
          },
        ]
      : []),
    {
      key: "status",
      header: "Status",
      render: (run: InventorySyncRun) => (
        <StatusTag label={run.status} tone={toneForSyncRunStatus(run.status)} />
      ),
    },
    {
      key: "started",
      header: "Started",
      render: (run: InventorySyncRun) => (
        <time dateTime={run.startedAt}>{formatDate(run.startedAt)}</time>
      ),
    },
    {
      key: "finished",
      header: "Finished",
      render: (run: InventorySyncRun) =>
        run.finishedAt ? (
          <time dateTime={run.finishedAt}>{formatDate(run.finishedAt)}</time>
        ) : (
          "—"
        ),
    },
    {
      key: "error",
      header: "Error",
      render: (run: InventorySyncRun) =>
        run.errorMessage ? (
          <span title={run.errorClass ?? undefined}>{run.errorMessage}</span>
        ) : (
          "—"
        ),
    },
  ];
  return (
    <AtlasTable
      columns={columns}
      rows={runs}
      rowKey={(run: InventorySyncRun) => String(run.id)}
      emptyLabel="No sync runs recorded."
    />
  );
}

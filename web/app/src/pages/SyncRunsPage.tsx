import React from "react";
import { Button } from "@carbon/react";
import { listSyncRuns } from "../api";
import { formatDate } from "../format";
import { toneForSyncRunStatus } from "../tones";
import { useResource } from "../resources";
import { AtlasTable } from "../components/tables";
import { ErrorState, PageIntro, StatusTag } from "../components/ui";

// SyncRunsPage promotes the old dashboard's three-run panel into the full
// inventory sync run history (control-plane pull and agent push runs).
export function SyncRunsPage() {
  const runs = useResource((signal) => listSyncRuns(signal), []);

  return (
    <>
      <h1 className="atlas-page-title">Inventory Sync Runs</h1>
      <PageIntro>
        Recent observation runs — control-plane pulls and Agent pushes alike — with
        their provider, scenario, and outcome.
      </PageIntro>
      <Button size="sm" kind="secondary" disabled={runs.loading} onClick={runs.reload}>
        {runs.loading ? "Loading…" : "Refresh"}
      </Button>
      {runs.error ? <ErrorState message={runs.error} /> : null}
      {runs.data ? (
        <AtlasTable
          columns={[
            { key: "id", header: "Run", render: (run) => `#${run.id}` },
            { key: "provider", header: "Provider", render: (run) => run.provider },
            {
              key: "scenario",
              header: "Scenario",
              render: (run) => run.scenario ?? "—",
            },
            {
              key: "status",
              header: "Status",
              render: (run) => (
                <StatusTag label={run.status} tone={toneForSyncRunStatus(run.status)} />
              ),
            },
            {
              key: "started",
              header: "Started",
              render: (run) => <time dateTime={run.startedAt}>{formatDate(run.startedAt)}</time>,
            },
            {
              key: "finished",
              header: "Finished",
              render: (run) =>
                run.finishedAt ? (
                  <time dateTime={run.finishedAt}>{formatDate(run.finishedAt)}</time>
                ) : (
                  "—"
                ),
            },
            {
              key: "error",
              header: "Error",
              render: (run) =>
                run.errorMessage ? (
                  <span title={run.errorClass ?? undefined}>{run.errorMessage}</span>
                ) : (
                  "—"
                ),
            },
          ]}
          rows={runs.data}
          rowKey={(run) => String(run.id)}
          emptyLabel="No sync runs recorded."
        />
      ) : null}
    </>
  );
}

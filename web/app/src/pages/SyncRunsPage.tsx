import React from "react";
import { Button } from "@carbon/react";
import { listSyncRuns, type InventorySyncRun } from "../api";
import { formatDate, errorMessage } from "../format";
import { toneForSyncRunStatus } from "../tones";
import { AtlasTable } from "../components/tables";
import { ErrorState, PageIntro, StatusTag } from "../components/ui";

// SyncRunsPage promotes the old dashboard's three-run panel into the full
// inventory sync run history (control-plane pull and agent push runs).
export function SyncRunsPage() {
  const [runs, setRuns] = React.useState<InventorySyncRun[] | null>(null);
  const [error, setError] = React.useState<string | null>(null);
  const [loading, setLoading] = React.useState(true);
  const [reloadKey, setReloadKey] = React.useState(0);

  React.useEffect(() => {
    const controller = new AbortController();

    async function load() {
      try {
        setLoading(true);
        setError(null);
        const loaded = await listSyncRuns(controller.signal);
        setRuns(loaded);
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
  }, [reloadKey]);

  return (
    <>
      <h1 className="atlas-page-title">Inventory Sync Runs</h1>
      <PageIntro>
        Recent observation runs — control-plane pulls and Agent pushes alike — with
        their provider, scenario, and outcome.
      </PageIntro>
      <Button size="sm" kind="secondary" disabled={loading} onClick={() => setReloadKey((key) => key + 1)}>
        {loading ? "Loading…" : "Refresh"}
      </Button>
      {error ? <ErrorState message={error} /> : null}
      {runs ? (
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
          rows={runs}
          rowKey={(run) => String(run.id)}
          emptyLabel="No sync runs recorded."
        />
      ) : null}
    </>
  );
}

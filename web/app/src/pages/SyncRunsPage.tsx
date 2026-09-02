import React from "react";
import { Button } from "@carbon/react";
import { listSyncRuns } from "../api";
import { SyncRunsTable } from "../components/SyncRunsTable";
import { ErrorState, PageIntro } from "../components/ui";
import { useResource } from "../resources";

// SyncRunsPage promotes the old dashboard's three-run panel into the
// full inventory sync run history (control-plane pull and agent push
// runs) across every registered cluster. Per-cluster history lives on
// the cluster's own Sync runs tab.
export function SyncRunsPage() {
  const runs = useResource((signal) => listSyncRuns({}, signal), []);

  return (
    <>
      <h1 className="atlas-page-title">Inventory Sync Runs</h1>
      <PageIntro>
        Recent observation runs across every registered cluster — control-plane
        pulls and Agent pushes alike — with their provider, scenario, and outcome.
      </PageIntro>
      <Button size="sm" kind="secondary" disabled={runs.loading} onClick={runs.reload}>
        {runs.loading ? "Loading…" : "Refresh"}
      </Button>
      {runs.error ? <ErrorState message={runs.error} /> : null}
      {runs.data ? <SyncRunsTable runs={runs.data} /> : null}
    </>
  );
}

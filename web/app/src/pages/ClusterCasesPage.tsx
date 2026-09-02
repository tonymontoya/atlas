import React from "react";
import { useParams } from "react-router-dom";
import { listCases, resolveCluster } from "../api";
import { CasesSection } from "../components/cases";
import { ClusterNotFound, ClusterPageHeader } from "../components/ClusterPageHeader";
import { OperatorPanel } from "../components/OperatorPanel";
import { ErrorState } from "../components/ui";
import { useOperator } from "../operator";
import { useResource } from "../resources";

// The cluster-scoped Cases section (issue #42): every Case bound to the
// selected cluster, with the same read and write surfaces as the
// fleet-wide page. The FSID in the route is the only cluster source.
export function ClusterCasesPage() {
  const { fsid } = useParams();
  const { operator, token, signIn, signOut } = useOperator();

  const cluster = useResource(
    fsid ? (signal) => resolveCluster(fsid, signal) : null,
    [fsid],
  );
  const cases = useResource(
    fsid && cluster.data ? (signal) => listCases({ cluster: fsid }, signal) : null,
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
        active="Cases"
        title={`Cases · ${cluster.data.name}`}
        intro={
          <>
            Cases bound to <span className="atlas-mono">{fsid}</span>. Manual writes
            need an operator token.
          </>
        }
      />
      <OperatorPanel operator={operator} onSignIn={signIn} onSignOut={signOut} />
      {cases.loading && !cases.data ? (
        <p className="atlas-empty">Loading cases…</p>
      ) : null}
      {cases.error ? <ErrorState message={cases.error} /> : null}
      {cases.data || cases.error ? (
        <CasesSection
          cases={cases.data ?? []}
          operator={operator}
          token={token}
          defaultClusterFsid={cluster.data.fsid ?? undefined}
          onCaseCreated={cases.reload}
          onCasesChanged={cases.reload}
        />
      ) : null}
    </>
  );
}

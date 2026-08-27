import React from "react";
import { listCases } from "../api";
import { CasesSection } from "../components/cases";
import { OperatorPanel } from "../components/OperatorPanel";
import { ErrorState, PageIntro } from "../components/ui";
import { useResource } from "../resources";
import { useOperator } from "../operator";

export function CasesPage() {
  const cases = useResource((signal) => listCases({}, signal), []);
  const { operator, token, signIn, signOut } = useOperator();

  return (
    <>
      <h1 className="atlas-page-title">Cases</h1>
      <PageIntro>
        The most recent Cases across every registered cluster. Manual writes need an
        operator token.
      </PageIntro>
      <OperatorPanel operator={operator} onSignIn={signIn} onSignOut={signOut} />
      {cases.loading && !cases.data ? <p className="atlas-empty">Loading cases…</p> : null}
      {cases.error ? <ErrorState message={cases.error} /> : null}
      {cases.data || cases.error ? (
        <CasesSection
          cases={cases.data ?? []}
          operator={operator}
          token={token}
          onCaseCreated={cases.reload}
          onCasesChanged={cases.reload}
        />
      ) : null}
    </>
  );
}

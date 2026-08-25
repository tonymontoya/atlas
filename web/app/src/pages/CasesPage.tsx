import React from "react";
import { listCases, type CaseRecord } from "../api";
import { CasesSection } from "../components/cases";
import { OperatorPanel } from "../components/OperatorPanel";
import { ErrorState, PageIntro } from "../components/ui";
import { errorMessage } from "../format";
import { useOperator } from "../operator";

// CasesPage keeps the global Case list (most recent updates first) with
// the full detail surface: transitions, assignment, notes, workflows.
export function CasesPage() {
  const [cases, setCases] = React.useState<CaseRecord[] | null>(null);
  const [casesError, setCasesError] = React.useState<string | null>(null);
  const [loading, setLoading] = React.useState(true);
  const [reloadKey, setReloadKey] = React.useState(0);
  const { operator, token, signIn, signOut } = useOperator();

  React.useEffect(() => {
    const controller = new AbortController();

    async function load() {
      try {
        setLoading(true);
        setCasesError(null);
        const loaded = await listCases({}, controller.signal);
        setCases(loaded);
      } catch (loadError) {
        if (controller.signal.aborted) {
          return;
        }
        setCasesError(errorMessage(loadError));
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
      <h1 className="atlas-page-title">Cases</h1>
      <PageIntro>
        The most recent Cases across every registered cluster. Manual writes need an
        operator token.
      </PageIntro>
      <OperatorPanel operator={operator} onSignIn={signIn} onSignOut={signOut} />
      {loading && !cases ? <p className="atlas-empty">Loading cases…</p> : null}
      {casesError ? <ErrorState message={casesError} /> : null}
      {cases || casesError ? (
        <CasesSection
          cases={cases ?? []}
          operator={operator}
          token={token}
          onCaseCreated={() => setReloadKey((key) => key + 1)}
        />
      ) : null}
    </>
  );
}

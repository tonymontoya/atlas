import React from "react";
import { Link } from "react-router-dom";
import { InlineNotification } from "@carbon/react";
import type { CaseRecord, Operator } from "../../api";
import { formatDate } from "../../format";
import { toneForCaseSeverity, toneForCaseStatus } from "../../tones";
import { useCaseDetail } from "../../useCaseDetail";
import { AtlasTable } from "../tables";
import { EmptyState, StatusTag } from "../ui";
import { CaseComposePanel } from "./CaseComposePanel";
import { CaseDetailPanel } from "./CaseDetailPanel";

// The Cases UI: one file per panel along the section's internal seams.
// index holds the list surface (CasesSection) and composes the panels.

export function CasesSection({
  cases,
  casesUnavailable,
  operator,
  token,
  defaultClusterFsid,
  onCaseCreated,
  onCasesChanged,
}: {
  cases: CaseRecord[];
  casesUnavailable?: string;
  operator: Operator | null;
  token: string | null;
  defaultClusterFsid?: string;
  onCaseCreated?: (created: CaseRecord) => void;
  onCasesChanged?: () => void;
}) {
  const [selectedCaseID, setSelectedCaseID] = React.useState<number | null>(null);
  const [reloadKey, setReloadKey] = React.useState(0);
  const detailState = useCaseDetail(selectedCaseID, reloadKey);

  const refresh = React.useCallback(() => {
    setReloadKey((key) => key + 1);
    onCasesChanged?.();
  }, [onCasesChanged]);

  return (
    <>
      {operator && token ? (
        <CaseComposePanel
          token={token}
          defaultClusterFsid={defaultClusterFsid}
          onCreated={(created) => {
            setSelectedCaseID(created.id);
            onCaseCreated?.(created);
          }}
        />
      ) : null}

      <section aria-label="Cases">
        {casesUnavailable ? (
          <InlineNotification
            kind="warning"
            lowContrast
            title="Cases unavailable"
            subtitle={casesUnavailable}
          />
        ) : cases.length === 0 ? (
          <EmptyState label="No cases recorded." />
        ) : (
          <AtlasTable
            columns={[
              {
                key: "title",
                header: "Title",
                render: (item) => (
                  <button
                    type="button"
                    className={
                      item.id === selectedCaseID
                        ? "atlas-case-link atlas-case-link-selected"
                        : "atlas-case-link"
                    }
                    onClick={() => setSelectedCaseID(item.id)}
                  >
                    {item.title}
                  </button>
                ),
              },
              {
                key: "severity",
                header: "Severity",
                render: (item) => (
                  <StatusTag label={item.severity} tone={toneForCaseSeverity(item.severity)} />
                ),
              },
              {
                key: "status",
                header: "Status",
                render: (item) => (
                  <StatusTag label={item.status} tone={toneForCaseStatus(item.status)} />
                ),
              },
              { key: "source", header: "Source", render: (item) => item.source },
              {
                key: "cluster",
                header: "Cluster",
                render: (item) => <CaseClusterLink fsid={item.clusterFsid} />,
              },
              {
                key: "assignee",
                header: "Assignee",
                render: (item) => item.assigneeDisplayName ?? "—",
              },
              {
                key: "updated",
                header: "Updated",
                render: (item) => (
                  <time dateTime={item.updatedAt}>{formatDate(item.updatedAt)}</time>
                ),
              },
            ]}
            rows={cases}
            rowKey={(item) => String(item.id)}
            emptyLabel="No cases recorded."
          />
        )}
      </section>

      {selectedCaseID !== null ? (
        <CaseDetailPanel
          state={detailState}
          operator={operator}
          token={token}
          onChanged={refresh}
          onClose={() => setSelectedCaseID(null)}
        />
      ) : null}
    </>
  );
}

// CaseClusterLink navigates a Case's cluster column when the Case is
// bound; unbound Cases stay plain text.
export function CaseClusterLink({ fsid }: { fsid: string | undefined }) {
  if (!fsid) {
    return <>—</>;
  }
  return <Link to={`/clusters/${fsid}`}>{fsid}</Link>;
}

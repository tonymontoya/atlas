import React from "react";
import { Button, Column, Grid, InlineLoading, InlineNotification } from "@carbon/react";
import type { Operator } from "../../api";
import { availableCaseActions } from "../../caseActions";
import { formatDate } from "../../format";
import { detectionLabel } from "../../timeline";
import { toneForCaseSeverity, toneForCaseStatus } from "../../tones";
import type { CaseDetailState } from "../../useCaseDetail";
import { StatusTag } from "../ui";
import { CaseActions } from "./CaseActions";
import { CaseNotes } from "./CaseNotes";
import { CaseTimeline } from "./CaseTimeline";
import { CaseWorkflows } from "./CaseWorkflows";

export function CaseDetailPanel({
  state,
  operator,
  token,
  onChanged,
  onClose,
}: {
  state: CaseDetailState;
  operator: Operator | null;
  token: string | null;
  onChanged: () => void;
  onClose: () => void;
}) {
  const { detail, detailError, detailLoading } = state;

  return (
    <section className="atlas-panel" aria-label="Case detail">
      <div className="atlas-panel-heading-row">
        <h2 className="atlas-panel-heading">Case Detail</h2>
        <Button size="sm" kind="ghost" onClick={onClose}>
          Close
        </Button>
      </div>
      {detailLoading ? <InlineLoading description="Loading case detail…" /> : null}
      {detailError ? (
        <InlineNotification
          kind="error"
          lowContrast
          title="Case unavailable"
          subtitle={detailError}
        />
      ) : null}
      {!detailLoading && !detailError && detail ? (
        <div className="atlas-case-detail">
          <div className="atlas-case-detail-header">
            <div>
              <h3>{detail.title}</h3>
              <p>{detail.summary}</p>
            </div>
            <div className="atlas-case-meta">
              <StatusTag
                label={detail.severity}
                tone={toneForCaseSeverity(detail.severity)}
              />
              <StatusTag label={detail.status} tone={toneForCaseStatus(detail.status)} />
              <span>{detail.source}</span>
            </div>
          </div>
          {detail.detectedBy ? (
            <p className="atlas-detection">{detectionLabel(detail.detectedBy)}</p>
          ) : null}
          <Grid fullWidth className="atlas-detail-grid">
            <DetailField label="Case ID" value={`#${detail.id}`} />
            <DetailField label="Cluster FSID" value={detail.clusterFsid ?? "unassigned"} />
            <DetailField
              label="Assignee"
              value={detail.assigneeDisplayName ?? detail.assignee ?? "unassigned"}
            />
            <DetailField label="Created" value={formatDate(detail.createdAt)} />
            <DetailField label="Updated" value={formatDate(detail.updatedAt)} />
            <DetailField
              label="Closed"
              value={detail.closedAt ? formatDate(detail.closedAt) : "open"}
            />
            {detail.detectedBy ? (
              <DetailField
                label="Alert first seen"
                value={formatDate(detail.detectedBy.firstSeenAt)}
              />
            ) : null}
          </Grid>
          {operator && token ? (
            <CaseActions detail={detail} token={token} onChanged={onChanged} />
          ) : null}
          <CaseWorkflows
            caseID={detail.id}
            error={state.workflowsError}
            loading={state.workflowsLoading}
            workflows={state.workflows}
            workflowJobs={state.workflowJobs}
            canAttach={
              operator !== null &&
              token !== null &&
              availableCaseActions(detail.status).canAttachWorkflow
            }
            token={token}
            onChanged={onChanged}
          />
          <CaseNotes
            error={state.notesError}
            loading={state.notesLoading}
            notes={state.notes}
            operator={operator}
            token={token}
            caseID={detail.id}
            onChanged={onChanged}
          />
          <CaseTimeline
            error={state.timelineError}
            loading={state.timelineLoading}
            timeline={state.timeline}
          />
        </div>
      ) : null}
    </section>
  );
}

function DetailField({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <Column sm={2} md={2} lg={3}>
      <p className="atlas-metric-label">{label}</p>
      <p className="atlas-detail-value">{value}</p>
    </Column>
  );
}

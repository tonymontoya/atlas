import React from "react";
import { Link } from "react-router-dom";
import {
  Button,
  Column,
  Grid,
  InlineLoading,
  InlineNotification,
  Select,
  SelectItem,
  TextArea,
  TextInput,
} from "@carbon/react";
import {
  addCaseNote,
  approveWorkflowGate,
  assignCase,
  attachCaseWorkflow,
  completeWorkflowTask,
  createCase,
  transitionCase,
  type CaseNote,
  type CaseRecord,
  type Operator,
  type TimelineEvent,
  type WorkflowInstanceRecord,
  type WorkflowJobRecord,
} from "../api";
import { availableCaseActions } from "../caseActions";
import { formatDate, errorMessage } from "../format";
import { detectionLabel, labelForTimelineEventType, timelinePayloadLabels } from "../timeline";
import {
  toneForCaseSeverity,
  toneForCaseStatus,
  toneForJobState,
  toneForWorkflowState,
} from "../tones";
import { useCaseDetail } from "../useCaseDetail";
import type { CaseDetailState } from "../useCaseDetail";
import { AtlasTable } from "./tables";
import { EmptyState, StatusTag } from "./ui";

const SEVERITIES: CaseRecord["severity"][] = ["info", "low", "medium", "high", "critical"];

// CasesSection owns Case selection for one page: the list, the manual
// compose form (when signed in), and the shared detail panel.
export function CasesSection({
  cases,
  casesUnavailable,
  operator,
  token,
  defaultClusterFsid,
  onCaseCreated,
}: {
  cases: CaseRecord[];
  casesUnavailable?: string;
  operator: Operator | null;
  token: string | null;
  defaultClusterFsid?: string;
  onCaseCreated?: (created: CaseRecord) => void;
}) {
  const [selectedCaseID, setSelectedCaseID] = React.useState<number | null>(null);
  const [reloadKey, setReloadKey] = React.useState(0);
  const detailState = useCaseDetail(selectedCaseID, reloadKey);

  const refresh = React.useCallback(() => {
    setReloadKey((key) => key + 1);
  }, []);

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

function CaseComposePanel({
  token,
  defaultClusterFsid,
  onCreated,
}: {
  token: string;
  defaultClusterFsid?: string;
  onCreated: (created: CaseRecord) => void;
}) {
  const [title, setTitle] = React.useState("");
  const [summary, setSummary] = React.useState("");
  const [severity, setSeverity] = React.useState<CaseRecord["severity"]>("medium");
  const [clusterFsid, setClusterFsid] = React.useState(defaultClusterFsid ?? "");
  const [busy, setBusy] = React.useState(false);
  const [composeError, setComposeError] = React.useState<string | null>(null);

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    if (busy || title.trim() === "" || summary.trim() === "") {
      return;
    }
    try {
      setBusy(true);
      setComposeError(null);
      const created = await createCase(
        {
          title: title.trim(),
          summary: summary.trim(),
          severity,
          clusterFsid: clusterFsid.trim() === "" ? undefined : clusterFsid.trim(),
        },
        token,
      );
      setTitle("");
      setSummary("");
      setSeverity("medium");
      setClusterFsid(defaultClusterFsid ?? "");
      onCreated(created);
    } catch (submitError) {
      setComposeError(errorMessage(submitError));
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="atlas-panel" aria-label="Create a manual case">
      <h2 className="atlas-panel-heading">New Manual Case</h2>
      <form onSubmit={submit}>
        <Grid fullWidth>
          <Column sm={4} md={4} lg={6}>
            <TextInput
              id="case-title"
              labelText="Title"
              placeholder="Manual review of slow OSD warnings"
              value={title}
              onChange={(event) => setTitle(event.target.value)}
            />
          </Column>
          <Column sm={4} md={4} lg={4}>
            <Select
              id="case-severity"
              labelText="Severity"
              value={severity}
              onChange={(event) => setSeverity(event.target.value as CaseRecord["severity"])}
            >
              {SEVERITIES.map((value) => (
                <SelectItem key={value} value={value} text={value} />
              ))}
            </Select>
          </Column>
          <Column sm={4} md={8} lg={10}>
            <TextArea
              id="case-summary"
              labelText="Summary"
              rows={2}
              placeholder="What the operator observed and why this needs a case."
              value={summary}
              onChange={(event) => setSummary(event.target.value)}
            />
          </Column>
          <Column sm={4} md={8} lg={10}>
            <TextInput
              id="case-cluster-fsid"
              labelText="Cluster FSID (optional)"
              placeholder="00000000-0000-4000-8000-000000000101"
              value={clusterFsid}
              onChange={(event) => setClusterFsid(event.target.value)}
            />
          </Column>
        </Grid>
        <div className="atlas-action-row">
          <Button type="submit" disabled={busy || title.trim() === "" || summary.trim() === ""}>
            {busy ? "Creating…" : "Create case"}
          </Button>
          {composeError ? <p className="atlas-form-error">{composeError}</p> : null}
        </div>
      </form>
    </section>
  );
}

function CaseDetailPanel({
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

function CaseActions({
  detail,
  token,
  onChanged,
}: {
  detail: CaseRecord;
  token: string;
  onChanged: () => void;
}) {
  const [busy, setBusy] = React.useState(false);
  const [actionError, setActionError] = React.useState<string | null>(null);
  const [assignee, setAssignee] = React.useState("");
  const [assigneeName, setAssigneeName] = React.useState("");

  async function run(action: () => Promise<unknown>) {
    if (busy) {
      return;
    }
    try {
      setBusy(true);
      setActionError(null);
      await action();
      onChanged();
    } catch (actionThrown) {
      setActionError(errorMessage(actionThrown));
    } finally {
      setBusy(false);
    }
  }

  const actions = availableCaseActions(detail.status);
  const assignFormReady = assignee.trim() !== "" && assigneeName.trim() !== "";

  return (
    <div className="atlas-case-actions">
      <div className="atlas-action-row">
        {actions.canTriage ? (
          <Button
            size="sm"
            disabled={busy}
            onClick={() => run(() => transitionCase(detail.id, "triaged", token))}
          >
            Triage
          </Button>
        ) : null}
        {actions.canClose ? (
          <Button
            size="sm"
            kind="danger"
            disabled={busy}
            onClick={() => run(() => transitionCase(detail.id, "closed", token))}
          >
            Close case
          </Button>
        ) : null}
        {actions.isClosed ? (
          <p className="atlas-subtle">
            Closed. Reopening means creating a new case for a recurring condition.
          </p>
        ) : (
          <div className="atlas-inline-form">
            <TextInput
              id="assignee-subject"
              hideLabel
              labelText="Assignee subject"
              placeholder="Assignee subject"
              value={assignee}
              onChange={(event) => setAssignee(event.target.value)}
            />
            <TextInput
              id="assignee-name"
              hideLabel
              labelText="Assignee display name"
              placeholder="Assignee display name"
              value={assigneeName}
              onChange={(event) => setAssigneeName(event.target.value)}
            />
            <Button
              size="sm"
              kind="secondary"
              disabled={busy || !assignFormReady}
              onClick={() =>
                run(async () => {
                  await assignCase(detail.id, assignee.trim(), assigneeName.trim(), token);
                  setAssignee("");
                  setAssigneeName("");
                })
              }
            >
              Assign
            </Button>
            <Button
              size="sm"
              kind="ghost"
              disabled={busy || !detail.assignee}
              onClick={() => run(() => assignCase(detail.id, "", "", token))}
            >
              Unassign
            </Button>
          </div>
        )}
      </div>
      {actionError ? <p className="atlas-form-error">{actionError}</p> : null}
    </div>
  );
}

function CaseWorkflows({
  caseID,
  error,
  loading,
  workflows,
  workflowJobs,
  canAttach,
  token,
  onChanged,
}: {
  caseID: number;
  error: string | null;
  loading: boolean;
  workflows: WorkflowInstanceRecord[];
  workflowJobs: Record<number, WorkflowJobRecord[]>;
  canAttach: boolean;
  token: string | null;
  onChanged: () => void;
}) {
  const [workflowID, setWorkflowID] = React.useState("replace-osd");
  const [workflowVersion, setWorkflowVersion] = React.useState("1");
  const [busy, setBusy] = React.useState(false);
  const [attachError, setAttachError] = React.useState<string | null>(null);

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    const version = Number.parseInt(workflowVersion, 10);
    if (busy || workflowID.trim() === "" || !Number.isInteger(version) || version < 1 || !token) {
      return;
    }
    try {
      setBusy(true);
      setAttachError(null);
      await attachCaseWorkflow(caseID, workflowID.trim(), version, token);
      onChanged();
    } catch (thrown) {
      setAttachError(errorMessage(thrown));
    } finally {
      setBusy(false);
    }
  }

  const formReady = workflowID.trim() !== "" && Number.parseInt(workflowVersion, 10) >= 1;

  return (
    <section className="atlas-case-subsection" aria-label="Workflows">
      <h3>
        Workflows{" "}
        {!loading && !error ? <span className="atlas-count">{workflows.length}</span> : null}
      </h3>
      {loading ? <EmptyState label="Loading Workflow Instances." /> : null}
      {error ? <EmptyState label={`Workflow Instances unavailable: ${error}`} /> : null}
      {!loading && !error && workflows.length === 0 ? (
        <EmptyState label="No Workflow Instances attached." />
      ) : null}
      {!loading && !error && workflows.length > 0 ? (
        <ul className="atlas-workflow-list">
          {workflows.map((instance) => (
            <li key={instance.id} className="atlas-workflow-row">
              <div className="atlas-workflow-heading">
                <strong>
                  {instance.workflowId} v{instance.workflowVersion}
                </strong>
                <StatusTag
                  label={instance.state.replace(/_/g, " ")}
                  tone={toneForWorkflowState(instance.state)}
                />
              </div>
              <p className="atlas-subtle">
                #{instance.id}
                {instance.currentStep ? ` · paused at ${instance.currentStep}` : ""}
                {` · updated ${formatDate(instance.updatedAt)}`}
              </p>
              {workflowJobs[instance.id]?.length ? (
                <ul className="atlas-job-list">
                  {workflowJobs[instance.id].map((job) => (
                    <li key={job.id} className="atlas-job-row">
                      <span>{job.stepId}</span>
                      <span>{job.operationType}</span>
                      <StatusTag label={job.state} tone={toneForJobState(job.state)} />
                      {job.maxAttempts > 1 ? (
                        <span className="atlas-subtle">{`attempt ${job.attempt}/${job.maxAttempts}`}</span>
                      ) : null}
                    </li>
                  ))}
                </ul>
              ) : null}
              {instance.state === "waiting_for_approval" && instance.currentStep && token ? (
                <WorkflowApproveForm
                  gateID={instance.currentStep}
                  instanceID={instance.id}
                  token={token}
                  onChanged={onChanged}
                />
              ) : null}
              {instance.state === "waiting_for_operator" && instance.currentStep && token ? (
                <WorkflowResumeForm
                  taskID={instance.currentStep}
                  instanceID={instance.id}
                  token={token}
                  onChanged={onChanged}
                />
              ) : null}
            </li>
          ))}
        </ul>
      ) : null}
      {canAttach ? (
        <form onSubmit={submit} className="atlas-inline-form">
          <TextInput
            id="workflow-id"
            hideLabel
            labelText="Workflow id"
            placeholder="Workflow id, e.g. replace-osd"
            value={workflowID}
            onChange={(event) => setWorkflowID(event.target.value)}
          />
          <TextInput
            id="workflow-version"
            hideLabel
            labelText="Version"
            placeholder="Version"
            value={workflowVersion}
            onChange={(event) => setWorkflowVersion(event.target.value)}
          />
          <Button type="submit" size="sm" kind="secondary" disabled={busy || !formReady}>
            {busy ? "Attaching…" : "Attach workflow"}
          </Button>
          {attachError ? <p className="atlas-form-error">{attachError}</p> : null}
        </form>
      ) : null}
    </section>
  );
}

function WorkflowApproveForm({
  gateID,
  instanceID,
  token,
  onChanged,
}: {
  gateID: string;
  instanceID: number;
  token: string;
  onChanged: () => void;
}) {
  const [reason, setReason] = React.useState("");
  const [busy, setBusy] = React.useState(false);
  const [approveError, setApproveError] = React.useState<string | null>(null);

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    if (busy) {
      return;
    }
    try {
      setBusy(true);
      setApproveError(null);
      await approveWorkflowGate(instanceID, gateID, reason.trim(), token);
      setReason("");
      onChanged();
    } catch (thrown) {
      setApproveError(errorMessage(thrown));
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={submit} className="atlas-inline-form">
      <TextInput
        id={`approve-reason-${instanceID}`}
        hideLabel
        labelText="Approval reason"
        placeholder="Reason (optional)"
        value={reason}
        onChange={(event) => setReason(event.target.value)}
      />
      <Button type="submit" size="sm">
        {busy ? "Approving…" : `Approve ${gateID}`}
      </Button>
      {approveError ? <p className="atlas-form-error">{approveError}</p> : null}
    </form>
  );
}

function WorkflowResumeForm({
  taskID,
  instanceID,
  token,
  onChanged,
}: {
  taskID: string;
  instanceID: number;
  token: string;
  onChanged: () => void;
}) {
  const [note, setNote] = React.useState("");
  const [busy, setBusy] = React.useState(false);
  const [resumeError, setResumeError] = React.useState<string | null>(null);

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    if (busy) {
      return;
    }
    try {
      setBusy(true);
      setResumeError(null);
      await completeWorkflowTask(instanceID, taskID, note.trim(), token);
      setNote("");
      onChanged();
    } catch (thrown) {
      setResumeError(errorMessage(thrown));
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={submit} className="atlas-inline-form">
      <TextInput
        id={`task-note-${instanceID}`}
        hideLabel
        labelText="Task completion note"
        placeholder="Note (optional)"
        value={note}
        onChange={(event) => setNote(event.target.value)}
      />
      <Button type="submit" size="sm">
        {busy ? "Resuming…" : `Done: ${taskID}`}
      </Button>
      {resumeError ? <p className="atlas-form-error">{resumeError}</p> : null}
    </form>
  );
}

function CaseNotes({
  caseID,
  error,
  loading,
  notes,
  operator,
  token,
  onChanged,
}: {
  caseID: number;
  error: string | null;
  loading: boolean;
  notes: CaseNote[];
  operator: Operator | null;
  token: string | null;
  onChanged: () => void;
}) {
  const [body, setBody] = React.useState("");
  const [busy, setBusy] = React.useState(false);
  const [noteError, setNoteError] = React.useState<string | null>(null);

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    if (busy || body.trim() === "" || !token) {
      return;
    }
    try {
      setBusy(true);
      setNoteError(null);
      await addCaseNote(caseID, body.trim(), token);
      setBody("");
      onChanged();
    } catch (submitError) {
      setNoteError(errorMessage(submitError));
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="atlas-case-subsection" aria-label="Case Notes">
      <h3>
        Notes {!loading && !error ? <span className="atlas-count">{notes.length}</span> : null}
      </h3>
      {loading ? <EmptyState label="Loading Case Notes." /> : null}
      {error ? <EmptyState label={`Case Notes unavailable: ${error}`} /> : null}
      {!loading && !error && notes.length === 0 ? (
        <EmptyState label="No Case Notes recorded." />
      ) : null}
      {!loading && !error && notes.length > 0 ? (
        <ul className="atlas-note-list">
          {notes.map((note) => (
            <li key={note.id} className="atlas-note-row">
              <div className="atlas-note-heading">
                <strong>{note.authorDisplayName}</strong>
                <time dateTime={note.createdAt}>{formatDate(note.createdAt)}</time>
              </div>
              <p>{note.body}</p>
            </li>
          ))}
        </ul>
      ) : null}
      {operator && token ? (
        <form onSubmit={submit}>
          <TextArea
            id="note-body"
            labelText="Add a note"
            rows={2}
            placeholder="Add a note about investigation progress."
            value={body}
            onChange={(event) => setBody(event.target.value)}
          />
          <div className="atlas-action-row">
            <Button type="submit" size="sm" kind="secondary" disabled={busy || body.trim() === ""}>
              {busy ? "Adding…" : "Add note"}
            </Button>
            {noteError ? <p className="atlas-form-error">{noteError}</p> : null}
          </div>
        </form>
      ) : null}
    </section>
  );
}

function CaseTimeline({
  error,
  loading,
  timeline,
}: {
  error: string | null;
  loading: boolean;
  timeline: TimelineEvent[];
}) {
  return (
    <section className="atlas-case-subsection" aria-label="Case Timeline Events">
      <h3>
        Timeline Events{" "}
        {!loading && !error ? <span className="atlas-count">{timeline.length}</span> : null}
      </h3>
      {loading ? <EmptyState label="Loading Timeline Events." /> : null}
      {error ? <EmptyState label={`Timeline Events unavailable: ${error}`} /> : null}
      {!loading && !error && timeline.length === 0 ? (
        <EmptyState label="No Timeline Events recorded." />
      ) : null}
      {!loading && !error && timeline.length > 0 ? (
        <ol className="atlas-timeline">
          {timeline.map((event) => (
            <TimelineEventRow key={event.id} event={event} />
          ))}
        </ol>
      ) : null}
    </section>
  );
}

function TimelineEventRow({ event }: { event: TimelineEvent }) {
  const payloadLabels = timelinePayloadLabels(event);

  return (
    <li className="atlas-timeline-event">
      <div className="atlas-timeline-marker" aria-hidden="true" />
      <div className="atlas-timeline-body">
        <div className="atlas-timeline-heading">
          <strong>{labelForTimelineEventType(event.type)}</strong>
          <time dateTime={event.occurredAt}>{formatDate(event.occurredAt)}</time>
        </div>
        <p>{event.message}</p>
        <div className="atlas-timeline-meta">
          <span>{event.actor.displayName}</span>
          <span>{event.actor.type.replace("_", " ")}</span>
          {payloadLabels.map((label) => (
            <span key={label}>{label}</span>
          ))}
        </div>
      </div>
    </li>
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

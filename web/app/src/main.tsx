import React from "react";
import { createRoot } from "react-dom/client";
import {
  addCaseNote,
  approveWorkflowGate,
  assignCase,
  attachCaseWorkflow,
  completeWorkflowTask,
  createCase,
  loadCase,
  loadCaseNotes,
  loadCaseTimeline,
  loadCaseWorkflows,
  loadDashboard,
  loadMe,
  loadWorkflowJobs,
  transitionCase,
  type CaseNote,
  type CaseRecord,
  type Daemon,
  type DashboardSnapshot,
  type InventorySyncRun,
  type Operator,
  type OSD,
  type Pool,
  type StorageDevice,
  type TimelineEvent,
  type WorkflowInstanceRecord,
  type WorkflowJobRecord,
} from "./api";
import { poolRedundancyLabel, stoppedDaemonCount, storageDeviceOSDLabel } from "./inventory";
import { availableCaseActions } from "./caseActions";
import { detectionLabel, labelForTimelineEventType, timelinePayloadLabels } from "./timeline";
import "./styles.css";

const SEVERITIES: CaseRecord["severity"][] = ["info", "low", "medium", "high", "critical"];

function App() {
  const [snapshot, setSnapshot] = React.useState<DashboardSnapshot | null>(null);
  const [error, setError] = React.useState<string | null>(null);
  const [loading, setLoading] = React.useState(true);
  const [reloadKey, setReloadKey] = React.useState(0);
  const [selectedCaseID, setSelectedCaseID] = React.useState<number | null>(null);
  const [caseDetail, setCaseDetail] = React.useState<CaseRecord | null>(null);
  const [caseDetailError, setCaseDetailError] = React.useState<string | null>(null);
  const [caseDetailLoading, setCaseDetailLoading] = React.useState(false);
  const [caseTimeline, setCaseTimeline] = React.useState<TimelineEvent[]>([]);
  const [caseTimelineError, setCaseTimelineError] = React.useState<string | null>(null);
  const [caseTimelineLoading, setCaseTimelineLoading] = React.useState(false);
  const [caseNotes, setCaseNotes] = React.useState<CaseNote[]>([]);
  const [caseNotesError, setCaseNotesError] = React.useState<string | null>(null);
  const [caseNotesLoading, setCaseNotesLoading] = React.useState(false);
  const [caseWorkflows, setCaseWorkflows] = React.useState<WorkflowInstanceRecord[]>([]);
  const [caseWorkflowJobs, setCaseWorkflowJobs] = React.useState<Record<number, WorkflowJobRecord[]>>({});
  const [caseWorkflowsError, setCaseWorkflowsError] = React.useState<string | null>(null);
  const [caseWorkflowsLoading, setCaseWorkflowsLoading] = React.useState(false);
  const [token, setToken] = React.useState<string | null>(null);
  const [operator, setOperator] = React.useState<Operator | null>(null);

  const refreshAll = React.useCallback(() => {
    setReloadKey((key) => key + 1);
  }, []);

  React.useEffect(() => {
    const controller = new AbortController();

    async function load() {
      try {
        setLoading(true);
        setError(null);
        const dashboard = await loadDashboard(controller.signal);
        setSnapshot(dashboard);
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

  React.useEffect(() => {
    if (selectedCaseID === null) {
      setCaseDetail(null);
      setCaseDetailError(null);
      setCaseDetailLoading(false);
      setCaseTimeline([]);
      setCaseTimelineError(null);
      setCaseTimelineLoading(false);
      setCaseNotes([]);
      setCaseNotesError(null);
      setCaseNotesLoading(false);
      setCaseWorkflows([]);
      setCaseWorkflowJobs({});
      setCaseWorkflowsError(null);
      setCaseWorkflowsLoading(false);
      return;
    }

    const caseID = selectedCaseID;
    const controller = new AbortController();

    async function loadSelectedCase() {
      try {
        setCaseDetailLoading(true);
        setCaseTimelineLoading(true);
        setCaseNotesLoading(true);
        setCaseWorkflowsLoading(true);
        setCaseDetailError(null);
        setCaseTimelineError(null);
        setCaseNotesError(null);
        setCaseWorkflowsError(null);

        const [detailResult, timelineResult, notesResult, workflowsResult] = await Promise.all([
          loadCase(caseID, controller.signal).then(
            (detail) => ({ ok: true as const, detail }),
            (error: unknown) => ({ ok: false as const, error }),
          ),
          loadCaseTimeline(caseID, controller.signal).then(
            (timeline) => ({ ok: true as const, timeline }),
            (error: unknown) => ({ ok: false as const, error }),
          ),
          loadCaseNotes(caseID, controller.signal).then(
            (notes) => ({ ok: true as const, notes }),
            (error: unknown) => ({ ok: false as const, error }),
          ),
          loadCaseWorkflows(caseID, controller.signal).then(
            (workflows) => ({ ok: true as const, workflows }),
            (error: unknown) => ({ ok: false as const, error }),
          ),
        ]);

        if (controller.signal.aborted) {
          return;
        }

        if (detailResult.ok) {
          setCaseDetail(detailResult.detail);
        } else {
          setCaseDetail(null);
          setCaseDetailError(errorMessage(detailResult.error));
          setCaseTimeline([]);
          setCaseTimelineError(null);
          setCaseNotes([]);
          setCaseNotesError(null);
          setCaseWorkflows([]);
          setCaseWorkflowJobs({});
          setCaseWorkflowsError(null);
          return;
        }

        if (timelineResult.ok) {
          setCaseTimeline(timelineResult.timeline);
        } else {
          setCaseTimeline([]);
          setCaseTimelineError(errorMessage(timelineResult.error));
        }

        if (notesResult.ok) {
          setCaseNotes(notesResult.notes);
        } else {
          setCaseNotes([]);
          setCaseNotesError(errorMessage(notesResult.error));
        }

        if (workflowsResult.ok) {
          setCaseWorkflows(workflowsResult.workflows);
          const jobEntries = await Promise.all(
            workflowsResult.workflows.map(async (instance) => {
              try {
                const jobs = await loadWorkflowJobs(instance.id, controller.signal);
                return [instance.id, jobs] as const;
              } catch {
                return [instance.id, []] as const;
              }
            }),
          );
          if (controller.signal.aborted) {
            return;
          }
          setCaseWorkflowJobs(Object.fromEntries(jobEntries));
        } else {
          setCaseWorkflows([]);
          setCaseWorkflowJobs({});
          setCaseWorkflowsError(errorMessage(workflowsResult.error));
        }
      } catch (loadError) {
        if (controller.signal.aborted) {
          return;
        }
        setCaseDetail(null);
        setCaseTimeline([]);
        setCaseNotes([]);
        setCaseWorkflows([]);
        setCaseWorkflowJobs({});
        setCaseDetailError(errorMessage(loadError));
        setCaseTimelineError(errorMessage(loadError));
        setCaseNotesError(errorMessage(loadError));
        setCaseWorkflowsError(errorMessage(loadError));
      } finally {
        if (!controller.signal.aborted) {
          setCaseDetailLoading(false);
          setCaseTimelineLoading(false);
          setCaseNotesLoading(false);
          setCaseWorkflowsLoading(false);
        }
      }
    }

    void loadSelectedCase();

    return () => controller.abort();
  }, [selectedCaseID, reloadKey]);

  const reloadSelectedCase = React.useCallback(() => {
    setReloadKey((key) => key + 1);
  }, []);

  async function handleSignIn(signedInToken: string) {
    const me = await loadMe(signedInToken);
    setToken(signedInToken);
    setOperator(me);
  }

  function handleSignOut() {
    setToken(null);
    setOperator(null);
  }

  if (loading && !snapshot) {
    return (
      <main className="shell">
        <section className="status-panel">
          <p className="eyebrow">Atlas</p>
          <h1>Loading cluster inventory</h1>
          <p>Waiting for the local API.</p>
        </section>
      </main>
    );
  }

  if (error || !snapshot) {
    return (
      <main className="shell">
        <section className="status-panel status-panel-error">
          <p className="eyebrow">Atlas</p>
          <h1>API unavailable</h1>
          <p>{error ?? "No dashboard data returned."}</p>
        </section>
      </main>
    );
  }

  const downOsds = snapshot.osds.filter((osd) => !osd.up).length;
  const outOsds = snapshot.osds.filter((osd) => !osd.in).length;
  const stoppedDaemons = stoppedDaemonCount(snapshot.daemons);

  return (
    <main className="shell">
      <header className="page-header">
        <div>
          <p className="eyebrow">Atlas</p>
          <h1>{snapshot.cluster.name}</h1>
          <p className="subtle">{snapshot.cluster.fsid}</p>
        </div>
        <div className="header-meta">
          <span className="pill">{snapshot.cluster.type}</span>
          <span className="pill">Ceph {snapshot.cluster.cephVersion}</span>
          <span className="pill pill-ok">API {snapshot.process.status}</span>
        </div>
      </header>

      <OperatorPanel
        operator={operator}
        onSignIn={handleSignIn}
        onSignOut={handleSignOut}
      />

      {operator && token ? (
        <CaseComposePanel
          token={token}
          onCreated={(created) => {
            setSelectedCaseID(created.id);
            refreshAll();
          }}
        />
      ) : null}

      <section className="summary-grid" aria-label="Cluster summary">
        <Metric
          label="Health"
          value={snapshot.health.status.replace("HEALTH_", "")}
          tone={toneForHealth(snapshot.health.status)}
          detail={snapshot.health.summary}
        />
        <Metric
          label="OSDs"
          value={String(snapshot.osds.length)}
          detail={`${downOsds} down, ${outOsds} out`}
        />
        <Metric
          label="Hosts"
          value={String(snapshot.hosts.length)}
          detail={`${snapshot.storageDevices.length} Storage Devices`}
        />
        <Metric
          label="Ceph Daemons"
          value={String(snapshot.daemons.length)}
          detail={stoppedDaemons === 0 ? "all running" : `${stoppedDaemons} stopped`}
          tone={stoppedDaemons === 0 ? "ok" : "warn"}
        />
        <Metric
          label="Sync Runs"
          value={String(snapshot.syncRuns.length)}
          detail={snapshot.syncRunsUnavailable ? "history unavailable" : "recent runs"}
          tone={snapshot.syncRunsUnavailable ? "warn" : "neutral"}
        />
        <Metric
          label="Cases"
          value={String(snapshot.cases.length)}
          detail={snapshot.casesUnavailable ? "records unavailable" : "recent updates"}
          tone={snapshot.casesUnavailable ? "warn" : "neutral"}
        />
      </section>

      <div className="content-grid">
        <section className="panel">
          <div className="section-heading">
            <h2>Health Checks</h2>
            <StatusBadge status={snapshot.health.status} />
          </div>
          {snapshot.health.checks.length === 0 ? (
            <p className="empty-state">No active health checks.</p>
          ) : (
            <ul className="check-list">
              {snapshot.health.checks.map((check) => (
                <li key={`${check.name}-${check.summary}`} className="check-row">
                  <div>
                    <strong>{check.name}</strong>
                    <span>{check.summary}</span>
                  </div>
                  <span className={`badge ${toneForHealth(check.severity)}`}>
                    {check.severity.replace("HEALTH_", "")}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </section>

        <section className="panel">
          <div className="section-heading">
            <h2>Inventory Sync Runs</h2>
            {snapshot.syncRunsUnavailable ? (
              <span className="badge warn">Unavailable</span>
            ) : (
              <span className="badge neutral">{snapshot.syncRuns.length}</span>
            )}
          </div>
          {snapshot.syncRunsUnavailable ? (
            <p className="empty-state">{snapshot.syncRunsUnavailable}</p>
          ) : snapshot.syncRuns.length === 0 ? (
            <p className="empty-state">No sync runs recorded.</p>
          ) : (
            <ul className="run-list">
              {snapshot.syncRuns.slice(0, 3).map((run) => (
                <SyncRunRow key={run.id} run={run} />
              ))}
            </ul>
          )}
        </section>

        <section className="panel">
          <div className="section-heading">
            <h2>Cases</h2>
            {snapshot.casesUnavailable ? (
              <span className="badge warn">Unavailable</span>
            ) : (
              <span className="badge neutral">{snapshot.cases.length}</span>
            )}
          </div>
          {snapshot.casesUnavailable ? (
            <p className="empty-state">{snapshot.casesUnavailable}</p>
          ) : snapshot.cases.length === 0 ? (
            <p className="empty-state">No cases recorded.</p>
          ) : (
            <ul className="case-list">
              {snapshot.cases.map((item) => (
                <CaseRow
                  key={item.id}
                  item={item}
                  selected={selectedCaseID === item.id}
                  onSelect={setSelectedCaseID}
                />
              ))}
            </ul>
          )}
        </section>
      </div>

      {selectedCaseID !== null ? (
        <CaseDetailPanel
          detail={caseDetail}
          error={caseDetailError}
          loading={caseDetailLoading}
          timeline={caseTimeline}
          timelineError={caseTimelineError}
          timelineLoading={caseTimelineLoading}
          notes={caseNotes}
          notesError={caseNotesError}
          notesLoading={caseNotesLoading}
          workflows={caseWorkflows}
          workflowJobs={caseWorkflowJobs}
          workflowsError={caseWorkflowsError}
          workflowsLoading={caseWorkflowsLoading}
          operator={operator}
          token={token}
          onChanged={reloadSelectedCase}
          onClose={() => setSelectedCaseID(null)}
        />
      ) : null}

      <section className="panel">
        <div className="section-heading">
          <h2>OSD Inventory</h2>
          <span className="badge neutral">{snapshot.osds.length}</span>
        </div>
        <OSDTable osds={snapshot.osds} />
      </section>

      <section className="panel">
        <div className="section-heading">
          <h2>Storage Devices</h2>
          <span className="badge neutral">{snapshot.storageDevices.length}</span>
        </div>
        <StorageDeviceTable devices={snapshot.storageDevices} />
      </section>

      <section className="panel">
        <div className="section-heading">
          <h2>Ceph Daemons</h2>
          <span className="badge neutral">{snapshot.daemons.length}</span>
        </div>
        <DaemonTable daemons={snapshot.daemons} />
      </section>

      <section className="panel">
        <div className="section-heading">
          <h2>Pools</h2>
          <span className="badge neutral">{snapshot.pools.length}</span>
        </div>
        <PoolTable pools={snapshot.pools} />
      </section>
    </main>
  );
}

function OperatorPanel({
  operator,
  onSignIn,
  onSignOut,
}: {
  operator: Operator | null;
  onSignIn: (token: string) => Promise<void>;
  onSignOut: () => void;
}) {
  const [tokenInput, setTokenInput] = React.useState("");
  const [busy, setBusy] = React.useState(false);
  const [signInError, setSignInError] = React.useState<string | null>(null);

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    const trimmed = tokenInput.trim();
    if (!trimmed || busy) {
      return;
    }
    try {
      setBusy(true);
      setSignInError(null);
      await onSignIn(trimmed);
      setTokenInput("");
    } catch (submitError) {
      setSignInError(errorMessage(submitError));
    } finally {
      setBusy(false);
    }
  }

  if (operator) {
    return (
      <section className="panel operator-panel" aria-label="Operator session">
        <div className="section-heading">
          <h2>Operator</h2>
          <button type="button" className="text-button" onClick={onSignOut}>
            Sign out
          </button>
        </div>
        <p>
          Signed in as <strong>{operator.displayName}</strong>{" "}
          <span className="subtle">({operator.subject})</span>. Manual case writes are
          enabled.
        </p>
      </section>
    );
  }

  return (
    <section className="panel operator-panel" aria-label="Operator session">
      <div className="section-heading">
        <h2>Operator</h2>
      </div>
      <form className="token-form" onSubmit={submit}>
        <label htmlFor="operator-token">Paste a bearer token to enable manual case writes</label>
        <div className="token-form-row">
          <input
            id="operator-token"
            type="password"
            value={tokenInput}
            placeholder="eyJhbGciOiJSUzI1NiIs..."
            onChange={(event) => setTokenInput(event.target.value)}
            autoComplete="off"
          />
          <button type="submit" disabled={busy || tokenInput.trim() === ""}>
            {busy ? "Verifying…" : "Sign in"}
          </button>
        </div>
        <p className="subtle">
          Local development: request one from the dev issuer at{" "}
          <code>POST /token</code> on the Atlas dev issuer port.
        </p>
        {signInError ? <p className="form-error">{signInError}</p> : null}
      </form>
    </section>
  );
}

function CaseComposePanel({
  token,
  onCreated,
}: {
  token: string;
  onCreated: (created: CaseRecord) => void;
}) {
  const [title, setTitle] = React.useState("");
  const [summary, setSummary] = React.useState("");
  const [severity, setSeverity] = React.useState<CaseRecord["severity"]>("medium");
  const [clusterFsid, setClusterFsid] = React.useState("");
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
      setClusterFsid("");
      onCreated(created);
    } catch (submitError) {
      setComposeError(errorMessage(submitError));
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="panel compose-panel" aria-label="Create a manual case">
      <div className="section-heading">
        <h2>New Manual Case</h2>
      </div>
      <form className="compose-form" onSubmit={submit}>
        <div className="form-grid">
          <label>
            Title
            <input
              value={title}
              onChange={(event) => setTitle(event.target.value)}
              placeholder="Manual review of slow OSD warnings"
              required
            />
          </label>
          <label>
            Severity
            <select
              value={severity}
              onChange={(event) => setSeverity(event.target.value as CaseRecord["severity"])}
            >
              {SEVERITIES.map((value) => (
                <option key={value} value={value}>
                  {value}
                </option>
              ))}
            </select>
          </label>
          <label className="form-grid-wide">
            Summary
            <textarea
              value={summary}
              onChange={(event) => setSummary(event.target.value)}
              placeholder="What the operator observed and why this needs a case."
              rows={2}
              required
            />
          </label>
          <label className="form-grid-wide">
            Cluster FSID (optional)
            <input
              value={clusterFsid}
              onChange={(event) => setClusterFsid(event.target.value)}
              placeholder="00000000-0000-4000-8000-000000000101"
            />
          </label>
        </div>
        <div className="action-row">
          <button type="submit" disabled={busy || title.trim() === "" || summary.trim() === ""}>
            {busy ? "Creating…" : "Create case"}
          </button>
          {composeError ? <p className="form-error">{composeError}</p> : null}
        </div>
      </form>
    </section>
  );
}

function Metric({
  label,
  value,
  detail,
  tone = "neutral",
}: {
  label: string;
  value: string;
  detail: string;
  tone?: "neutral" | "ok" | "warn" | "err";
}) {
  return (
    <section className={`metric ${tone}`}>
      <span>{label}</span>
      <strong>{value}</strong>
      <p>{detail}</p>
    </section>
  );
}

function StatusBadge({ status }: { status: string }) {
  return <span className={`badge ${toneForHealth(status)}`}>{status.replace("HEALTH_", "")}</span>;
}

function SyncRunRow({ run }: { run: InventorySyncRun }) {
  return (
    <li className="run-row">
      <div>
        <strong>#{run.id}</strong>
        <span>{run.scenario ?? run.provider}</span>
      </div>
      <div>
        <span className={`badge ${toneForRun(run.status)}`}>{run.status}</span>
        <time dateTime={run.startedAt}>{formatDate(run.startedAt)}</time>
      </div>
      {run.errorMessage ? <p>{run.errorMessage}</p> : null}
    </li>
  );
}

function CaseRow({
  item,
  selected,
  onSelect,
}: {
  item: CaseRecord;
  selected: boolean;
  onSelect: (id: number) => void;
}) {
  return (
    <li className="case-row">
      <button
        type="button"
        className={`case-row-button${selected ? " selected" : ""}`}
        onClick={() => onSelect(item.id)}
      >
        <span>
          <strong>{item.title}</strong>
          <span>{item.summary}</span>
        </span>
        <span className="case-meta">
          <span className={`badge ${toneForCaseSeverity(item.severity)}`}>{item.severity}</span>
          <span className={`badge ${toneForCaseStatus(item.status)}`}>{item.status}</span>
          <span>{item.source}</span>
          {item.assigneeDisplayName ? <span>{item.assigneeDisplayName}</span> : null}
          <time dateTime={item.updatedAt}>{formatDate(item.updatedAt)}</time>
        </span>
      </button>
    </li>
  );
}

function CaseDetailPanel({
  detail,
  error,
  loading,
  timeline,
  timelineError,
  timelineLoading,
  notes,
  notesError,
  notesLoading,
  workflows,
  workflowJobs,
  workflowsError,
  workflowsLoading,
  operator,
  token,
  onChanged,
  onClose,
}: {
  detail: CaseRecord | null;
  error: string | null;
  loading: boolean;
  timeline: TimelineEvent[];
  timelineError: string | null;
  timelineLoading: boolean;
  notes: CaseNote[];
  notesError: string | null;
  notesLoading: boolean;
  workflows: WorkflowInstanceRecord[];
  workflowJobs: Record<number, WorkflowJobRecord[]>;
  workflowsError: string | null;
  workflowsLoading: boolean;
  operator: Operator | null;
  token: string | null;
  onChanged: () => void;
  onClose: () => void;
}) {
  return (
    <section className="panel case-detail-panel">
      <div className="section-heading">
        <h2>Case Detail</h2>
        <button type="button" className="text-button" onClick={onClose}>
          Close
        </button>
      </div>
      {loading ? <p className="empty-state">Loading case detail.</p> : null}
      {error ? <p className="empty-state">{error}</p> : null}
      {!loading && !error && detail ? (
        <div className="case-detail">
          <div className="case-detail-header">
            <div>
              <strong>{detail.title}</strong>
              <p>{detail.summary}</p>
            </div>
            <div className="case-meta">
              <span className={`badge ${toneForCaseSeverity(detail.severity)}`}>
                {detail.severity}
              </span>
              <span className={`badge ${toneForCaseStatus(detail.status)}`}>{detail.status}</span>
              <span>{detail.source}</span>
            </div>
          </div>
          {detail.detectedBy ? (
            <p className="detection-link">{detectionLabel(detail.detectedBy)}</p>
          ) : null}
          <div className="detail-grid">
            <DetailField label="Case ID" value={`#${detail.id}`} />
            <DetailField label="Cluster FSID" value={detail.clusterFsid ?? "unassigned"} />
            <DetailField
              label="Assignee"
              value={detail.assigneeDisplayName ?? detail.assignee ?? "unassigned"}
            />
            <DetailField label="Created" value={formatDate(detail.createdAt)} />
            <DetailField label="Updated" value={formatDate(detail.updatedAt)} />
            <DetailField label="Closed" value={detail.closedAt ? formatDate(detail.closedAt) : "open"} />
            {detail.detectedBy ? (
              <DetailField
                label="Alert first seen"
                value={formatDate(detail.detectedBy.firstSeenAt)}
              />
            ) : null}
          </div>
          {operator && token ? (
            <CaseActions detail={detail} token={token} onChanged={onChanged} />
          ) : null}
          <CaseWorkflows
            caseID={detail.id}
            error={workflowsError}
            loading={workflowsLoading}
            workflows={workflows}
            workflowJobs={workflowJobs}
            canAttach={operator !== null && token !== null && availableCaseActions(detail.status).canAttachWorkflow}
            token={token}
            onChanged={onChanged}
          />
          <CaseNotes
            error={notesError}
            loading={notesLoading}
            notes={notes}
            operator={operator}
            token={token}
            caseID={detail.id}
            onChanged={onChanged}
          />
          <CaseTimeline
            error={timelineError}
            loading={timelineLoading}
            timeline={timeline}
          />
        </div>
      ) : null}
    </section>
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
    <div className="case-actions">
      <div className="action-row">
        {actions.canTriage ? (
          <button type="button" disabled={busy} onClick={() => run(() => transitionCase(detail.id, "triaged", token))}>
            Triage
          </button>
        ) : null}
        {actions.canClose ? (
          <button type="button" disabled={busy} onClick={() => run(() => transitionCase(detail.id, "closed", token))}>
            Close case
          </button>
        ) : null}
        {actions.isClosed ? (
          <p className="subtle">
            Closed. Reopening means creating a new case for a recurring condition.
          </p>
        ) : (
          <div className="assignment-form">
            <input
              value={assignee}
              onChange={(event) => setAssignee(event.target.value)}
              placeholder="Assignee subject"
              aria-label="Assignee subject"
            />
            <input
              value={assigneeName}
              onChange={(event) => setAssigneeName(event.target.value)}
              placeholder="Assignee display name"
              aria-label="Assignee display name"
            />
            <button
              type="button"
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
            </button>
            <button
              type="button"
              disabled={busy || !detail.assignee}
              onClick={() => run(() => assignCase(detail.id, "", "", token))}
            >
              Unassign
            </button>
          </div>
        )}
      </div>
      {actionError ? <p className="form-error">{actionError}</p> : null}
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
    <section className="case-workflows" aria-label="Workflows">
      <div className="section-heading">
        <h3>Workflows</h3>
        {!loading && !error ? <span className="badge neutral">{workflows.length}</span> : null}
      </div>
      {loading ? <p className="empty-state">Loading Workflow Instances.</p> : null}
      {error ? <p className="empty-state">Workflow Instances unavailable: {error}</p> : null}
      {!loading && !error && workflows.length === 0 ? (
        <p className="empty-state">No Workflow Instances attached.</p>
      ) : null}
      {!loading && !error && workflows.length > 0 ? (
        <ul className="workflow-instance-list">
          {workflows.map((instance) => (
            <li key={instance.id} className="workflow-instance-row">
              <div className="workflow-instance-heading">
                <strong>
                  {instance.workflowId} v{instance.workflowVersion}
                </strong>
                <span className={`badge ${toneForWorkflowState(instance.state)}`}>
                  {instance.state.replace(/_/g, " ")}
                </span>
              </div>
              <p className="workflow-instance-meta">
                #{instance.id}
                {instance.currentStep ? ` · paused at ${instance.currentStep}` : ""}
                {` · updated ${formatDate(instance.updatedAt)}`}
              </p>
              {workflowJobs[instance.id]?.length ? (
                <ul className="workflow-job-list">
                  {workflowJobs[instance.id].map((job) => (
                    <li key={job.id} className="workflow-job-row">
                      <span className="workflow-job-step">{job.stepId}</span>
                      <span className="workflow-job-operation">{job.operationType}</span>
                      <span className={`badge ${toneForJobState(job.state)}`}>
                        {job.state}
                      </span>
                      {job.maxAttempts > 1 ? (
                        <span className="workflow-job-attempts">
                          {`attempt ${job.attempt}/${job.maxAttempts}`}
                        </span>
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
        <form className="workflow-attach-form" onSubmit={submit}>
          <input
            value={workflowID}
            onChange={(event) => setWorkflowID(event.target.value)}
            placeholder="Workflow id, e.g. replace-osd"
            aria-label="Workflow id"
          />
          <input
            value={workflowVersion}
            onChange={(event) => setWorkflowVersion(event.target.value)}
            placeholder="Version"
            aria-label="Workflow version"
          />
          <div className="action-row">
            <button type="submit" disabled={busy || !formReady}>
              {busy ? "Attaching…" : "Attach workflow"}
            </button>
            {attachError ? <p className="form-error">{attachError}</p> : null}
          </div>
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
    <form className="workflow-approve-form" onSubmit={submit}>
      <input
        value={reason}
        onChange={(event) => setReason(event.target.value)}
        placeholder="Reason (optional)"
        aria-label="Approval reason"
      />
      <div className="action-row">
        <button type="submit" disabled={busy}>
          {busy ? "Approving…" : `Approve ${gateID}`}
        </button>
        {approveError ? <p className="form-error">{approveError}</p> : null}
      </div>
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
    <form className="workflow-resume-form" onSubmit={submit}>
      <input
        value={note}
        onChange={(event) => setNote(event.target.value)}
        placeholder="Note (optional)"
        aria-label="Task completion note"
      />
      <div className="action-row">
        <button type="submit" disabled={busy}>
          {busy ? "Resuming…" : `Done: ${taskID}`}
        </button>
        {resumeError ? <p className="form-error">{resumeError}</p> : null}
      </div>
    </form>
  );
}

function toneForWorkflowState(state: WorkflowInstanceRecord["state"]): string {
  switch (state) {
    case "succeeded":
      return "ok";
    case "failed":
    case "cancelled":
      return "err";
    case "running":
    case "pending":
      return "neutral";
    case "waiting_for_approval":
    case "waiting_for_operator":
      return "warn";
  }
}

function toneForJobState(state: WorkflowJobRecord["state"]): string {
  switch (state) {
    case "succeeded":
      return "ok";
    case "failed":
      return "err";
    case "pending":
    case "dispatched":
      return "neutral";
  }
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
    <section className="case-notes" aria-label="Case Notes">
      <div className="section-heading">
        <h3>Notes</h3>
        {!loading && !error ? <span className="badge neutral">{notes.length}</span> : null}
      </div>
      {loading ? <p className="empty-state">Loading Case Notes.</p> : null}
      {error ? <p className="empty-state">Case Notes unavailable: {error}</p> : null}
      {!loading && !error && notes.length === 0 ? (
        <p className="empty-state">No Case Notes recorded.</p>
      ) : null}
      {!loading && !error && notes.length > 0 ? (
        <ul className="note-list">
          {notes.map((note) => (
            <li key={note.id} className="note-row">
              <div className="note-heading">
                <strong>{note.authorDisplayName}</strong>
                <time dateTime={note.createdAt}>{formatDate(note.createdAt)}</time>
              </div>
              <p>{note.body}</p>
            </li>
          ))}
        </ul>
      ) : null}
      {operator && token ? (
        <form className="note-form" onSubmit={submit}>
          <textarea
            value={body}
            onChange={(event) => setBody(event.target.value)}
            placeholder="Add a note about investigation progress."
            rows={2}
          />
          <div className="action-row">
            <button type="submit" disabled={busy || body.trim() === ""}>
              {busy ? "Adding…" : "Add note"}
            </button>
            {noteError ? <p className="form-error">{noteError}</p> : null}
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
    <section className="case-timeline" aria-label="Case Timeline Events">
      <div className="section-heading">
        <h3>Timeline Events</h3>
        {!loading && !error ? <span className="badge neutral">{timeline.length}</span> : null}
      </div>
      {loading ? <p className="empty-state">Loading Timeline Events.</p> : null}
      {error ? <p className="empty-state">Timeline Events unavailable: {error}</p> : null}
      {!loading && !error && timeline.length === 0 ? (
        <p className="empty-state">No Timeline Events recorded.</p>
      ) : null}
      {!loading && !error && timeline.length > 0 ? (
        <ol className="timeline-list">
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
    <li className="timeline-event">
      <div className="timeline-marker" aria-hidden="true" />
      <div className="timeline-event-body">
        <div className="timeline-event-heading">
          <strong>{labelForTimelineEventType(event.type)}</strong>
          <time dateTime={event.occurredAt}>{formatDate(event.occurredAt)}</time>
        </div>
        <p>{event.message}</p>
        <div className="timeline-event-meta">
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

function DetailField({ label, value }: { label: string; value: string }) {
  return (
    <div className="detail-field">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

function OSDTable({ osds }: { osds: OSD[] }) {
  if (osds.length === 0) {
    return <p className="empty-state">No OSDs returned.</p>;
  }

  return (
    <div className="table-wrap">
      <table>
        <thead>
          <tr>
            <th>ID</th>
            <th>Host</th>
            <th>Up</th>
            <th>In</th>
            <th>Device</th>
          </tr>
        </thead>
        <tbody>
          {osds.map((osd) => (
            <tr key={osd.id}>
              <td>{osd.id}</td>
              <td>{osd.host}</td>
              <td>
                <span className={`badge ${osd.up ? "ok" : "err"}`}>{osd.up ? "yes" : "no"}</span>
              </td>
              <td>
                <span className={`badge ${osd.in ? "ok" : "warn"}`}>{osd.in ? "yes" : "no"}</span>
              </td>
              <td>{osd.device ?? "unreported"}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function StorageDeviceTable({ devices }: { devices: StorageDevice[] }) {
  if (devices.length === 0) {
    return <p className="empty-state">No Storage Devices returned.</p>;
  }

  return (
    <div className="table-wrap">
      <table>
        <thead>
          <tr>
            <th>Serial</th>
            <th>Host</th>
            <th>Type</th>
            <th>Path</th>
            <th>Health</th>
            <th>Backing</th>
          </tr>
        </thead>
        <tbody>
          {devices.map((device) => (
            <tr key={`${device.host}-${device.serial}`}>
              <td>{device.serial}</td>
              <td>{device.host}</td>
              <td>{device.type ?? "unknown"}</td>
              <td>{device.path ?? "unreported"}</td>
              <td>
                <span className={`badge ${toneForDeviceHealth(device.health)}`}>
                  {device.health ?? "unknown"}
                </span>
              </td>
              <td>{storageDeviceOSDLabel(device)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function DaemonTable({ daemons }: { daemons: Daemon[] }) {
  if (daemons.length === 0) {
    return <p className="empty-state">No Ceph Daemons returned.</p>;
  }

  return (
    <div className="table-wrap">
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>Type</th>
            <th>Host</th>
            <th>Status</th>
            <th>Version</th>
          </tr>
        </thead>
        <tbody>
          {daemons.map((daemon) => (
            <tr key={`${daemon.type}-${daemon.name}`}>
              <td>{daemon.name}</td>
              <td>{daemon.type}</td>
              <td>{daemon.host}</td>
              <td>
                <span className={`badge ${daemon.status === "running" ? "ok" : "err"}`}>
                  {daemon.status}
                </span>
              </td>
              <td>{daemon.version ?? "unreported"}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function PoolTable({ pools }: { pools: Pool[] }) {
  if (pools.length === 0) {
    return <p className="empty-state">No Pools returned.</p>;
  }

  return (
    <div className="table-wrap">
      <table>
        <thead>
          <tr>
            <th>ID</th>
            <th>Name</th>
            <th>Type</th>
            <th>Redundancy</th>
          </tr>
        </thead>
        <tbody>
          {pools.map((pool) => (
            <tr key={pool.id}>
              <td>{pool.id}</td>
              <td>{pool.name}</td>
              <td>{pool.type}</td>
              <td>{poolRedundancyLabel(pool)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function toneForDeviceHealth(health: string | undefined): "ok" | "warn" | "err" | "neutral" {
  if (health === "ok") {
    return "ok";
  }
  if (health === "warning") {
    return "warn";
  }
  if (health === "error") {
    return "err";
  }
  return "neutral";
}

function toneForHealth(status: string): "ok" | "warn" | "err" | "neutral" {
  if (status === "HEALTH_OK") {
    return "ok";
  }
  if (status === "HEALTH_WARN") {
    return "warn";
  }
  if (status === "HEALTH_ERR") {
    return "err";
  }
  return "neutral";
}

function toneForRun(status: InventorySyncRun["status"]): "ok" | "warn" | "err" {
  if (status === "succeeded") {
    return "ok";
  }
  if (status === "failed") {
    return "err";
  }
  return "warn";
}

function toneForCaseStatus(status: CaseRecord["status"]): "ok" | "warn" | "neutral" {
  if (status === "closed") {
    return "ok";
  }
  if (status === "detected") {
    return "warn";
  }
  return "neutral";
}

function toneForCaseSeverity(severity: CaseRecord["severity"]): "ok" | "warn" | "err" | "neutral" {
  if (severity === "critical" || severity === "high") {
    return "err";
  }
  if (severity === "medium") {
    return "warn";
  }
  if (severity === "low") {
    return "neutral";
  }
  return "ok";
}

function formatDate(value: string): string {
  return new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(value));
}

function errorMessage(error: unknown): string {
  if (error instanceof Error) {
    return error.message;
  }
  return "Request failed.";
}

const root = document.getElementById("root");
if (!root) {
  throw new Error("root element not found");
}

createRoot(root).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);

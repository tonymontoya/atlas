import React from "react";
import { createRoot } from "react-dom/client";
import {
  loadCase,
  loadCaseTimeline,
  loadDashboard,
  type CaseRecord,
  type Daemon,
  type DashboardSnapshot,
  type InventorySyncRun,
  type OSD,
  type Pool,
  type StorageDevice,
  type TimelineEvent,
} from "./api";
import { poolRedundancyLabel, stoppedDaemonCount, storageDeviceOSDLabel } from "./inventory";
import { detectionLabel, labelForTimelineEventType, timelinePayloadLabels } from "./timeline";
import "./styles.css";

function App() {
  const [snapshot, setSnapshot] = React.useState<DashboardSnapshot | null>(null);
  const [error, setError] = React.useState<string | null>(null);
  const [loading, setLoading] = React.useState(true);
  const [selectedCaseID, setSelectedCaseID] = React.useState<number | null>(null);
  const [caseDetail, setCaseDetail] = React.useState<CaseRecord | null>(null);
  const [caseDetailError, setCaseDetailError] = React.useState<string | null>(null);
  const [caseDetailLoading, setCaseDetailLoading] = React.useState(false);
  const [caseTimeline, setCaseTimeline] = React.useState<TimelineEvent[]>([]);
  const [caseTimelineError, setCaseTimelineError] = React.useState<string | null>(null);
  const [caseTimelineLoading, setCaseTimelineLoading] = React.useState(false);

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
  }, []);

  React.useEffect(() => {
    if (selectedCaseID === null) {
      setCaseDetail(null);
      setCaseDetailError(null);
      setCaseDetailLoading(false);
      setCaseTimeline([]);
      setCaseTimelineError(null);
      setCaseTimelineLoading(false);
      return;
    }

    const caseID = selectedCaseID;
    const controller = new AbortController();

    async function loadSelectedCase() {
      try {
        setCaseDetailLoading(true);
        setCaseTimelineLoading(true);
        setCaseDetailError(null);
        setCaseTimelineError(null);

        const [detailResult, timelineResult] = await Promise.all([
          loadCase(caseID, controller.signal).then(
            (detail) => ({ ok: true as const, detail }),
            (error: unknown) => ({ ok: false as const, error }),
          ),
          loadCaseTimeline(caseID, controller.signal).then(
            (timeline) => ({ ok: true as const, timeline }),
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
          return;
        }

        if (timelineResult.ok) {
          setCaseTimeline(timelineResult.timeline);
        } else {
          setCaseTimeline([]);
          setCaseTimelineError(errorMessage(timelineResult.error));
        }
      } catch (loadError) {
        if (controller.signal.aborted) {
          return;
        }
        setCaseDetail(null);
        setCaseTimeline([]);
        setCaseDetailError(errorMessage(loadError));
        setCaseTimelineError(errorMessage(loadError));
      } finally {
        if (!controller.signal.aborted) {
          setCaseDetailLoading(false);
          setCaseTimelineLoading(false);
        }
      }
    }

    void loadSelectedCase();

    return () => controller.abort();
  }, [selectedCaseID]);

  if (loading) {
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
              {snapshot.cases.slice(0, 3).map((item) => (
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
  onClose,
}: {
  detail: CaseRecord | null;
  error: string | null;
  loading: boolean;
  timeline: TimelineEvent[];
  timelineError: string | null;
  timelineLoading: boolean;
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

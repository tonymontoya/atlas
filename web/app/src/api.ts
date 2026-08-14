export type HealthzResponse = {
  status: "ok";
};

export type ClusterIdentity = {
  fsid: string;
  name: string;
  cephVersion: string;
  type: "bare-metal" | "rook";
};

export type ClusterHealth = {
  status: "HEALTH_OK" | "HEALTH_WARN" | "HEALTH_ERR";
  summary: string;
  checks: HealthCheck[];
};

export type HealthCheck = {
  name: string;
  severity: string;
  summary: string;
};

export type OSD = {
  id: number;
  host: string;
  up: boolean;
  in: boolean;
  device?: string;
};

export type InventorySyncRun = {
  id: number;
  provider: "fake";
  scenario?: string;
  status: "running" | "succeeded" | "failed";
  startedAt: string;
  finishedAt?: string;
  snapshotId?: number;
  errorClass?: string;
  errorMessage?: string;
};

export type CaseRecord = {
  id: number;
  title: string;
  summary: string;
  status: "detected" | "triaged" | "closed";
  severity: "info" | "low" | "medium" | "high" | "critical";
  source: "manual" | "prometheus" | "ceph" | "rook" | "atlas";
  clusterFsid?: string;
  createdAt: string;
  updatedAt: string;
  closedAt?: string;
};

export type TimelineEventType =
  | "case_detected"
  | "case_triaged"
  | "case_status_changed"
  | "case_note_added"
  | "workflow_attached"
  | "workflow_state_changed";

export type TimelineActor = {
  type: "system" | "user" | "atlas_agent" | "provider";
  id?: string;
  displayName: string;
};

export type TimelineEvent = {
  id: number;
  caseId: number;
  type: TimelineEventType;
  message: string;
  occurredAt: string;
  actor: TimelineActor;
  payload: Record<string, unknown>;
};

export type DashboardSnapshot = {
  process: HealthzResponse;
  cluster: ClusterIdentity;
  health: ClusterHealth;
  osds: OSD[];
  syncRuns: InventorySyncRun[];
  syncRunsUnavailable?: string;
  cases: CaseRecord[];
  casesUnavailable?: string;
};

type ApiErrorBody = {
  error?: {
    class?: string;
    message?: string;
  };
};

export class ApiRequestError extends Error {
  readonly status: number;
  readonly errorClass?: string;

  constructor(message: string, status: number, errorClass?: string) {
    super(message);
    this.name = "ApiRequestError";
    this.status = status;
    this.errorClass = errorClass;
  }
}

export async function loadDashboard(
  signal?: AbortSignal,
): Promise<DashboardSnapshot> {
  const [process, cluster, health, osds, syncRunsResult, casesResult] = await Promise.all([
    request<HealthzResponse>("/healthz", signal),
    request<ClusterIdentity>("/api/v1/clusters/current", signal),
    request<ClusterHealth>("/api/v1/clusters/current/health", signal),
    request<OSD[]>("/api/v1/clusters/current/osds", signal),
    request<InventorySyncRun[]>("/api/v1/inventory-sync-runs", signal).then(
      (syncRuns) => ({ ok: true as const, syncRuns }),
      (error: unknown) => ({ ok: false as const, error }),
    ),
    request<CaseRecord[]>("/api/v1/cases", signal).then(
      (cases) => ({ ok: true as const, cases }),
      (error: unknown) => ({ ok: false as const, error }),
    ),
  ]);

  return {
    process,
    cluster,
    health,
    osds,
    syncRuns: syncRunsResult.ok ? syncRunsResult.syncRuns : [],
    syncRunsUnavailable: syncRunsResult.ok ? undefined : messageForError(syncRunsResult.error),
    cases: casesResult.ok ? casesResult.cases : [],
    casesUnavailable: casesResult.ok ? undefined : messageForError(casesResult.error),
  };
}

export async function loadCase(id: number, signal?: AbortSignal): Promise<CaseRecord> {
  return request<CaseRecord>(`/api/v1/cases/${id}`, signal);
}

export async function loadCaseTimeline(
  id: number,
  signal?: AbortSignal,
): Promise<TimelineEvent[]> {
  return request<TimelineEvent[]>(`/api/v1/cases/${id}/timeline`, signal);
}

async function request<T>(path: string, signal?: AbortSignal): Promise<T> {
  const response = await fetch(path, {
    headers: { Accept: "application/json" },
    signal,
  });

  if (!response.ok) {
    const body = await readErrorBody(response);
    const message = body.error?.message ?? `${response.status} ${response.statusText}`;
    throw new ApiRequestError(message, response.status, body.error?.class);
  }

  return response.json() as Promise<T>;
}

async function readErrorBody(response: Response): Promise<ApiErrorBody> {
  try {
    return (await response.json()) as ApiErrorBody;
  } catch {
    return {};
  }
}

function messageForError(error: unknown): string {
  if (error instanceof ApiRequestError) {
    return error.message;
  }
  if (error instanceof Error) {
    return error.message;
  }
  return "requested data is unavailable";
}

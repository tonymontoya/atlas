export type ClusterType = "bare-metal" | "rook";

export type ClusterSummary = {
  id: number | null;
  fsid: string | null;
  name: string;
  clusterType: ClusterType;
  cephVersion: string | null;
  healthStatus: string | null;
  healthSummary: string | null;
  agentLastSeen: string | null;
};

export type ClusterIndex = {
  clusters: ClusterSummary[];
  total: number;
  limit: number;
  offset: number;
};

export type ClusterQuery = {
  q?: string;
  limit?: number;
  offset?: number;
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

export type Host = {
  name: string;
  address?: string;
};

export type StorageDevice = {
  host: string;
  serial: string;
  type?: string;
  path?: string;
  health?: string;
  osdId?: number;
};

export type Daemon = {
  type: "mon" | "mgr" | "osd" | "mds" | "rgw";
  name: string;
  host: string;
  status: "running" | "stopped" | "starting" | "error" | "unknown";
  version?: string;
};

export type Pool = {
  id: number;
  name: string;
  type: "replicated" | "erasure";
  size?: number;
  minSize?: number;
};

export type InventorySyncRun = {
  id: number;
  provider: "fake" | "ceph" | "agent";
  scenario?: string;
  status: "running" | "succeeded" | "failed";
  startedAt: string;
  finishedAt?: string;
  snapshotId?: number;
  errorClass?: string;
  errorMessage?: string;
};

export type CaseDetectionLink = {
  source: CaseRecord["source"];
  alertName: string;
  signal?: string;
  firstSeenAt: string;
  lastSeenAt: string;
};

export type CaseRecord = {
  id: number;
  title: string;
  summary: string;
  status: "detected" | "triaged" | "closed";
  severity: "info" | "low" | "medium" | "high" | "critical";
  source: "manual" | "prometheus" | "ceph" | "rook" | "atlas";
  clusterFsid?: string;
  assignee?: string;
  assigneeDisplayName?: string;
  createdAt: string;
  updatedAt: string;
  closedAt?: string;
  detectedBy?: CaseDetectionLink;
};

export type TimelineEventType =
  | "case_detected"
  | "case_triaged"
  | "case_status_changed"
  | "case_note_added"
  | "case_assigned"
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

export type ClusterView = {
  cluster: ClusterSummary;
  health: ClusterHealth;
  osds: OSD[];
  hosts: Host[];
  storageDevices: StorageDevice[];
  daemons: Daemon[];
  pools: Pool[];
  cases: CaseRecord[];
  casesUnavailable?: string;
};

export type ClusterViewNotFound = {
  notFound: true;
};

function messageForError(error: unknown): string {
  if (error instanceof ApiRequestError) {
    return error.message;
  }
  if (error instanceof Error) {
    return error.message;
  }
  return "requested data is unavailable";
}

export async function listClusters(
  query: ClusterQuery = {},
  signal?: AbortSignal,
): Promise<ClusterIndex> {
  const params = new URLSearchParams();
  if (query.q !== undefined && query.q !== "") {
    params.set("q", query.q);
  }
  if (query.limit !== undefined) {
    params.set("limit", String(query.limit));
  }
  if (query.offset !== undefined && query.offset > 0) {
    params.set("offset", String(query.offset));
  }
  const suffix = params.size > 0 ? `?${params.toString()}` : "";
  return request<ClusterIndex>(`/api/v1/clusters${suffix}`, signal);
}

// loadClusterView gathers everything the per-cluster page shows. The
// cluster itself is resolved through the index (searching by FSID) so
// provider-mode clusters — whose numeric id is null — stay addressable.
// A FSID no registered cluster carries resolves to notFound.
export async function loadClusterView(
  fsid: string,
  signal?: AbortSignal,
): Promise<ClusterView | ClusterViewNotFound> {
  const index = await listClusters({ q: fsid }, signal);
  const cluster = index.clusters.find(
    (candidate) =>
      candidate.fsid !== null &&
      candidate.fsid.toLowerCase() === fsid.toLowerCase(),
  );
  if (!cluster || cluster.fsid === null) {
    return { notFound: true };
  }

  const scopedFSID = cluster.fsid;
  const [health, osds, hosts, storageDevices, daemons, pools, casesResult] =
    await Promise.all([
      request<ClusterHealth>(`/api/v1/clusters/${scopedFSID}/health`, signal),
      request<OSD[]>(`/api/v1/clusters/${scopedFSID}/osds`, signal),
      request<Host[]>(`/api/v1/clusters/${scopedFSID}/hosts`, signal),
      request<StorageDevice[]>(
        `/api/v1/clusters/${scopedFSID}/storage-devices`,
        signal,
      ),
      request<Daemon[]>(`/api/v1/clusters/${scopedFSID}/daemons`, signal),
      request<Pool[]>(`/api/v1/clusters/${scopedFSID}/pools`, signal),
      request<CaseRecord[]>(`/api/v1/cases?cluster=${encodeURIComponent(scopedFSID)}`, signal).then(
        (cases) => ({ ok: true as const, cases }),
        (error: unknown) => ({ ok: false as const, error }),
      ),
    ]);

  return {
    cluster,
    health,
    osds,
    hosts,
    storageDevices,
    daemons,
    pools,
    cases: casesResult.ok ? casesResult.cases : [],
    casesUnavailable: casesResult.ok ? undefined : messageForError(casesResult.error),
  };
}

export async function listCases(
  filter: { cluster?: string } = {},
  signal?: AbortSignal,
): Promise<CaseRecord[]> {
  const suffix =
    filter.cluster !== undefined && filter.cluster !== ""
      ? `?cluster=${encodeURIComponent(filter.cluster)}`
      : "";
  return request<CaseRecord[]>(`/api/v1/cases${suffix}`, signal);
}

export async function listSyncRuns(signal?: AbortSignal): Promise<InventorySyncRun[]> {
  return request<InventorySyncRun[]>("/api/v1/inventory-sync-runs", signal);
}

export type Operator = {
  subject: string;
  displayName: string;
};

export type CaseNote = {
  id: number;
  caseId: number;
  authorId: string;
  authorDisplayName: string;
  body: string;
  createdAt: string;
};

export type CreateCaseInput = {
  title: string;
  summary: string;
  severity: CaseRecord["severity"];
  clusterFsid?: string;
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

export async function loadCase(id: number, signal?: AbortSignal): Promise<CaseRecord> {
  return request<CaseRecord>(`/api/v1/cases/${id}`, signal);
}

export async function loadCaseTimeline(
  id: number,
  signal?: AbortSignal,
): Promise<TimelineEvent[]> {
  return request<TimelineEvent[]>(`/api/v1/cases/${id}/timeline`, signal);
}

export async function loadMe(token: string, signal?: AbortSignal): Promise<Operator> {
  return request<Operator>("/api/v1/me", signal, token);
}

export async function createCase(
  input: CreateCaseInput,
  token: string,
  signal?: AbortSignal,
): Promise<CaseRecord> {
  return request<CaseRecord>("/api/v1/cases", signal, token, {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export async function transitionCase(
  id: number,
  status: CaseRecord["status"],
  token: string,
  signal?: AbortSignal,
): Promise<CaseRecord> {
  return request<CaseRecord>(`/api/v1/cases/${id}/transitions`, signal, token, {
    method: "POST",
    body: JSON.stringify({ status }),
  });
}

export async function assignCase(
  id: number,
  assignee: string,
  assigneeDisplayName: string,
  token: string,
  signal?: AbortSignal,
): Promise<CaseRecord> {
  return request<CaseRecord>(`/api/v1/cases/${id}/assignment`, signal, token, {
    method: "POST",
    body: JSON.stringify({ assignee, assigneeDisplayName }),
  });
}

export async function addCaseNote(
  id: number,
  body: string,
  token: string,
  signal?: AbortSignal,
): Promise<CaseNote> {
  return request<CaseNote>(`/api/v1/cases/${id}/notes`, signal, token, {
    method: "POST",
    body: JSON.stringify({ body }),
  });
}

export type WorkflowInstanceState =
  | "pending"
  | "running"
  | "waiting_for_approval"
  | "waiting_for_operator"
  | "succeeded"
  | "failed"
  | "cancelled";

export type WorkflowInstanceRecord = {
  id: number;
  caseId: number;
  workflowId: string;
  workflowVersion: number;
  state: WorkflowInstanceState;
  currentStep: string | null;
  createdAt: string;
  updatedAt: string;
  finishedAt: string | null;
};

export type WorkflowJobState = "pending" | "dispatched" | "succeeded" | "failed";

export type WorkflowJobRecord = {
  id: number;
  workflowInstanceId: number;
  position: number;
  stepId: string;
  operationType: string;
  state: WorkflowJobState;
  attempt: number;
  maxAttempts: number;
  createdAt: string;
  updatedAt: string;
  finishedAt: string | null;
};

export async function attachCaseWorkflow(
  id: number,
  workflowId: string,
  workflowVersion: number,
  token: string,
  signal?: AbortSignal,
): Promise<WorkflowInstanceRecord> {
  return request<WorkflowInstanceRecord>(`/api/v1/cases/${id}/workflows`, signal, token, {
    method: "POST",
    body: JSON.stringify({ workflowId, workflowVersion }),
  });
}

export async function loadCaseWorkflows(
  id: number,
  signal?: AbortSignal,
): Promise<WorkflowInstanceRecord[]> {
  return request<WorkflowInstanceRecord[]>(`/api/v1/cases/${id}/workflows`, signal);
}

export type WorkflowApprovalRecord = {
  id: number;
  workflowInstanceId: number;
  gateId: string;
  approverId: string;
  approverDisplayName: string;
  reason: string | null;
  createdAt: string;
};

export async function approveWorkflowGate(
  instanceID: number,
  gateId: string,
  reason: string,
  token: string,
  signal?: AbortSignal,
): Promise<WorkflowApprovalRecord> {
  return request<WorkflowApprovalRecord>(
    `/api/v1/workflow-instances/${instanceID}/approvals`,
    signal,
    token,
    {
      method: "POST",
      body: JSON.stringify(reason === "" ? { gateId } : { gateId, reason }),
    },
  );
}

export type WorkflowTaskCompletionRecord = {
  id: number;
  workflowInstanceId: number;
  taskId: string;
  operatorId: string;
  operatorDisplayName: string;
  note: string | null;
  createdAt: string;
};

export async function completeWorkflowTask(
  instanceID: number,
  taskId: string,
  note: string,
  token: string,
  signal?: AbortSignal,
): Promise<WorkflowTaskCompletionRecord> {
  return request<WorkflowTaskCompletionRecord>(
    `/api/v1/workflow-instances/${instanceID}/task-completions`,
    signal,
    token,
    {
      method: "POST",
      body: JSON.stringify(note === "" ? { taskId } : { taskId, note }),
    },
  );
}

export async function loadCaseNotes(
  id: number,
  signal?: AbortSignal,
): Promise<CaseNote[]> {
  return request<CaseNote[]>(`/api/v1/cases/${id}/notes`, signal);
}

export async function loadWorkflowJobs(
  instanceID: number,
  signal?: AbortSignal,
): Promise<WorkflowJobRecord[]> {
  return request<WorkflowJobRecord[]>(`/api/v1/workflow-instances/${instanceID}/jobs`, signal);
}

async function request<T>(
  path: string,
  signal?: AbortSignal,
  token?: string,
  init?: RequestInit,
): Promise<T> {
  const headers: Record<string, string> = { Accept: "application/json" };
  if (token) {
    headers.Authorization = `Bearer ${token}`;
  }
  if (init?.body) {
    headers["Content-Type"] = "application/json";
  }
  const response = await fetch(path, { ...init, headers, signal });

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

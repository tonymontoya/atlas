import {
  loadCase,
  loadCaseNotes,
  loadCaseTimeline,
  loadCaseWorkflows,
  loadWorkflowJobs,
  type CaseNote,
  type CaseRecord,
  type TimelineEvent,
  type WorkflowInstanceRecord,
  type WorkflowJobRecord,
} from "./api";
import { useResource } from "./resources";

export type CaseDetailState = {
  detail: CaseRecord | null;
  detailError: string | null;
  detailLoading: boolean;
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
};

// useCaseDetail loads one Case with its Timeline Events, Notes, and
// Workflow Instances (plus per-instance Jobs) whenever the selection
// or the reload key changes. Each section is an independent resource:
// a null selection idles every fetcher, and a failed section shows its
// own error without blanking the others. The Jobs fan-out is its own
// resource that runs once the Workflow Instances are known.
export function useCaseDetail(caseID: number | null, reloadKey: number): CaseDetailState {
  const detail = useResource(
    caseID === null ? null : (signal) => loadCase(caseID, signal),
    [caseID, reloadKey],
  );
  const timeline = useResource(
    caseID === null ? null : (signal) => loadCaseTimeline(caseID, signal),
    [caseID, reloadKey],
  );
  const notes = useResource(
    caseID === null ? null : (signal) => loadCaseNotes(caseID, signal),
    [caseID, reloadKey],
  );
  const workflows = useResource(
    caseID === null ? null : (signal) => loadCaseWorkflows(caseID, signal),
    [caseID, reloadKey],
  );

  const workflowInstances = workflows.data;
  const jobs = useResource(
    workflowInstances === null
      ? null
      : (signal) =>
          Promise.all(
            workflowInstances.map(async (instance) => {
              try {
                const loaded = await loadWorkflowJobs(instance.id, signal);
                return [instance.id, loaded] as const;
              } catch {
                return [instance.id, []] as const;
              }
            }),
          ).then((entries) => Object.fromEntries(entries)),
    [workflowInstances],
  );

  return {
    detail: detail.data,
    detailError: detail.error,
    detailLoading: detail.loading,
    timeline: timeline.data ?? [],
    timelineError: timeline.error,
    timelineLoading: timeline.loading,
    notes: notes.data ?? [],
    notesError: notes.error,
    notesLoading: notes.loading,
    workflows: workflows.data ?? [],
    workflowJobs: jobs.data ?? {},
    workflowsError: workflows.error,
    workflowsLoading: workflows.loading,
  };
}

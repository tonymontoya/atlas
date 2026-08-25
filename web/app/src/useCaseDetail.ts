import React from "react";
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
import { errorMessage } from "./format";

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
// Workflow Instances (plus per-instance Jobs) whenever the selection or
// the reload counter changes. Writes bump the reload counter through the
// returned refresh function so every section re-reads together.
export function useCaseDetail(caseID: number | null, reloadKey: number): CaseDetailState {
  const [detail, setDetail] = React.useState<CaseRecord | null>(null);
  const [detailError, setDetailError] = React.useState<string | null>(null);
  const [detailLoading, setDetailLoading] = React.useState(false);
  const [timeline, setTimeline] = React.useState<TimelineEvent[]>([]);
  const [timelineError, setTimelineError] = React.useState<string | null>(null);
  const [timelineLoading, setTimelineLoading] = React.useState(false);
  const [notes, setNotes] = React.useState<CaseNote[]>([]);
  const [notesError, setNotesError] = React.useState<string | null>(null);
  const [notesLoading, setNotesLoading] = React.useState(false);
  const [workflows, setWorkflows] = React.useState<WorkflowInstanceRecord[]>([]);
  const [workflowJobs, setWorkflowJobs] = React.useState<Record<number, WorkflowJobRecord[]>>({});
  const [workflowsError, setWorkflowsError] = React.useState<string | null>(null);
  const [workflowsLoading, setWorkflowsLoading] = React.useState(false);

  React.useEffect(() => {
    if (caseID === null) {
      setDetail(null);
      setDetailError(null);
      setDetailLoading(false);
      setTimeline([]);
      setTimelineError(null);
      setTimelineLoading(false);
      setNotes([]);
      setNotesError(null);
      setNotesLoading(false);
      setWorkflows([]);
      setWorkflowJobs({});
      setWorkflowsError(null);
      setWorkflowsLoading(false);
      return;
    }

    const id = caseID;
    const controller = new AbortController();

    async function loadSelectedCase() {
      try {
        setDetailLoading(true);
        setTimelineLoading(true);
        setNotesLoading(true);
        setWorkflowsLoading(true);
        setDetailError(null);
        setTimelineError(null);
        setNotesError(null);
        setWorkflowsError(null);

        const [detailResult, timelineResult, notesResult, workflowsResult] = await Promise.all([
          loadCase(id, controller.signal).then(
            (loaded) => ({ ok: true as const, loaded }),
            (error: unknown) => ({ ok: false as const, error }),
          ),
          loadCaseTimeline(id, controller.signal).then(
            (loaded) => ({ ok: true as const, loaded }),
            (error: unknown) => ({ ok: false as const, error }),
          ),
          loadCaseNotes(id, controller.signal).then(
            (loaded) => ({ ok: true as const, loaded }),
            (error: unknown) => ({ ok: false as const, error }),
          ),
          loadCaseWorkflows(id, controller.signal).then(
            (loaded) => ({ ok: true as const, loaded }),
            (error: unknown) => ({ ok: false as const, error }),
          ),
        ]);

        if (controller.signal.aborted) {
          return;
        }

        if (detailResult.ok) {
          setDetail(detailResult.loaded);
        } else {
          setDetail(null);
          setDetailError(errorMessage(detailResult.error));
          setTimeline([]);
          setTimelineError(null);
          setNotes([]);
          setNotesError(null);
          setWorkflows([]);
          setWorkflowJobs({});
          setWorkflowsError(null);
          return;
        }

        if (timelineResult.ok) {
          setTimeline(timelineResult.loaded);
        } else {
          setTimeline([]);
          setTimelineError(errorMessage(timelineResult.error));
        }

        if (notesResult.ok) {
          setNotes(notesResult.loaded);
        } else {
          setNotes([]);
          setNotesError(errorMessage(notesResult.error));
        }

        if (workflowsResult.ok) {
          setWorkflows(workflowsResult.loaded);
          const jobEntries = await Promise.all(
            workflowsResult.loaded.map(async (instance) => {
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
          setWorkflowJobs(Object.fromEntries(jobEntries));
        } else {
          setWorkflows([]);
          setWorkflowJobs({});
          setWorkflowsError(errorMessage(workflowsResult.error));
        }
      } catch (loadError) {
        if (controller.signal.aborted) {
          return;
        }
        setDetail(null);
        setTimeline([]);
        setNotes([]);
        setWorkflows([]);
        setWorkflowJobs({});
        setDetailError(errorMessage(loadError));
        setTimelineError(errorMessage(loadError));
        setNotesError(errorMessage(loadError));
        setWorkflowsError(errorMessage(loadError));
      } finally {
        if (!controller.signal.aborted) {
          setDetailLoading(false);
          setTimelineLoading(false);
          setNotesLoading(false);
          setWorkflowsLoading(false);
        }
      }
    }

    void loadSelectedCase();

    return () => controller.abort();
  }, [caseID, reloadKey]);

  return {
    detail,
    detailError,
    detailLoading,
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
  };
}

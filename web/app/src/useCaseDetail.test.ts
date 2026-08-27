// @vitest-environment jsdom
import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { useCaseDetail } from "./useCaseDetail";
import * as api from "./api";
import type { CaseRecord, WorkflowInstanceRecord } from "./api";

vi.mock("./api", () => ({
  loadCase: vi.fn(),
  loadCaseNotes: vi.fn(),
  loadCaseTimeline: vi.fn(),
  loadCaseWorkflows: vi.fn(),
  loadWorkflowJobs: vi.fn(),
}));

const loadCase = vi.mocked(api.loadCase);
const loadCaseNotes = vi.mocked(api.loadCaseNotes);
const loadCaseTimeline = vi.mocked(api.loadCaseTimeline);
const loadCaseWorkflows = vi.mocked(api.loadCaseWorkflows);
const loadWorkflowJobs = vi.mocked(api.loadWorkflowJobs);

const CASE: CaseRecord = {
  id: 11,
  title: "OSD down requires triage",
  summary: "one OSD is down",
  status: "detected",
  severity: "high",
  source: "manual",
  clusterFsid: "00000000-0000-4000-8000-000000000101",
  createdAt: "2026-08-13T12:00:00Z",
  updatedAt: "2026-08-13T12:00:00Z",
};

function instance(id: number, state = "succeeded"): WorkflowInstanceRecord {
  return {
    id,
    caseId: CASE.id,
    workflowId: "replace-osd",
    workflowVersion: 1,
    state: state as WorkflowInstanceRecord["state"],
    currentStep: null,
    createdAt: "2026-08-13T12:00:00Z",
    updatedAt: "2026-08-13T12:00:00Z",
    finishedAt: null,
  };
}

function mockSectionLoads(instances: WorkflowInstanceRecord[]) {
  loadCase.mockResolvedValue(CASE);
  loadCaseNotes.mockResolvedValue([]);
  loadCaseTimeline.mockResolvedValue([]);
  loadCaseWorkflows.mockResolvedValue(instances);
}

afterEach(() => {
  vi.clearAllMocks();
});

describe("useCaseDetail", () => {
  it("idles with no selection and fetches nothing", () => {
    const { result } = renderHook(() => useCaseDetail(null, 0));

    expect(result.current.detail).toBeNull();
    expect(result.current.detailLoading).toBe(false);
    expect(result.current.workflows).toEqual([]);
    expect(loadCase).not.toHaveBeenCalled();
  });

  it("loads every section for the selected Case", async () => {
    mockSectionLoads([instance(71)]);

    const { result } = renderHook(() => useCaseDetail(11, 0));
    await waitFor(() => {
      expect(result.current.detailLoading).toBe(false);
    });

    expect(result.current.detail).toEqual(CASE);
    expect(result.current.detailError).toBeNull();
    expect(result.current.workflows).toHaveLength(1);
  });

  it("assembles jobs keyed by instance id for many instances", async () => {
    mockSectionLoads([instance(71), instance(72), instance(73)]);
    loadWorkflowJobs.mockImplementation(async (instanceID: number) => {
      if (instanceID === 72) {
        throw new Error("jobs unavailable");
      }
      return [
        {
          id: instanceID * 10,
          workflowInstanceId: instanceID,
          position: 1,
          stepId: "zap",
          operationType: "storage.device.zap",
          state: "succeeded",
          attempt: 1,
          maxAttempts: 3,
          createdAt: "2026-08-13T12:00:00Z",
          updatedAt: "2026-08-13T12:00:00Z",
          finishedAt: null,
        },
      ];
    });

    const { result } = renderHook(() => useCaseDetail(11, 0));
    await waitFor(() => {
      expect(result.current.workflowJobs[71]).toHaveLength(1);
      expect(result.current.workflowJobs[73]).toHaveLength(1);
    });

    expect(Object.keys(result.current.workflowJobs).sort()).toEqual(["71", "72", "73"]);
    expect(result.current.workflowJobs[72]).toEqual([]);
  });

  it("maps jobs to an empty record with no workflow instances", async () => {
    mockSectionLoads([]);

    const { result } = renderHook(() => useCaseDetail(11, 0));
    await waitFor(() => {
      expect(result.current.workflowsLoading).toBe(false);
    });

    expect(result.current.workflowJobs).toEqual({});
    expect(loadWorkflowJobs).not.toHaveBeenCalled();
  });

  it("keeps section errors independent", async () => {
    loadCase.mockResolvedValue(CASE);
    loadCaseNotes.mockRejectedValue(new Error("notes unavailable"));
    loadCaseTimeline.mockResolvedValue([]);
    loadCaseWorkflows.mockResolvedValue([]);

    const { result } = renderHook(() => useCaseDetail(11, 0));
    await waitFor(() => {
      expect(result.current.notesError).toBe("notes unavailable");
    });

    expect(result.current.detailError).toBeNull();
    expect(result.current.timelineError).toBeNull();
    expect(result.current.workflowsError).toBeNull();
  });

  it("refetches every section when the reload key bumps", async () => {
    mockSectionLoads([]);

    const { result, rerender } = renderHook(
      ({ caseID, reloadKey }) => useCaseDetail(caseID, reloadKey),
      { initialProps: { caseID: 11, reloadKey: 0 } },
    );
    await waitFor(() => {
      expect(result.current.workflowsLoading).toBe(false);
    });
    expect(loadCase).toHaveBeenCalledTimes(1);

    await act(async () => {
      rerender({ caseID: 11, reloadKey: 1 });
    });
    await waitFor(() => {
      expect(loadCase).toHaveBeenCalledTimes(2);
    });
    expect(loadCaseWorkflows).toHaveBeenCalledTimes(2);
  });
});

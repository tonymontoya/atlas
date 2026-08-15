import { describe, expect, it } from "vitest";

import type { CaseDetectionLink, TimelineEvent, TimelineEventType } from "./api";
import { detectionLabel, labelForTimelineEventType, timelinePayloadLabels } from "./timeline";

describe("detectionLabel", () => {
  it("renders a normalized detection source and alert name", () => {
    const detectedBy: CaseDetectionLink = {
      source: "prometheus",
      alertName: "CephOSDDown",
      signal: "CEPH_OSD_DOWN",
      firstSeenAt: "2026-08-14T09:20:00Z",
      lastSeenAt: "2026-08-14T09:25:00Z",
    };
    expect(detectionLabel(detectedBy)).toBe("Detected by Prometheus · CephOSDDown");
  });
});

describe("labelForTimelineEventType", () => {
  it("returns operator-readable labels for timeline event types", () => {
    const cases: Array<[TimelineEventType, string]> = [
      ["case_detected", "Case detected"],
      ["case_triaged", "Case triaged"],
      ["case_status_changed", "Status changed"],
      ["case_note_added", "Note added"],
      ["case_assigned", "Assignment changed"],
      ["workflow_attached", "Workflow attached"],
      ["workflow_state_changed", "Workflow state changed"],
    ];

    for (const [type, label] of cases) {
      expect(labelForTimelineEventType(type)).toBe(label);
    }
  });
});

describe("timelinePayloadLabels", () => {
  it("returns normalized context labels without exposing raw payload JSON", () => {
    expect(
      timelinePayloadLabels(
        timelineEvent({
          source: "manual",
          previousStatus: "detected",
          newStatus: "triaged",
          workflowId: "replace-osd",
          workflowInstanceId: 101,
          noteId: 17,
        }),
      ),
    ).toEqual([
      "manual",
      "detected to triaged",
      "replace-osd",
      "Workflow #101",
      "Note #17",
    ]);
  });

  it("ignores payload values with unexpected shapes", () => {
    expect(
      timelinePayloadLabels(
        timelineEvent({
          source: 12,
          signal: ["OSD_DOWN"],
          previousStatus: "detected",
          newStatus: null,
          workflowInstanceId: "101",
        }),
      ),
    ).toEqual([]);
  });

  it("labels workflow state changes with pause context", () => {
    expect(
      timelinePayloadLabels(
        timelineEvent({
          previousState: "running",
          newState: "waiting_for_approval",
          pausedAtStep: "approve-destroy",
        }),
      ),
    ).toEqual(["running to waiting_for_approval", "Paused at approve-destroy"]);
    expect(
      timelinePayloadLabels(
        timelineEvent({
          previousState: "waiting_for_approval",
          newState: "running",
        }),
      ),
    ).toEqual(["waiting_for_approval to running"]);
  });

  it("labels assignment, reassignment, and unassignment events", () => {
    expect(
      timelinePayloadLabels(
        assignmentEvent({ previousAssignee: null, newAssignee: "operator-2" }),
      ),
    ).toEqual(["assigned to operator-2"]);
    expect(
      timelinePayloadLabels(
        assignmentEvent({ previousAssignee: "operator-1", newAssignee: "operator-2" }),
      ),
    ).toEqual(["operator-1 to operator-2"]);
    expect(
      timelinePayloadLabels(
        assignmentEvent({ previousAssignee: "operator-1", newAssignee: null }),
      ),
    ).toEqual(["unassigned"]);
  });
});

function assignmentEvent(payload: Record<string, unknown>): TimelineEvent {
  return {
    id: 4,
    caseId: 2,
    type: "case_assigned",
    message: "Case assigned to Second Operator.",
    occurredAt: "2026-08-13T12:15:00Z",
    actor: {
      type: "user",
      displayName: "Storage Operator",
    },
    payload,
  };
}

function timelineEvent(payload: Record<string, unknown>): TimelineEvent {
  return {
    id: 1,
    caseId: 2,
    type: "case_detected",
    message: "Case detected.",
    occurredAt: "2026-08-13T12:00:00Z",
    actor: {
      type: "system",
      displayName: "Atlas",
    },
    payload,
  };
}

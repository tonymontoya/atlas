import type { CaseDetectionLink, TimelineEvent, TimelineEventType } from "./api";

export function labelForTimelineEventType(type: TimelineEventType): string {
  switch (type) {
    case "case_detected":
      return "Case detected";
    case "case_triaged":
      return "Case triaged";
    case "case_status_changed":
      return "Status changed";
    case "case_note_added":
      return "Note added";
    case "case_assigned":
      return "Assignment changed";
    case "workflow_attached":
      return "Workflow attached";
    case "workflow_state_changed":
      return "Workflow state changed";
  }
}

export function timelinePayloadLabels(event: TimelineEvent): string[] {
  const labels: string[] = [];
  const payload = event.payload;

  if (typeof payload.source === "string") {
    labels.push(payload.source);
  }
  if (typeof payload.signal === "string") {
    labels.push(payload.signal);
  }
  if (typeof payload.previousStatus === "string" && typeof payload.newStatus === "string") {
    labels.push(`${payload.previousStatus} to ${payload.newStatus}`);
  }
  if (typeof payload.previousState === "string" && typeof payload.newState === "string") {
    labels.push(`${payload.previousState} to ${payload.newState}`);
  }
  if (typeof payload.pausedAtStep === "string") {
    labels.push(`Paused at ${payload.pausedAtStep}`);
  }
  if (typeof payload.workflowId === "string") {
    labels.push(payload.workflowId);
  }
  if (typeof payload.workflowInstanceId === "number") {
    labels.push(`Workflow #${payload.workflowInstanceId}`);
  }
  if (typeof payload.noteId === "number") {
    labels.push(`Note #${payload.noteId}`);
  }
  if (event.type === "case_assigned") {
    labels.push(assignmentPayloadLabel(payload));
  }

  return labels;
}

export function assignmentPayloadLabel(payload: Record<string, unknown>): string {
  const previous = assigneeLabel(payload.previousAssignee);
  const next = assigneeLabel(payload.newAssignee);
  if (previous === null && next !== null) {
    return `assigned to ${next}`;
  }
  if (previous !== null && next === null) {
    return "unassigned";
  }
  if (previous !== null && next !== null) {
    return `${previous} to ${next}`;
  }
  return "assignment unchanged";
}

function assigneeLabel(value: unknown): string | null {
  return typeof value === "string" && value !== "" ? value : null;
}

export function detectionLabel(detectedBy: CaseDetectionLink): string {
  const source = detectedBy.source.charAt(0).toUpperCase() + detectedBy.source.slice(1);
  return `Detected by ${source} · ${detectedBy.alertName}`;
}

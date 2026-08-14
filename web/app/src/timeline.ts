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
  if (typeof payload.workflowId === "string") {
    labels.push(payload.workflowId);
  }
  if (typeof payload.workflowInstanceId === "number") {
    labels.push(`Workflow #${payload.workflowInstanceId}`);
  }
  if (typeof payload.noteId === "number") {
    labels.push(`Note #${payload.noteId}`);
  }

  return labels;
}

export function detectionLabel(detectedBy: CaseDetectionLink): string {
  const source = detectedBy.source.charAt(0).toUpperCase() + detectedBy.source.slice(1);
  return `Detected by ${source} · ${detectedBy.alertName}`;
}

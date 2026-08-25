// Tone vocabulary shared by every status surface in the UI. Tones map onto
// Carbon Tag types; `warn` has no Carbon Tag type, so it carries a
// token-backed className instead (see styles.css `atlas-tag-warn`).
export type Tone = "ok" | "warn" | "err" | "neutral";

export type TagKind = "green" | "red" | "gray";

export function tagTypeFor(tone: Tone): TagKind {
  switch (tone) {
    case "ok":
      return "green";
    case "err":
      return "red";
    case "warn":
    case "neutral":
      return "gray";
  }
}

export function tagClassNameFor(tone: Tone): string | undefined {
  return tone === "warn" ? "atlas-tag-warn" : undefined;
}

export function toneForHealth(status: string | null | undefined): Tone {
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

export function toneForSyncRunStatus(status: "running" | "succeeded" | "failed"): Tone {
  if (status === "succeeded") {
    return "ok";
  }
  if (status === "failed") {
    return "err";
  }
  return "warn";
}

export function toneForCaseStatus(status: "detected" | "triaged" | "closed"): Tone {
  if (status === "closed") {
    return "ok";
  }
  if (status === "detected") {
    return "warn";
  }
  return "neutral";
}

export function toneForCaseSeverity(
  severity: "info" | "low" | "medium" | "high" | "critical",
): Tone {
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

export function toneForWorkflowState(
  state:
    | "pending"
    | "running"
    | "waiting_for_approval"
    | "waiting_for_operator"
    | "succeeded"
    | "failed"
    | "cancelled",
): Tone {
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

export function toneForJobState(
  state: "pending" | "dispatched" | "succeeded" | "failed",
): Tone {
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

export function toneForDeviceHealth(health: string | undefined): Tone {
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

export function toneForDaemonStatus(status: string): Tone {
  return status === "running" ? "ok" : "err";
}

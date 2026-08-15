import { describe, expect, it } from "vitest";

import type { CaseRecord } from "./api";
import { availableCaseActions } from "./caseActions";

describe("availableCaseActions", () => {
  it("allows triage, close, and workflow attach from detected", () => {
    expect(availableCaseActions("detected")).toEqual({
      canTriage: true,
      canClose: true,
      isClosed: false,
      canAssign: true,
      canAttachWorkflow: true,
    });
  });

  it("allows close and workflow attach but not re-triage from triaged", () => {
    expect(availableCaseActions("triaged")).toEqual({
      canTriage: false,
      canClose: true,
      isClosed: false,
      canAssign: true,
      canAttachWorkflow: true,
    });
  });

  it("treats closed as terminal, including workflow attach", () => {
    expect(availableCaseActions("closed")).toEqual({
      canTriage: false,
      canClose: false,
      isClosed: true,
      canAssign: false,
      canAttachWorkflow: false,
    });
  });

  it("covers every case status", () => {
    const statuses: CaseRecord["status"][] = ["detected", "triaged", "closed"];
    for (const status of statuses) {
      expect(() => availableCaseActions(status)).not.toThrow();
    }
  });
});

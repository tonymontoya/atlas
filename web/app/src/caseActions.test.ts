import { describe, expect, it } from "vitest";

import type { CaseRecord } from "./api";
import { availableCaseActions } from "./caseActions";

describe("availableCaseActions", () => {
  it("allows triage and close from detected", () => {
    expect(availableCaseActions("detected")).toEqual({
      canTriage: true,
      canClose: true,
      isClosed: false,
      canAssign: true,
    });
  });

  it("allows close but not re-triage from triaged", () => {
    expect(availableCaseActions("triaged")).toEqual({
      canTriage: false,
      canClose: true,
      isClosed: false,
      canAssign: true,
    });
  });

  it("treats closed as terminal", () => {
    expect(availableCaseActions("closed")).toEqual({
      canTriage: false,
      canClose: false,
      isClosed: true,
      canAssign: false,
    });
  });

  it("covers every case status", () => {
    const statuses: CaseRecord["status"][] = ["detected", "triaged", "closed"];
    for (const status of statuses) {
      expect(() => availableCaseActions(status)).not.toThrow();
    }
  });
});

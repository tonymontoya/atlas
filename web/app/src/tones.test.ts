import { describe, expect, it } from "vitest";
import {
  tagClassNameFor,
  tagTypeFor,
  toneForCaseSeverity,
  toneForCaseStatus,
  toneForDaemonStatus,
  toneForDeviceHealth,
  toneForHealth,
  toneForJobState,
  toneForSyncRunStatus,
  toneForWorkflowState,
} from "./tones";
import type { Tone } from "./tones";

describe("tagTypeFor", () => {
  it("maps every tone to a Carbon Tag type", () => {
    expect(tagTypeFor("ok")).toBe("green");
    expect(tagTypeFor("err")).toBe("red");
    expect(tagTypeFor("warn")).toBe("gray");
    expect(tagTypeFor("neutral")).toBe("gray");
  });
});

describe("tagClassNameFor", () => {
  it("flags only warn tones for the token-backed warn tag", () => {
    expect(tagClassNameFor("warn")).toBe("atlas-tag-warn");
    expect(tagClassNameFor("ok")).toBeUndefined();
    expect(tagClassNameFor("err")).toBeUndefined();
    expect(tagClassNameFor("neutral")).toBeUndefined();
  });
});

describe("toneForHealth", () => {
  it("keeps Ceph health statuses distinct and treats missing health as neutral", () => {
    expect(toneForHealth("HEALTH_OK")).toBe<Tone>("ok");
    expect(toneForHealth("HEALTH_WARN")).toBe<Tone>("warn");
    expect(toneForHealth("HEALTH_ERR")).toBe<Tone>("err");
    expect(toneForHealth(null)).toBe<Tone>("neutral");
    expect(toneForHealth(undefined)).toBe<Tone>("neutral");
    expect(toneForHealth("WHATEVER")).toBe<Tone>("neutral");
  });
});

describe("toneForSyncRunStatus", () => {
  it("maps sync run statuses", () => {
    expect(toneForSyncRunStatus("succeeded")).toBe<Tone>("ok");
    expect(toneForSyncRunStatus("failed")).toBe<Tone>("err");
    expect(toneForSyncRunStatus("running")).toBe<Tone>("warn");
  });
});

describe("toneForCaseStatus", () => {
  it("maps case statuses with closed reading healthy", () => {
    expect(toneForCaseStatus("closed")).toBe<Tone>("ok");
    expect(toneForCaseStatus("detected")).toBe<Tone>("warn");
    expect(toneForCaseStatus("triaged")).toBe<Tone>("neutral");
  });
});

describe("toneForCaseSeverity", () => {
  it("maps severities monotonically", () => {
    expect(toneForCaseSeverity("critical")).toBe<Tone>("err");
    expect(toneForCaseSeverity("high")).toBe<Tone>("err");
    expect(toneForCaseSeverity("medium")).toBe<Tone>("warn");
    expect(toneForCaseSeverity("low")).toBe<Tone>("neutral");
    expect(toneForCaseSeverity("info")).toBe<Tone>("ok");
  });
});

describe("toneForWorkflowState", () => {
  it("maps every workflow instance state", () => {
    expect(toneForWorkflowState("succeeded")).toBe<Tone>("ok");
    expect(toneForWorkflowState("failed")).toBe<Tone>("err");
    expect(toneForWorkflowState("cancelled")).toBe<Tone>("err");
    expect(toneForWorkflowState("running")).toBe<Tone>("neutral");
    expect(toneForWorkflowState("pending")).toBe<Tone>("neutral");
    expect(toneForWorkflowState("waiting_for_approval")).toBe<Tone>("warn");
    expect(toneForWorkflowState("waiting_for_operator")).toBe<Tone>("warn");
  });
});

describe("toneForJobState", () => {
  it("maps every workflow job state", () => {
    expect(toneForJobState("succeeded")).toBe<Tone>("ok");
    expect(toneForJobState("failed")).toBe<Tone>("err");
    expect(toneForJobState("pending")).toBe<Tone>("neutral");
    expect(toneForJobState("dispatched")).toBe<Tone>("neutral");
  });
});

describe("toneForDeviceHealth", () => {
  it("maps device health strings and unknowns", () => {
    expect(toneForDeviceHealth("ok")).toBe<Tone>("ok");
    expect(toneForDeviceHealth("warning")).toBe<Tone>("warn");
    expect(toneForDeviceHealth("error")).toBe<Tone>("err");
    expect(toneForDeviceHealth(undefined)).toBe<Tone>("neutral");
    expect(toneForDeviceHealth("something-else")).toBe<Tone>("neutral");
  });
});

describe("toneForDaemonStatus", () => {
  it("gives all five daemon statuses a deliberate tone", () => {
    expect(toneForDaemonStatus("running")).toBe<Tone>("ok");
    expect(toneForDaemonStatus("stopped")).toBe<Tone>("err");
    expect(toneForDaemonStatus("error")).toBe<Tone>("err");
    expect(toneForDaemonStatus("starting")).toBe<Tone>("warn");
    expect(toneForDaemonStatus("unknown")).toBe<Tone>("neutral");
  });

  it("reads unrecognized statuses as unknown, not as failures", () => {
    expect(toneForDaemonStatus("something-else")).toBe<Tone>("neutral");
  });
});

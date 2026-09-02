import { describe, expect, it } from "vitest";

import {
  agentLastPushLabel,
  agentLastSeenLabel,
  clusterFsidFromPath,
  clusterRoute,
  healthStatusLabel,
  offsetForPage,
} from "./clusters";
import type { ClusterSummary, InventorySyncRun } from "./api";

const NOW = Date.parse("2026-08-25T12:00:00Z");

function summary(overrides: Partial<ClusterSummary> = {}): ClusterSummary {
  return {
    id: 1,
    fsid: "00000000-0000-4000-8000-000000000101",
    name: "fixture-healthy",
    clusterType: "bare-metal",
    cephVersion: "18.2.4",
    healthStatus: "HEALTH_OK",
    healthSummary: "ok",
    agentLastSeen: null,
    ...overrides,
  };
}

describe("healthStatusLabel", () => {
  it("strips the HEALTH_ prefix and labels missing observations", () => {
    expect(healthStatusLabel("HEALTH_OK")).toBe("OK");
    expect(healthStatusLabel("HEALTH_WARN")).toBe("WARN");
    expect(healthStatusLabel("HEALTH_ERR")).toBe("ERR");
    expect(healthStatusLabel(null)).toBe("No observation yet");
  });
});

describe("agentLastSeenLabel", () => {
  it("says never when no agent batch has arrived", () => {
    expect(agentLastSeenLabel(null, NOW)).toBe("never");
  });

  it("buckets recency", () => {
    expect(agentLastSeenLabel("2026-08-25T11:59:40Z", NOW)).toBe("just now");
    expect(agentLastSeenLabel("2026-08-25T11:30:00Z", NOW)).toBe("30m ago");
    expect(agentLastSeenLabel("2026-08-25T07:00:00Z", NOW)).toBe("5h ago");
    expect(agentLastSeenLabel("2026-08-22T12:00:00Z", NOW)).toBe("3d ago");
    expect(agentLastSeenLabel("2026-07-01T12:00:00Z", NOW)).toBe("Jul 1, 2026");
  });

  it("treats future timestamps and garbage defensively", () => {
    expect(agentLastSeenLabel("2026-08-25T12:00:10Z", NOW)).toBe("just now");
    expect(agentLastSeenLabel("not-a-date", NOW)).toBe("unknown");
  });
});

describe("pagination translation", () => {
  it("turns pages into offsets", () => {
    expect(offsetForPage(1, 20)).toBe(0);
    expect(offsetForPage(3, 20)).toBe(40);
  });

  it("degrades to the first page for degenerate inputs", () => {
    expect(offsetForPage(0, 20)).toBe(0);
    expect(offsetForPage(2, 0)).toBe(0);
    expect(offsetForPage(-1, 20)).toBe(0);
  });
});

describe("clusterRoute", () => {
  it("addresses clusters by FSID and leaves unenrolled clusters unaddressable", () => {
    expect(clusterRoute(summary())).toBe(
      "/clusters/00000000-0000-4000-8000-000000000101",
    );
    expect(clusterRoute(summary({ fsid: null }))).toBeNull();
  });
});

describe("agentLastPushLabel", () => {
  it("says never when no agent run is in the history", () => {
    expect(agentLastPushLabel([], NOW)).toBe("never");
    expect(
      agentLastPushLabel([run({ provider: "fake", startedAt: "2026-08-25T11:00:00Z" })], NOW),
    ).toBe("never");
  });

  it("labels the latest agent run — runs arrive newest first", () => {
    expect(
      agentLastPushLabel(
        [
          run({ provider: "agent", startedAt: "2026-08-25T11:30:00Z" }),
          run({ provider: "agent", startedAt: "2026-08-25T07:00:00Z" }),
          run({ provider: "fake", startedAt: "2026-08-25T11:00:00Z" }),
        ],
        NOW,
      ),
    ).toBe("30m ago");
  });

  it("treats garbage timestamps defensively", () => {
    expect(agentLastPushLabel([run({ provider: "agent", startedAt: "not-a-date" })], NOW)).toBe(
      "unknown",
    );
  });
});

describe("clusterFsidFromPath", () => {
  it("reads the selected cluster from every cluster-scoped route", () => {
    expect(clusterFsidFromPath("/clusters/00000000-0000-4000-8000-000000000101")).toBe(
      "00000000-0000-4000-8000-000000000101",
    );
    expect(
      clusterFsidFromPath("/clusters/00000000-0000-4000-8000-000000000101/cases"),
    ).toBe("00000000-0000-4000-8000-000000000101");
    expect(
      clusterFsidFromPath("/clusters/00000000-0000-4000-8000-000000000101/sync-runs"),
    ).toBe("00000000-0000-4000-8000-000000000101");
  });

  it("reads no selection off the cluster tree", () => {
    expect(clusterFsidFromPath("/")).toBeNull();
    expect(clusterFsidFromPath("/cases")).toBeNull();
    expect(clusterFsidFromPath("/sync-runs")).toBeNull();
    expect(clusterFsidFromPath("/clusters/")).toBeNull();
  });
});

function run(overrides: Partial<InventorySyncRun>): InventorySyncRun {
  return {
    id: 1,
    provider: "agent",
    status: "succeeded",
    startedAt: "2026-08-25T11:30:00Z",
    ...overrides,
  };
}

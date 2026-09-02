import { describe, expect, it } from "vitest";

import type { Daemon, Pool, StorageDevice } from "./api";
import {
  daemonStatusCounts,
  daemonStatusSummary,
  notRunningDaemonCount,
  poolRedundancyLabel,
  storageDeviceOSDLabel,
} from "./inventory";

describe("storageDeviceOSDLabel", () => {
  it("labels the currently backing OSD identity", () => {
    expect(storageDeviceOSDLabel(device({ osdId: 3 }))).toBe("OSD 3");
  });

  it("labels devices without an OSD link as spare capacity", () => {
    expect(storageDeviceOSDLabel(device({}))).toBe("no OSD");
  });
});

describe("daemonStatusCounts", () => {
  it("tallies every one of the five Ceph Daemon statuses deliberately", () => {
    const daemons: Daemon[] = [
      daemon({ name: "mon.a", status: "running" }),
      daemon({ name: "mgr.a", status: "running" }),
      daemon({ name: "osd.1", status: "stopped" }),
      daemon({ name: "osd.2", status: "starting" }),
      daemon({ name: "mds.a", status: "error" }),
      daemon({ name: "rgw.a", status: "unknown" }),
    ];

    expect(daemonStatusCounts(daemons)).toEqual({
      total: 6,
      running: 2,
      stopped: 1,
      starting: 1,
      error: 1,
      unknown: 1,
    });
    expect(daemonStatusCounts([])).toEqual({
      total: 0,
      running: 0,
      stopped: 0,
      starting: 0,
      error: 0,
      unknown: 0,
    });
  });

  it("counts stopped, starting, error, and unknown as not running", () => {
    const daemons: Daemon[] = [
      daemon({ name: "mon.a", status: "running" }),
      daemon({ name: "osd.1", status: "stopped" }),
      daemon({ name: "osd.2", status: "starting" }),
      daemon({ name: "mds.a", status: "error" }),
      daemon({ name: "rgw.a", status: "unknown" }),
    ];

    expect(notRunningDaemonCount(daemons)).toBe(4);
    expect(notRunningDaemonCount([])).toBe(0);
  });
});

describe("daemonStatusSummary", () => {
  it("reads an all-running fleet as healthy", () => {
    const counts = daemonStatusCounts([
      daemon({ name: "mon.a", status: "running" }),
      daemon({ name: "mgr.a", status: "running" }),
    ]);
    expect(daemonStatusSummary(counts)).toBe("all running");
  });

  it("lists the non-zero not-running statuses in reading order", () => {
    const counts = daemonStatusCounts([
      daemon({ name: "mon.a", status: "running" }),
      daemon({ name: "osd.1", status: "stopped" }),
      daemon({ name: "osd.2", status: "stopped" }),
      daemon({ name: "mds.a", status: "error" }),
      daemon({ name: "rgw.a", status: "unknown" }),
    ]);
    expect(daemonStatusSummary(counts)).toBe("2 stopped, 1 error, 1 unknown");
  });

  it("names an empty daemon list honestly", () => {
    expect(daemonStatusSummary(daemonStatusCounts([]))).toBe("none observed");
  });

  it("keeps starting visible as its own transitional state", () => {
    const counts = daemonStatusCounts([
      daemon({ name: "mon.a", status: "starting" }),
    ]);
    expect(daemonStatusSummary(counts)).toBe("1 starting");
  });

  it("counts unrecognized statuses as not running instead of claiming all running", () => {
    const daemons = [
      daemon({ name: "mon.a", status: "running" }),
      daemon({ name: "mon.b", status: "degraded" as Daemon["status"] }),
    ];
    const counts = daemonStatusCounts(daemons);
    expect(counts.running).toBe(1);
    expect(notRunningDaemonCount(daemons)).toBe(1);
    expect(daemonStatusSummary(counts)).toBe("1 not running");
  });
});

describe("poolRedundancyLabel", () => {
  it("describes replicated pool redundancy", () => {
    expect(poolRedundancyLabel(pool({ type: "replicated", size: 3, minSize: 2 }))).toBe(
      "3 replicas, min 2",
    );
    expect(poolRedundancyLabel(pool({ type: "replicated", size: 3 }))).toBe("3 replicas");
    expect(poolRedundancyLabel(pool({ type: "replicated" }))).toBe("replicated");
  });

  it("describes erasure coded pools without chunk counts", () => {
    expect(poolRedundancyLabel(pool({ type: "erasure", size: 4, minSize: 2 }))).toBe(
      "erasure coded",
    );
  });
});

function device(overrides: Partial<StorageDevice>): StorageDevice {
  return {
    host: "host-a.example.invalid",
    serial: "nvme-serial-a",
    ...overrides,
  };
}

function daemon(overrides: Partial<Daemon>): Daemon {
  return {
    type: "osd",
    name: "osd.0",
    host: "host-a.example.invalid",
    status: "running",
    ...overrides,
  };
}

function pool(overrides: Partial<Pool>): Pool {
  return {
    id: 1,
    name: "device_health_metrics",
    type: "replicated",
    ...overrides,
  };
}

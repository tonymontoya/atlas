import { describe, expect, it } from "vitest";

import type { Daemon, Pool, StorageDevice } from "./api";
import { poolRedundancyLabel, stoppedDaemonCount, storageDeviceOSDLabel } from "./inventory";

describe("storageDeviceOSDLabel", () => {
  it("labels the currently backing OSD identity", () => {
    expect(storageDeviceOSDLabel(device({ osdId: 3 }))).toBe("OSD 3");
  });

  it("labels devices without an OSD link as spare capacity", () => {
    expect(storageDeviceOSDLabel(device({}))).toBe("no OSD");
  });
});

describe("stoppedDaemonCount", () => {
  it("counts stopped Ceph Daemons", () => {
    const daemons: Daemon[] = [
      daemon({ name: "mon.a", status: "running" }),
      daemon({ name: "mgr.a", status: "running" }),
      daemon({ name: "osd.1", status: "stopped" }),
    ];

    expect(stoppedDaemonCount(daemons)).toBe(1);
    expect(stoppedDaemonCount([])).toBe(0);
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

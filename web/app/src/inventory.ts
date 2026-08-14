import type { Daemon, Pool, StorageDevice } from "./api";

export function storageDeviceOSDLabel(device: StorageDevice): string {
  return device.osdId !== undefined ? `OSD ${device.osdId}` : "no OSD";
}

export function stoppedDaemonCount(daemons: Daemon[]): number {
  return daemons.filter((daemon) => daemon.status === "stopped").length;
}

export function poolRedundancyLabel(pool: Pool): string {
  if (pool.type === "erasure") {
    return "erasure coded";
  }
  if (pool.size === undefined) {
    return "replicated";
  }
  if (pool.minSize === undefined) {
    return `${pool.size} replicas`;
  }
  return `${pool.size} replicas, min ${pool.minSize}`;
}

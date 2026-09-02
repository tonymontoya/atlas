import type { Daemon, Pool, StorageDevice } from "./api";

export function storageDeviceOSDLabel(device: StorageDevice): string {
  return device.osdId !== undefined ? `OSD ${device.osdId}` : "no OSD";
}

// Every Ceph Daemon status is tallied deliberately (issue #30): a
// daemon that is not running counts as not running whether it is
// stopped, starting, in error, or unknown — the breakdown keeps the
// distinctions visible instead of folding them into one number.
export type DaemonStatusCounts = {
  total: number;
  running: number;
  stopped: number;
  starting: number;
  error: number;
  unknown: number;
};

export function daemonStatusCounts(daemons: Daemon[]): DaemonStatusCounts {
  const counts: DaemonStatusCounts = {
    total: daemons.length,
    running: 0,
    stopped: 0,
    starting: 0,
    error: 0,
    unknown: 0,
  };
  for (const daemon of daemons) {
    switch (daemon.status) {
      case "running":
      case "stopped":
      case "starting":
      case "error":
      case "unknown":
        counts[daemon.status] += 1;
        break;
    }
  }
  return counts;
}

export function notRunningDaemonCount(daemons: Daemon[]): number {
  const counts = daemonStatusCounts(daemons);
  return counts.total - counts.running;
}

// The inventory tile's detail line: "all running", or the non-zero
// not-running statuses in reading order.
export function daemonStatusSummary(counts: DaemonStatusCounts): string {
  if (counts.total === 0) {
    return "none observed";
  }
  if (counts.running === counts.total) {
    return "all running";
  }
  const breakdown: string[] = [];
  if (counts.stopped > 0) {
    breakdown.push(`${counts.stopped} stopped`);
  }
  if (counts.starting > 0) {
    breakdown.push(`${counts.starting} starting`);
  }
  if (counts.error > 0) {
    breakdown.push(`${counts.error} error`);
  }
  if (counts.unknown > 0) {
    breakdown.push(`${counts.unknown} unknown`);
  }
  // A status outside the five known ones still counts as not running;
  // name it honestly instead of contradicting the tally.
  if (breakdown.length === 0) {
    return `${counts.total - counts.running} not running`;
  }
  return breakdown.join(", ");
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

// Pure helpers for the cluster index surfaces. The API returns nullable
// health and agent activity (a registered cluster may have no observation
// yet); these helpers give every nullable field a stable display label.
import { coarseDuration } from "./format";
import type { ClusterSummary } from "./api";

export function healthStatusLabel(status: ClusterSummary["healthStatus"]): string {
  if (status === null || status === undefined || status === "") {
    return "No observation yet";
  }
  return status.replace("HEALTH_", "");
}

// Buckets for a past timestamp, from "just now" to a formatted date.
function ageLabel(timestamp: string, now: number): string {
  const seen = Date.parse(timestamp);
  if (Number.isNaN(seen)) {
    return "unknown";
  }
  const ago = coarseDuration(now - seen);
  if (ago.seconds < 60) {
    return "just now";
  }
  if (ago.minutes < 60) {
    return `${ago.minutes}m ago`;
  }
  if (ago.hours < 24) {
    return `${ago.hours}h ago`;
  }
  if (ago.days < 7) {
    return `${ago.days}d ago`;
  }
  return new Intl.DateTimeFormat(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
  }).format(new Date(seen));
}

// Buckets for agent activity, from "just now" to "never". Both agent
// timestamps — last-seen (the batch's observation time) and last-push
// (the run's server-side start) — share the buckets.
export function agentActivityLabel(
  activity: string | null | undefined,
  now: number = Date.now(),
): string {
  if (activity === null || activity === undefined || activity === "") {
    return "never";
  }
  return ageLabel(activity, now);
}

// Carbon Pagination is 1-based; the cluster index API takes a 0-based
// offset. Keep the translation in one place.
export function offsetForPage(page: number, pageSize: number): number {
  if (page < 1 || pageSize <= 0) {
    return 0;
  }
  return (page - 1) * pageSize;
}

export function clusterRoute(cluster: ClusterSummary): string | null {
  return cluster.fsid ? `/clusters/${cluster.fsid}` : null;
}

// The selected cluster is the FSID in the URL (/clusters/<fsid> and its
// scoped sections), so no page can show one cluster's data while
// another is selected — the route is the selection.
export function clusterFsidFromPath(pathname: string): string | null {
  const match = /^\/clusters\/([^/]+)(?:\/.*)?$/.exec(pathname);
  return match ? decodeURIComponent(match[1]) : null;
}

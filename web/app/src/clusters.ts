// Pure helpers for the cluster index surfaces. The API returns nullable
// health and agent activity (a registered cluster may have no observation
// yet); these helpers give every nullable field a stable display label.
import type { ClusterSummary } from "./api";

export function healthStatusLabel(status: ClusterSummary["healthStatus"]): string {
  if (status === null || status === undefined || status === "") {
    return "No observation yet";
  }
  return status.replace("HEALTH_", "");
}

// Buckets for agent activity, from "just now" to "never".
export function agentLastSeenLabel(
  lastSeen: ClusterSummary["agentLastSeen"],
  now: number = Date.now(),
): string {
  if (lastSeen === null || lastSeen === undefined || lastSeen === "") {
    return "never";
  }
  const seen = Date.parse(lastSeen);
  if (Number.isNaN(seen)) {
    return "unknown";
  }
  const seconds = Math.floor((now - seen) / 1000);
  if (seconds < 0) {
    return "just now";
  }
  if (seconds < 60) {
    return "just now";
  }
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) {
    return `${minutes}m ago`;
  }
  const hours = Math.floor(minutes / 60);
  if (hours < 24) {
    return `${hours}h ago`;
  }
  const days = Math.floor(hours / 24);
  if (days < 7) {
    return `${days}d ago`;
  }
  return new Intl.DateTimeFormat(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
  }).format(new Date(seen));
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

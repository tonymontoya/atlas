import { afterEach, describe, expect, it, vi } from "vitest";

import {
  listCases,
  listClusters,
  listSyncRuns,
  loadClusterView,
  type CaseRecord,
  type ClusterIndex,
} from "./api";

const CLUSTER_JSON = {
  id: 7,
  fsid: "00000000-0000-4000-8000-000000000101",
  name: "fixture-healthy",
  clusterType: "bare-metal",
  cephVersion: "18.2.4",
  healthStatus: "HEALTH_OK",
  healthSummary: "cluster is healthy",
  agentLastSeen: null,
};

const EMPTY_INDEX: ClusterIndex = {
  clusters: [],
  total: 0,
  limit: 50,
  offset: 0,
};

const CASE_JSON: CaseRecord = {
  id: 11,
  title: "OSD down requires triage",
  summary: "one OSD is down",
  status: "detected",
  severity: "high",
  source: "manual",
  clusterFsid: "00000000-0000-4000-8000-000000000101",
  createdAt: "2026-08-13T12:00:00Z",
  updatedAt: "2026-08-13T12:00:00Z",
};

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function stubFetch(handler: (path: string) => Response) {
  return vi.fn(async (input: RequestInfo | URL) => {
    const url = new URL(String(input), "http://atlas.test");
    return handler(url.pathname + (url.search === "" ? "" : url.search));
  });
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("listClusters", () => {
  it("builds the query string from only the parameters it needs", async () => {
    const fetchMock = stubFetch((path) => {
      expect(path).toBe("/api/v1/clusters?q=osd&limit=25&offset=50");
      return jsonResponse(EMPTY_INDEX);
    });
    vi.stubGlobal("fetch", fetchMock);

    await listClusters({ q: "osd", limit: 25, offset: 50 });
    expect(fetchMock).toHaveBeenCalledOnce();
  });

  it("omits the query string entirely for the default page", async () => {
    const fetchMock = stubFetch((path) => {
      expect(path).toBe("/api/v1/clusters");
      return jsonResponse(EMPTY_INDEX);
    });
    vi.stubGlobal("fetch", fetchMock);

    await listClusters();
  });
});

describe("listCases and listSyncRuns", () => {
  it("filters cases by cluster and encodes the FSID", async () => {
    const fetchMock = stubFetch((path) => {
      expect(path).toBe("/api/v1/cases?cluster=00000000-0000-4000-8000-000000000101");
      return jsonResponse([CASE_JSON]);
    });
    vi.stubGlobal("fetch", fetchMock);

    const cases = await listCases({ cluster: "00000000-0000-4000-8000-000000000101" });
    expect(cases).toEqual([CASE_JSON]);
  });

  it("requests the unfiltered case list without a query string", async () => {
    const fetchMock = stubFetch((path) => {
      expect(path).toBe("/api/v1/cases");
      return jsonResponse([CASE_JSON]);
    });
    vi.stubGlobal("fetch", fetchMock);

    await listCases();
  });

  it("reads the sync run history", async () => {
    const fetchMock = stubFetch((path) => {
      expect(path).toBe("/api/v1/inventory-sync-runs");
      return jsonResponse([]);
    });
    vi.stubGlobal("fetch", fetchMock);

    await listSyncRuns();
  });
});

describe("loadClusterView", () => {
  it("gathers the scoped reads for the matching cluster", async () => {
    const fetchMock = stubFetch((path) => {
      if (path === "/api/v1/clusters?q=00000000-0000-4000-8000-000000000101") {
        return jsonResponse({ clusters: [CLUSTER_JSON], total: 1, limit: 50, offset: 0 });
      }
      switch (path) {
        case "/api/v1/clusters/00000000-0000-4000-8000-000000000101/health":
          return jsonResponse({ status: "HEALTH_OK", summary: "cluster is healthy", checks: [] });
        case "/api/v1/clusters/00000000-0000-4000-8000-000000000101/osds":
          return jsonResponse([{ id: 0, host: "host-a", up: true, in: true }]);
        case "/api/v1/clusters/00000000-0000-4000-8000-000000000101/hosts":
          return jsonResponse([{ name: "host-a" }]);
        case "/api/v1/clusters/00000000-0000-4000-8000-000000000101/storage-devices":
          return jsonResponse([{ host: "host-a", serial: "serial-a" }]);
        case "/api/v1/clusters/00000000-0000-4000-8000-000000000101/daemons":
          return jsonResponse([]);
        case "/api/v1/clusters/00000000-0000-4000-8000-000000000101/pools":
          return jsonResponse([]);
        case "/api/v1/cases?cluster=00000000-0000-4000-8000-000000000101":
          return jsonResponse([CASE_JSON]);
        default:
          throw new Error(`unexpected fetch ${path}`);
      }
    });
    vi.stubGlobal("fetch", fetchMock);

    const view = await loadClusterView("00000000-0000-4000-8000-000000000101");
    expect(view).not.toHaveProperty("notFound");
    if ("notFound" in view) {
      throw new Error("expected a cluster view");
    }
    expect(view.cluster.name).toBe("fixture-healthy");
    expect(view.health.status).toBe("HEALTH_OK");
    expect(view.osds).toHaveLength(1);
    expect(view.cases).toEqual([CASE_JSON]);
    expect(view.casesUnavailable).toBeUndefined();
  });

  it("resolves a cluster case-insensitively by FSID", async () => {
    const fetchMock = stubFetch((path) => {
      if (path.startsWith("/api/v1/clusters?")) {
        return jsonResponse({ clusters: [CLUSTER_JSON], total: 1, limit: 50, offset: 0 });
      }
      return jsonResponse([]);
    });
    vi.stubGlobal("fetch", fetchMock);

    const view = await loadClusterView("00000000-0000-4000-8000-000000000101".toUpperCase());
    expect(view).not.toHaveProperty("notFound");
  });

  it("reports notFound when no registered cluster carries the FSID", async () => {
    const fetchMock = stubFetch(() => jsonResponse(EMPTY_INDEX));
    vi.stubGlobal("fetch", fetchMock);

    const view = await loadClusterView("ffffffff-ffff-4000-8000-ffffffffffff");
    expect(view).toEqual({ notFound: true });
  });

  it("degrades to an unavailable note instead of failing when the case list rejects", async () => {
    const fetchMock = stubFetch((path) => {
      if (path.startsWith("/api/v1/clusters?")) {
        return jsonResponse({ clusters: [CLUSTER_JSON], total: 1, limit: 50, offset: 0 });
      }
      if (path.startsWith("/api/v1/cases")) {
        return jsonResponse({ error: { class: "Unavailable", message: "cases are down" } }, 503);
      }
      return jsonResponse([]);
    });
    vi.stubGlobal("fetch", fetchMock);

    const view = await loadClusterView("00000000-0000-4000-8000-000000000101");
    if ("notFound" in view) {
      throw new Error("expected a cluster view");
    }
    expect(view.cases).toEqual([]);
    expect(view.casesUnavailable).toBe("cases are down");
  });
});

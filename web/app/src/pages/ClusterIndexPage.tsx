import React from "react";
import { Link } from "react-router-dom";
import { Column, Grid, Pagination, Search } from "@carbon/react";
import { listClusters, type ClusterIndex } from "../api";
import {
  agentLastSeenLabel,
  clusterRoute,
  healthStatusLabel,
  offsetForPage,
} from "../clusters";
import { errorMessage } from "../format";
import { toneForHealth } from "../tones";
import { AtlasTable } from "../components/tables";
import { ErrorState, PageIntro, StatusTag } from "../components/ui";

const PAGE_SIZES = [10, 25, 50, 100];
const DEFAULT_PAGE_SIZE = 25;

// ClusterIndexPage is the operator's fleet landing page: every registered
// cluster with health summary and agent last-seen, searchable by name or
// FSID (server-side q) and paginated (ADR-0025 fleet reads).
export function ClusterIndexPage() {
  const [query, setQuery] = React.useState("");
  const [debouncedQuery, setDebouncedQuery] = React.useState("");
  const [page, setPage] = React.useState(1);
  const [pageSize, setPageSize] = React.useState(DEFAULT_PAGE_SIZE);
  const [index, setIndex] = React.useState<ClusterIndex | null>(null);
  const [loading, setLoading] = React.useState(true);
  const [error, setError] = React.useState<string | null>(null);

  React.useEffect(() => {
    setPage(1);
    const timer = setTimeout(() => {
      setDebouncedQuery(query.trim());
    }, 250);
    return () => clearTimeout(timer);
  }, [query]);

  React.useEffect(() => {
    const controller = new AbortController();

    async function load() {
      try {
        setLoading(true);
        setError(null);
        const loaded = await listClusters(
          {
            q: debouncedQuery === "" ? undefined : debouncedQuery,
            limit: pageSize,
            offset: offsetForPage(page, pageSize),
          },
          controller.signal,
        );
        setIndex(loaded);
      } catch (loadError) {
        if (controller.signal.aborted) {
          return;
        }
        setError(errorMessage(loadError));
      } finally {
        if (!controller.signal.aborted) {
          setLoading(false);
        }
      }
    }

    void load();

    return () => controller.abort();
  }, [debouncedQuery, page, pageSize]);

  const rows = index?.clusters ?? [];
  const total = index?.total ?? 0;

  return (
    <>
      <h1 className="atlas-page-title">Clusters</h1>
      <PageIntro>
        Registered Ceph clusters with their latest observed health and Agent
        last-seen. Search by name or FSID.
      </PageIntro>
      <Grid fullWidth>
        <Column sm={4} md={6} lg={8}>
          <Search
            id="cluster-search"
            labelText="Search clusters"
            placeholder="Search by name or FSID"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            onClear={() => setQuery("")}
          />
        </Column>
      </Grid>
      {error ? <ErrorState message={error} /> : null}
      {!error && loading && !index ? <p className="atlas-empty">Loading clusters…</p> : null}
      {index ? (
        <AtlasTable
          columns={[
            {
              key: "name",
              header: "Name",
              render: (cluster) =>
                clusterRoute(cluster) ? (
                  <Link className="atlas-table-link" to={clusterRoute(cluster) as string}>
                    {cluster.name}
                  </Link>
                ) : (
                  cluster.name
                ),
            },
            {
              key: "fsid",
              header: "FSID",
              render: (cluster) =>
                cluster.fsid ? (
                  <span className="atlas-mono">{cluster.fsid}</span>
                ) : (
                  <span className="atlas-subtle">not enrolled</span>
                ),
            },
            {
              key: "type",
              header: "Type",
              render: (cluster) => cluster.clusterType,
            },
            {
              key: "version",
              header: "Ceph version",
              render: (cluster) => cluster.cephVersion ?? "unreported",
            },
            {
              key: "health",
              header: "Health",
              render: (cluster) => (
                <StatusTag
                  label={healthStatusLabel(cluster.healthStatus)}
                  tone={toneForHealth(cluster.healthStatus)}
                  title={cluster.healthSummary ?? undefined}
                />
              ),
            },
            {
              key: "agent",
              header: "Agent last seen",
              render: (cluster) => agentLastSeenLabel(cluster.agentLastSeen),
            },
          ]}
          rows={rows}
          rowKey={(cluster) => cluster.fsid ?? `unregistered-${cluster.name}`}
          emptyLabel={
            debouncedQuery === ""
              ? "No clusters registered yet."
              : `No clusters match “${debouncedQuery}”.`
          }
        />
      ) : null}
      <Pagination
        page={page}
        pageSize={pageSize}
        pageSizes={PAGE_SIZES}
        totalItems={total}
        onChange={({ page: nextPage, pageSize: nextSize }) => {
          if (nextSize !== pageSize) {
            setPageSize(nextSize);
            setPage(1);
          } else {
            setPage(nextPage);
          }
        }}
      />
      <p className="atlas-subtle">
        {total} cluster{total === 1 ? "" : "s"} · page {page} of{" "}
        {Math.max(1, Math.ceil(total / pageSize))} · click a cluster name to open its view
      </p>
    </>
  );
}

package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/tonymontoya/ceph-atlas/internal/app"
	"github.com/tonymontoya/ceph-atlas/internal/inventory/entities"
	"github.com/tonymontoya/ceph-atlas/internal/store"
)

// parseQueryInt reads a non-negative integer query parameter, falling
// back when absent. Malformed or negative values are invalid requests.
func parseQueryInt(r *http.Request, key string, fallback int) (int, error) {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, invalidRequest(key + " must be a non-negative integer")
	}
	return value, nil
}

// listClusters serves the cluster index (ADR-0025 fleet reads): every
// active registered cluster with its latest observed health and Agent
// last-seen, searchable by name or FSID, paginated.
func (s *Server) listClusters(w http.ResponseWriter, r *http.Request) {
	limit, err := parseQueryInt(r, "limit", 50)
	if err != nil {
		writeError(w, err)
		return
	}
	offset, err := parseQueryInt(r, "offset", 0)
	if err != nil {
		writeError(w, err)
		return
	}
	index, err := s.app.ClusterInventory.ListClusterSummaries(r.Context(), store.ListClustersQuery{
		Search: r.URL.Query().Get("q"),
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, index)
}

// clusterFSID extracts the {fsid} route value every scoped read
// addresses a cluster by.
func clusterFSID(r *http.Request) string {
	return r.PathValue("fsid")
}

func (s *Server) clusterHealth(w http.ResponseWriter, r *http.Request) {
	health, err := s.app.ClusterInventory.ClusterHealth(r.Context(), clusterFSID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, health)
}

// inventoryRead serves one declared entity's rows through the cluster
// inventory seam. The concrete slice type crosses only into the JSON
// encoder; handlers never inspect it.
type inventoryRead func(inv app.ClusterInventory, ctx context.Context, fsid string) (any, error)

// inventoryReadBindings names how the cluster inventory seam serves
// each declared entity.
var inventoryReadBindings = map[entities.Entity]inventoryRead{
	entities.OSDs: func(inv app.ClusterInventory, ctx context.Context, fsid string) (any, error) {
		return inv.ClusterOSDs(ctx, fsid)
	},
	entities.Hosts: func(inv app.ClusterInventory, ctx context.Context, fsid string) (any, error) {
		return inv.ClusterHosts(ctx, fsid)
	},
	entities.StorageDevices: func(inv app.ClusterInventory, ctx context.Context, fsid string) (any, error) {
		return inv.ClusterStorageDevices(ctx, fsid)
	},
	entities.Daemons: func(inv app.ClusterInventory, ctx context.Context, fsid string) (any, error) {
		return inv.ClusterDaemons(ctx, fsid)
	},
	entities.Pools: func(inv app.ClusterInventory, ctx context.Context, fsid string) (any, error) {
		return inv.ClusterPools(ctx, fsid)
	},
}

// validateInventoryReadBindings fails loudly when a declared entity
// lacks its read binding.
func validateInventoryReadBindings(bindings map[entities.Entity]inventoryRead) {
	for _, entity := range entities.All {
		if _, ok := bindings[entity]; !ok {
			panic(fmt.Sprintf("api: declared entity %q has no cluster inventory read binding", entity.Noun))
		}
	}
}

// clusterEntityRoutes generates one route per declared entity: the
// entity's Noun addresses it under the cluster scope, and its binding
// names the seam method that serves it. A declared entity without a
// binding fails construction loudly; the OpenAPI parity test then
// proves every generated route is documented and every documented
// entity path is generated.
func (s *Server) clusterEntityRoutes() []route {
	validateInventoryReadBindings(inventoryReadBindings)
	routes := make([]route, 0, len(entities.All))
	for _, entity := range entities.All {
		routes = append(routes, route{
			method:  http.MethodGet,
			pattern: "/api/v1/clusters/{fsid}/" + entity.Noun,
			handler: s.clusterEntityRead(inventoryReadBindings[entity]),
		})
	}
	return routes
}

// clusterEntityRead adapts one entity's seam read to the shared
// scoped-read response: the entity's rows, or the seam's error.
func (s *Server) clusterEntityRead(read inventoryRead) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := read(s.app.ClusterInventory, r.Context(), clusterFSID(r))
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, rows)
	}
}

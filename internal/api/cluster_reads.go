package api

import (
	"net/http"
	"strconv"

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

func (s *Server) osds(w http.ResponseWriter, r *http.Request) {
	osds, err := s.app.ClusterInventory.ClusterOSDs(r.Context(), clusterFSID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, osds)
}

func (s *Server) hosts(w http.ResponseWriter, r *http.Request) {
	hosts, err := s.app.ClusterInventory.ClusterHosts(r.Context(), clusterFSID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, hosts)
}

func (s *Server) storageDevices(w http.ResponseWriter, r *http.Request) {
	devices, err := s.app.ClusterInventory.ClusterStorageDevices(r.Context(), clusterFSID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, devices)
}

func (s *Server) daemons(w http.ResponseWriter, r *http.Request) {
	daemons, err := s.app.ClusterInventory.ClusterDaemons(r.Context(), clusterFSID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, daemons)
}

func (s *Server) pools(w http.ResponseWriter, r *http.Request) {
	pools, err := s.app.ClusterInventory.ClusterPools(r.Context(), clusterFSID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, pools)
}

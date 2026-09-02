package api

import (
	"net/http"
	"time"

	"github.com/tonymontoya/ceph-atlas/internal/apperr"
	"github.com/tonymontoya/ceph-atlas/internal/ca"
	"github.com/tonymontoya/ceph-atlas/internal/fleet"
	"github.com/tonymontoya/ceph-atlas/internal/inventory"
	"github.com/tonymontoya/ceph-atlas/internal/inventorysync"
	"github.com/tonymontoya/ceph-atlas/internal/store"
)

func agentObservationsUnsupported() apperr.Error {
	return apperr.Error{
		Class:   apperr.Unsupported,
		Message: "agent observations require the enrollment CA and postgres read source",
	}
}

// agentObservationBatch is the request body: one typed Observation
// Batch for one collection cycle (ADR-0025) — all entity types, one
// consistent snapshot, the same normalized shape the sync command
// produces. Provider and scenario are deliberately absent: Atlas
// records provider "agent" server-side, never from the payload.
type agentObservationBatch struct {
	ObservedAt time.Time                 `json:"observedAt"`
	Cluster    fleet.ClusterIdentity     `json:"cluster"`
	Health     inventory.Health          `json:"health"`
	OSDs       []inventory.OSD           `json:"osds"`
	Hosts      []inventory.Host          `json:"hosts"`
	Devices    []inventory.StorageDevice `json:"devices"`
	Daemons    []inventory.Daemon        `json:"daemons"`
	Pools      []inventory.Pool          `json:"pools"`
}

type agentObservationReceipt struct {
	ClusterID  int64 `json:"clusterId"`
	SnapshotID int64 `json:"snapshotId"`
}

// pushAgentObservations accepts an enrolled Agent's Observation Batch
// over mutual TLS (ADR-0025, ADR-0026). The client certificate is the
// authentication: Atlas resolves the Cluster from the certificate's
// recorded serial number and rejects any batch whose FSID does not
// match, so attribution never comes from payload claims. The endpoint
// deliberately sits outside requireIdentity.
func (s *Server) pushAgentObservations(w http.ResponseWriter, r *http.Request) {
	if s.app.AgentObservations == nil || s.app.EnrollmentCA == nil {
		writeError(w, agentObservationsUnsupported())
		return
	}
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
		writeError(w, apperr.Error{
			Class:   apperr.Unauthorized,
			Message: "agent observations require a TLS client certificate",
		})
		return
	}
	resolved, err := s.app.AgentObservations.ResolveAgentCluster(r.Context(), ca.SerialNumberHex(r.TLS.PeerCertificates[0]))
	if err != nil {
		writeError(w, err)
		return
	}

	var batch agentObservationBatch
	if err := decodeJSONBody(w, r, &batch); err != nil {
		return
	}
	if batch.ObservedAt.IsZero() {
		writeError(w, invalidRequest("observedAt is required"))
		return
	}
	if batch.Cluster.FSID == "" {
		writeError(w, invalidRequest("cluster fsid is required"))
		return
	}
	if batch.Cluster.Name == "" {
		writeError(w, invalidRequest("cluster name is required"))
		return
	}
	if batch.Cluster.CephVersion == "" {
		writeError(w, invalidRequest("cluster ceph version is required"))
		return
	}
	switch batch.Cluster.Type {
	case fleet.ClusterTypeBareMetal, fleet.ClusterTypeRook:
	default:
		writeError(w, invalidRequest("cluster type must be bare-metal or rook"))
		return
	}
	if batch.Cluster.FSID != resolved.FSID {
		writeError(w, apperr.Error{
			Class:   apperr.Conflict,
			Message: "payload cluster fsid does not match the cluster the certificate is enrolled for",
		})
		return
	}

	result, err := inventorysync.RunPush(r.Context(), s.app.AgentObservations, resolved.ClusterID, store.InventoryObservation{
		ObservedAt: batch.ObservedAt,
		Cluster:    batch.Cluster,
		Health:     batch.Health,
		OSDs:       batch.OSDs,
		Hosts:      batch.Hosts,
		Devices:    batch.Devices,
		Daemons:    batch.Daemons,
		Pools:      batch.Pools,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, agentObservationReceipt{
		ClusterID:  result.ClusterID,
		SnapshotID: result.SnapshotID,
	})
}

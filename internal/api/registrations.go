package api

import (
	"github.com/tonymontoya/ceph-atlas/internal/fleet"
	"net/http"
	"strconv"

	"github.com/tonymontoya/ceph-atlas/internal/apperr"
	"github.com/tonymontoya/ceph-atlas/internal/store"
)

func clusterRegistrationUnsupported() apperr.Error {
	return apperr.Error{
		Class:   apperr.Unsupported,
		Message: "cluster registration requires postgres read source",
	}
}

func parseClusterID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func clusterNotFound() apperr.Error {
	return apperr.Error{
		Class:   apperr.NotFound,
		Message: "cluster not found",
	}
}

// createClusterRegistration registers a Cluster and returns the one-time
// Enrollment Credential exactly once (ADR-0025/0026).
func (s *Server) createClusterRegistration(w http.ResponseWriter, r *http.Request) {
	if s.app.ClusterRegistrations == nil {
		writeError(w, clusterRegistrationUnsupported())
		return
	}
	var request struct {
		Name        string `json:"name"`
		ClusterType string `json:"clusterType"`
	}
	if err := decodeJSONBody(w, r, &request); err != nil {
		return
	}

	registration, credential, err := s.app.ClusterRegistrations.CreateClusterRegistration(r.Context(), store.ClusterRegistrationInput{
		Name:        request.Name,
		ClusterType: request.ClusterType,
		Actor:       actorFromRequest(r),
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, createClusterRegistrationResponse{
		Cluster:              registration,
		EnrollmentCredential: credential,
	})
}

type createClusterRegistrationResponse struct {
	Cluster              fleet.ClusterRegistration  `json:"cluster"`
	EnrollmentCredential fleet.EnrollmentCredential `json:"enrollmentCredential"`
}

func (s *Server) clusterRegistration(w http.ResponseWriter, r *http.Request) {
	if s.app.ClusterRegistrations == nil {
		writeError(w, clusterRegistrationUnsupported())
		return
	}
	id, ok := parseClusterID(r)
	if !ok {
		writeError(w, clusterNotFound())
		return
	}
	registration, err := s.app.ClusterRegistrations.GetClusterRegistration(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, registration)
}

// deregisterCluster retires a registration; history, snapshots, and Cases
// are preserved.
func (s *Server) deregisterCluster(w http.ResponseWriter, r *http.Request) {
	if s.app.ClusterRegistrations == nil {
		writeError(w, clusterRegistrationUnsupported())
		return
	}
	id, ok := parseClusterID(r)
	if !ok {
		writeError(w, clusterNotFound())
		return
	}
	registration, err := s.app.ClusterRegistrations.DeregisterCluster(r.Context(), store.DeregisterClusterInput{
		ClusterID: id,
		Actor:     actorFromRequest(r),
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, registration)
}

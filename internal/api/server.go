package api

import (
	"encoding/json"
	"errors"
	"github.com/tonymontoya/ceph-atlas/internal/app"
	"github.com/tonymontoya/ceph-atlas/internal/apperr"
	"github.com/tonymontoya/ceph-atlas/internal/identity"
	"github.com/tonymontoya/ceph-atlas/internal/providers"
	"github.com/tonymontoya/ceph-atlas/internal/store"
	"net/http"
	"strconv"
	"strings"
)

type Server struct {
	app *app.App
}

func NewServer(app *app.App) *Server {
	return &Server{app: app}
}

type route struct {
	method  string
	pattern string
	handler func(http.ResponseWriter, *http.Request)
}

func (s *Server) routes() []route {
	return []route{
		{"GET", "/healthz", s.healthz},
		{"GET", "/api/v1/me", s.requireIdentity(s.me)},
		{"GET", "/api/v1/clusters/current", s.cluster},
		{"GET", "/api/v1/clusters/current/health", s.clusterHealth},
		{"GET", "/api/v1/clusters/current/osds", s.osds},
		{"GET", "/api/v1/clusters/current/hosts", s.hosts},
		{"GET", "/api/v1/clusters/current/storage-devices", s.storageDevices},
		{"GET", "/api/v1/clusters/current/daemons", s.daemons},
		{"GET", "/api/v1/clusters/current/pools", s.pools},
		{"GET", "/api/v1/inventory-sync-runs", s.inventorySyncRuns},
		{"GET", "/api/v1/alert-evaluation-runs", s.alertEvaluationRuns},
		{"GET", "/api/v1/cases", s.cases},
		{"POST", "/api/v1/cases", s.requireIdentity(s.createCase)},
		{"GET", "/api/v1/cases/{id}", s.caseByID},
		{"GET", "/api/v1/cases/{id}/timeline", s.caseTimeline},
		{"POST", "/api/v1/cases/{id}/transitions", s.requireIdentity(s.transitionCase)},
		{"POST", "/api/v1/cases/{id}/assignment", s.requireIdentity(s.assignCase)},
		{"POST", "/api/v1/cases/{id}/notes", s.requireIdentity(s.addCaseNote)},
		{"GET", "/api/v1/cases/{id}/notes", s.caseNotes},
		{"POST", "/api/v1/cases/{id}/workflows", s.requireIdentity(s.attachWorkflow)},
		{"GET", "/api/v1/cases/{id}/workflows", s.listCaseWorkflows},
		{"POST", "/api/v1/workflow-instances/{id}/approvals", s.requireIdentity(s.approveWorkflowGate)},
		{"POST", "/api/v1/workflow-instances/{id}/task-completions", s.requireIdentity(s.completeWorkflowTask)},
		{"GET", "/api/v1/workflow-instances/{id}/jobs", s.listWorkflowJobs},
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	for _, r := range s.routes() {
		mux.HandleFunc(r.method+" "+r.pattern, r.handler)
	}
	return mux
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	id, _ := identity.FromContext(r.Context())
	writeJSON(w, http.StatusOK, map[string]string{
		"subject":     id.Subject,
		"displayName": id.DisplayName,
	})
}

// actorFromRequest converts the authenticated identity into the actor
// the durable writes attribute. Write handlers run behind
// requireIdentity, so the identity is always present.
func actorFromRequest(r *http.Request) store.Actor {
	actor, _ := identity.FromContext(r.Context())
	return store.Actor{Subject: actor.Subject, DisplayName: actor.DisplayName}
}

func (s *Server) requireIdentity(next func(http.ResponseWriter, *http.Request)) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.app.Verifier == nil {
			writeError(w, apperr.Error{
				Class:   apperr.Unauthorized,
				Message: "authentication is not configured",
			})
			return
		}
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			writeError(w, apperr.Error{
				Class:   apperr.Unauthorized,
				Message: "missing bearer token",
			})
			return
		}
		id, err := s.app.Verifier.Verify(r.Context(), strings.TrimPrefix(header, "Bearer "))
		if err != nil {
			writeError(w, apperr.Error{
				Class:   apperr.Unauthorized,
				Message: "invalid or expired token",
			})
			return
		}
		next(w, r.WithContext(identity.WithContext(r.Context(), id)))
	}
}

func (s *Server) cluster(w http.ResponseWriter, r *http.Request) {
	identity, err := s.app.CephProvider.ClusterIdentity(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, identity)
}

func (s *Server) clusterHealth(w http.ResponseWriter, r *http.Request) {
	health, err := s.app.CephProvider.Health(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, health)
}

func (s *Server) osds(w http.ResponseWriter, r *http.Request) {
	osds, err := s.app.CephProvider.OSDs(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, osds)
}

func (s *Server) hosts(w http.ResponseWriter, r *http.Request) {
	hosts, err := s.app.CephProvider.Hosts(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, hosts)
}

func (s *Server) storageDevices(w http.ResponseWriter, r *http.Request) {
	hosts, err := s.app.CephProvider.Hosts(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	devices, err := providers.AllHostDevices(r.Context(), s.app.CephProvider, hosts)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, devices)
}

func (s *Server) daemons(w http.ResponseWriter, r *http.Request) {
	daemons, err := s.app.CephProvider.Daemons(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, daemons)
}

func (s *Server) pools(w http.ResponseWriter, r *http.Request) {
	pools, err := s.app.CephProvider.Pools(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, pools)
}

func (s *Server) inventorySyncRuns(w http.ResponseWriter, r *http.Request) {
	if s.app.InventorySyncRuns == nil {
		writeError(w, apperr.Error{
			Class:   apperr.Unsupported,
			Message: "inventory sync run history requires postgres read source",
		})
		return
	}
	runs, err := s.app.InventorySyncRuns.ListInventorySyncRuns(r.Context(), 50)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

func (s *Server) alertEvaluationRuns(w http.ResponseWriter, r *http.Request) {
	if s.app.AlertEvaluationRuns == nil {
		writeError(w, apperr.Error{
			Class:   apperr.Unsupported,
			Message: "alert evaluation run history requires postgres read source",
		})
		return
	}
	runs, err := s.app.AlertEvaluationRuns.ListAlertEvaluationRuns(r.Context(), 50)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

func (s *Server) cases(w http.ResponseWriter, r *http.Request) {
	if s.app.Cases == nil {
		writeError(w, caseReadsUnsupported())
		return
	}
	cases, err := s.app.Cases.ListCases(r.Context(), 50)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cases)
}

func (s *Server) caseByID(w http.ResponseWriter, r *http.Request) {
	if s.app.Cases == nil {
		writeError(w, caseReadsUnsupported())
		return
	}
	id, ok := parseCaseID(r)
	if !ok {
		writeError(w, caseNotFound())
		return
	}
	item, err := s.app.Cases.GetCase(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) caseTimeline(w http.ResponseWriter, r *http.Request) {
	if s.app.Cases == nil {
		writeError(w, caseReadsUnsupported())
		return
	}
	id, ok := parseCaseID(r)
	if !ok {
		writeError(w, caseNotFound())
		return
	}
	timeline, err := s.app.Cases.ListCaseTimeline(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, timeline)
}

func parseCaseID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func caseReadsUnsupported() apperr.Error {
	return apperr.Error{
		Class:   apperr.Unsupported,
		Message: "case reads require postgres read source",
	}
}

func caseNotFound() apperr.Error {
	return apperr.Error{
		Class:   apperr.NotFound,
		Message: "case not found",
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, err error) {
	var appErr apperr.Error
	if errors.As(err, &appErr) {
		writeJSON(w, statusFor(appErr.Class), map[string]apiError{
			"error": {
				Class:   string(appErr.Class),
				Message: appErr.Message,
			},
		})
		return
	}
	writeJSON(w, http.StatusInternalServerError, map[string]apiError{
		"error": {
			Class:   string(apperr.Internal),
			Message: err.Error(),
		},
	})
}

type apiError struct {
	Class   string `json:"class"`
	Message string `json:"message"`
}

func statusFor(class apperr.Class) int {
	switch class {
	case apperr.InvalidRequest:
		return http.StatusBadRequest
	case apperr.Unauthorized:
		return http.StatusUnauthorized
	case apperr.NotFound:
		return http.StatusNotFound
	case apperr.Conflict, apperr.Unsafe:
		return http.StatusConflict
	case apperr.Unsupported, apperr.VersionUnsupported:
		return http.StatusUnprocessableEntity
	case apperr.Timeout:
		return http.StatusGatewayTimeout
	case apperr.Unavailable:
		return http.StatusServiceUnavailable
	case apperr.Partial, apperr.MalformedResponse:
		return http.StatusBadGateway
	default:
		return http.StatusInternalServerError
	}
}

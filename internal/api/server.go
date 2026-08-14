package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/tonymontoya/ceph-atlas/internal/app"
	"github.com/tonymontoya/ceph-atlas/internal/inventory"
	"github.com/tonymontoya/ceph-atlas/internal/providers"
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
		{"GET", "/api/v1/clusters/current", s.cluster},
		{"GET", "/api/v1/clusters/current/health", s.clusterHealth},
		{"GET", "/api/v1/clusters/current/osds", s.osds},
		{"GET", "/api/v1/clusters/current/hosts", s.hosts},
		{"GET", "/api/v1/clusters/current/storage-devices", s.storageDevices},
		{"GET", "/api/v1/clusters/current/daemons", s.daemons},
		{"GET", "/api/v1/clusters/current/pools", s.pools},
		{"GET", "/api/v1/inventory-sync-runs", s.inventorySyncRuns},
		{"GET", "/api/v1/cases", s.cases},
		{"GET", "/api/v1/cases/{id}", s.caseByID},
		{"GET", "/api/v1/cases/{id}/timeline", s.caseTimeline},
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
	devices := make([]inventory.StorageDevice, 0)
	for _, host := range hosts {
		hostDevices, err := s.app.CephProvider.HostDevices(r.Context(), host.Name)
		if err != nil {
			writeError(w, err)
			return
		}
		devices = append(devices, hostDevices...)
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
		writeError(w, providers.ProviderError{
			Class:   providers.ErrorUnsupported,
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

func caseReadsUnsupported() providers.ProviderError {
	return providers.ProviderError{
		Class:   providers.ErrorUnsupported,
		Message: "case reads require postgres read source",
	}
}

func caseNotFound() providers.ProviderError {
	return providers.ProviderError{
		Class:   providers.ErrorNotFound,
		Message: "case not found",
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, err error) {
	var providerErr providers.ProviderError
	if errors.As(err, &providerErr) {
		writeJSON(w, statusForProviderError(providerErr.Class), map[string]apiError{
			"error": {
				Class:   string(providerErr.Class),
				Message: providerErr.Message,
			},
		})
		return
	}
	writeJSON(w, http.StatusInternalServerError, map[string]apiError{
		"error": {
			Class:   "Internal",
			Message: err.Error(),
		},
	})
}

type apiError struct {
	Class   string `json:"class"`
	Message string `json:"message"`
}

func statusForProviderError(class providers.ErrorClass) int {
	switch class {
	case providers.ErrorUnauthorized:
		return http.StatusUnauthorized
	case providers.ErrorNotFound:
		return http.StatusNotFound
	case providers.ErrorConflict, providers.ErrorUnsafe:
		return http.StatusConflict
	case providers.ErrorUnsupported, providers.ErrorVersionUnsupported:
		return http.StatusUnprocessableEntity
	case providers.ErrorTimeout:
		return http.StatusGatewayTimeout
	case providers.ErrorUnavailable:
		return http.StatusServiceUnavailable
	case providers.ErrorPartial, providers.ErrorMalformedResponse:
		return http.StatusBadGateway
	default:
		return http.StatusInternalServerError
	}
}

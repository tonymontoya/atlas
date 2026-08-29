// Package dashfake is the pure fake Ceph Dashboard: the Reef-shaped
// HTTP contract without any test-framework dependency, so both
// in-process tests (through internal/providers/ceph/dashtest) and
// dev-stack containers (through cmd/atlas-dev-dashboard) serve the
// exact same fixture responses. It never talks to a real cluster.
package dashfake

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

type Mode string

const (
	ModeSuccess      Mode = "success"
	ModeUnavailable  Mode = "unavailable"
	ModeUnauthorized Mode = "unauthorized"
	ModeMalformed    Mode = "malformed"
)

const (
	FSID        = "00000000-0000-4000-8000-000000000301"
	CephVersion = "18.2.4"
	Username    = "atlas-reader"
	Password    = "atlas-reader-password"
	Token       = "dashboard-test-token"

	HostA = "host-a.example.invalid"
	HostB = "host-b.example.invalid"
)

// Dashboard is the fake Dashboard's state: the mode's handler plus
// the request observability tests assert against.
type Dashboard struct {
	mode        Mode
	mu          sync.Mutex
	logins      int
	osdRequests []string
}

// NewDashboard builds the fake Dashboard for one mode; Handler serves
// its routes.
func NewDashboard(mode Mode) *Dashboard {
	return &Dashboard{mode: mode}
}

// Handler returns the fake Dashboard's routes.
func (d *Dashboard) Handler() http.Handler {
	mode := d.mode
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/auth", func(w http.ResponseWriter, r *http.Request) {
		d.mu.Lock()
		d.logins++
		d.mu.Unlock()
		switch mode {
		case ModeUnavailable:
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
		case ModeUnauthorized:
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
		default:
			WriteJSON(w, http.StatusCreated, map[string]any{"token": Token})
		}
	})
	guard := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer "+Token {
				http.Error(w, "unauthenticated", http.StatusUnauthorized)
				return
			}
			switch mode {
			case ModeUnavailable:
				http.Error(w, "unavailable", http.StatusServiceUnavailable)
			case ModeMalformed:
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, `{"not json`)
			default:
				next(w, r)
			}
		}
	}
	mux.HandleFunc("GET /api/health/get_cluster_fsid", guard(func(w http.ResponseWriter, r *http.Request) {
		WriteJSON(w, http.StatusOK, FSID)
	}))
	mux.HandleFunc("GET /api/summary", guard(func(w http.ResponseWriter, r *http.Request) {
		WriteJSON(w, http.StatusOK, map[string]any{
			"version":         CephVersion,
			"health_status":   "HEALTH_OK",
			"mgr_id":          "x",
			"executing_tasks": []any{},
		})
	}))
	mux.HandleFunc("GET /api/health/full", guard(func(w http.ResponseWriter, r *http.Request) {
		WriteJSON(w, http.StatusOK, map[string]any{
			"health": map[string]any{
				"status":  "HEALTH_OK",
				"summary": []any{},
				"checks": []any{
					map[string]any{
						"type":     "OSD_DOWN",
						"severity": "HEALTH_WARN",
						"summary":  "1 osd down",
					},
				},
			},
		})
	}))
	mux.HandleFunc("GET /api/osd", guard(d.handleOSDs))
	mux.HandleFunc("GET /api/host", guard(func(w http.ResponseWriter, r *http.Request) {
		paginate(w, r, []any{
			map[string]any{"hostname": HostA, "addr": "10.10.0.11", "labels": []any{}, "ceph_version": CephVersion},
			map[string]any{"hostname": HostB, "addr": "10.10.0.12", "labels": []any{}, "ceph_version": CephVersion},
		})
	}))
	mux.HandleFunc("GET /api/daemon", guard(func(w http.ResponseWriter, r *http.Request) {
		WriteJSON(w, http.StatusOK, []any{
			map[string]any{"daemon_type": "mon", "daemon_id": "a", "daemon_name": "mon.a", "hostname": HostA, "status": 1, "version": CephVersion},
			map[string]any{"daemon_type": "mon", "daemon_id": "b", "daemon_name": "mon.b", "hostname": HostB, "status": 1, "version": CephVersion},
			map[string]any{"daemon_type": "mgr", "daemon_id": "a", "daemon_name": "mgr.a", "hostname": HostA, "status": 2, "version": CephVersion},
			map[string]any{"daemon_type": "osd", "daemon_id": "0", "daemon_name": "osd.0", "hostname": HostA, "status": 1, "version": CephVersion},
			map[string]any{"daemon_type": "osd", "daemon_id": "1", "daemon_name": "osd.1", "hostname": HostB, "status": 0, "version": CephVersion},
		})
	}))
	mux.HandleFunc("GET /api/pool", guard(func(w http.ResponseWriter, r *http.Request) {
		WriteJSON(w, http.StatusOK, []any{
			map[string]any{"pool": 1, "pool_name": "device_health_metrics", "type": "replicated", "size": 3, "min_size": 2},
			map[string]any{"pool": 2, "pool_name": ".mgr", "type": "replicated", "size": 3, "min_size": 2},
		})
	}))
	mux.HandleFunc("GET /api/host/", guard(d.handleHostItem))
	return mux
}

// Logins reports how many login attempts the fake Dashboard saw.
func (d *Dashboard) Logins() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.logins
}

// OSDRequests reports the offset/limit pairs the OSD listing saw.
func (d *Dashboard) OSDRequests() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.osdRequests...)
}

func (d *Dashboard) handleOSDs(w http.ResponseWriter, r *http.Request) {
	d.mu.Lock()
	d.osdRequests = append(d.osdRequests, r.URL.Query().Get("offset")+"/"+r.URL.Query().Get("limit"))
	d.mu.Unlock()
	paginate(w, r, []any{
		map[string]any{"id": 0, "up": 1, "in": 1, "host": map[string]any{"name": HostA}},
		map[string]any{"id": 1, "up": 1, "in": 1, "host": map[string]any{"name": HostB}},
		map[string]any{"id": 2, "up": 0, "in": 1, "host": map[string]any{"name": HostB}},
	})
}

func (d *Dashboard) handleHostItem(w http.ResponseWriter, r *http.Request) {
	rest := r.URL.Path[len("/api/host/"):]
	host, subpath, _ := strings.Cut(rest, "/")
	switch host {
	case HostA, HostB:
	default:
		http.Error(w, "host not found", http.StatusNotFound)
		return
	}
	if subpath == "inventory" {
		d.handleHostInventory(w, host)
		return
	}
	if subpath != "" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"hostname": host, "labels": []any{}})
}

func (d *Dashboard) handleHostInventory(w http.ResponseWriter, host string) {
	devices := []any{}
	switch host {
	case HostA:
		devices = append(devices, map[string]any{
			"path":                "/dev/nvme0n1",
			"device_id":           "nvme-serial-a1",
			"human_readable_type": "ssd",
			"available":           false,
			"osd_ids":             []int{0},
		})
	case HostB:
		devices = append(devices,
			map[string]any{
				"path":                "/dev/sda",
				"device_id":           "ata-serial-b1",
				"human_readable_type": "hdd",
				"osd_ids":             []int{},
				"lvs": []any{
					map[string]any{"name": "osd-block-1", "osd_id": "1"},
				},
			},
			map[string]any{
				"path":                "/dev/sdb",
				"device_id":           "ata-serial-b2",
				"human_readable_type": "hdd",
				"available":           true,
			},
		)
	}
	WriteJSON(w, http.StatusOK, map[string]any{"name": host, "devices": devices})
}

func paginate(w http.ResponseWriter, r *http.Request, items []any) {
	query := r.URL.Query()
	offset, _ := strconv.Atoi(query.Get("offset"))
	limit, _ := strconv.Atoi(query.Get("limit"))
	if offset < 0 || offset > len(items) {
		offset = len(items)
	}
	end := len(items)
	if limit > 0 && offset+limit < end {
		end = offset + limit
	}
	w.Header().Set("X-Total-Count", strconv.Itoa(len(items)))
	WriteJSON(w, http.StatusOK, items[offset:end])
}

// WriteJSON lets bespoke servers reuse the same JSON response helper.
func WriteJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

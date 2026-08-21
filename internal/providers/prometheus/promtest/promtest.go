// Package promtest provides an in-process fake Prometheus server serving
// /api/v1/alerts-shaped JSON for tests. It exists so provider package
// tests and case-detection wiring tests run the same HTTP contract
// without a real Prometheus.
package promtest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

type Mode string

const (
	ModeSuccess      Mode = "success"
	ModeUnavailable  Mode = "unavailable"
	ModeUnauthorized Mode = "unauthorized"
	ModeMalformed    Mode = "malformed"
)

const (
	Token = "prometheus-test-token"

	AlertName    = "CephOSDDown"
	ClusterLabel = "promtest-reef"
	OSDLabel     = "1"
	Severity     = "warning"
	ActiveAt     = "2026-08-20T09:15:00Z"
)

type Prometheus struct {
	server *httptest.Server

	mu             sync.Mutex
	authorizations []string
}

func New(t *testing.T, mode Mode) *Prometheus {
	t.Helper()
	prometheus := &Prometheus{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/alerts", func(w http.ResponseWriter, r *http.Request) {
		prometheus.mu.Lock()
		prometheus.authorizations = append(prometheus.authorizations, r.Header.Get("Authorization"))
		prometheus.mu.Unlock()
		switch mode {
		case ModeUnavailable:
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
		case ModeUnauthorized:
			http.Error(w, "unauthenticated", http.StatusUnauthorized)
		case ModeMalformed:
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"not json`)
		default:
			WriteJSON(w, http.StatusOK, map[string]any{
				"status": "success",
				"data": map[string]any{
					"alerts": []any{
						map[string]any{
							"labels": map[string]any{
								"alertname": AlertName,
								"severity":  Severity,
								"cluster":   ClusterLabel,
								"osd":       OSDLabel,
							},
							"annotations": map[string]any{
								"summary": "OSD 1 is down (promtest)",
							},
							"state":    "firing",
							"activeAt": ActiveAt,
							"value":    "1",
						},
					},
				},
			})
		}
	})
	prometheus.server = httptest.NewServer(mux)
	t.Cleanup(prometheus.server.Close)
	return prometheus
}

func (p *Prometheus) URL() string {
	return p.server.URL
}

// Close stops the server early; use to simulate a Prometheus that went away.
func (p *Prometheus) Close() {
	p.server.Close()
}

func (p *Prometheus) Authorizations() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.authorizations...)
}

// WriteJSON lets bespoke test servers reuse the same JSON response helper.
func WriteJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

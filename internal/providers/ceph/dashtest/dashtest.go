// Package dashtest provides the in-process fake Ceph Dashboard for
// tests: the pure dashfake handler behind an httptest server, with the
// server lifetime tied to the test. It exists so provider package
// tests and sync wiring tests run the same HTTP contract without a
// real cluster; dev-stack containers serve the same handler through
// cmd/atlas-dev-dashboard.
package dashtest

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tonymontoya/ceph-atlas/internal/providers/ceph/dashfake"
)

// Mode and the fixture constants alias dashfake so existing test
// callers keep one import.
type Mode = dashfake.Mode

const (
	ModeSuccess      = dashfake.ModeSuccess
	ModeUnavailable  = dashfake.ModeUnavailable
	ModeUnauthorized = dashfake.ModeUnauthorized
	ModeMalformed    = dashfake.ModeMalformed
)

const (
	FSID        = dashfake.FSID
	CephVersion = dashfake.CephVersion
	Username    = dashfake.Username
	Password    = dashfake.Password
	Token       = dashfake.Token

	HostA = dashfake.HostA
	HostB = dashfake.HostB
)

// Dashboard is a fake Ceph Dashboard bound to a test's lifetime.
type Dashboard struct {
	dashboard *dashfake.Dashboard
	server    *httptest.Server
}

// New starts a fake Dashboard for one mode and stops it when the test
// ends.
func New(t *testing.T, mode Mode) *Dashboard {
	t.Helper()
	dashboard := dashfake.NewDashboard(mode)
	server := httptest.NewServer(dashboard.Handler())
	t.Cleanup(server.Close)
	return &Dashboard{dashboard: dashboard, server: server}
}

func (d *Dashboard) URL() string {
	return d.server.URL
}

// Close stops the server early; use to simulate a dashboard that went away.
func (d *Dashboard) Close() {
	d.server.Close()
}

func (d *Dashboard) Logins() int {
	return d.dashboard.Logins()
}

func (d *Dashboard) OSDRequests() []string {
	return d.dashboard.OSDRequests()
}

// WriteJSON lets bespoke test servers reuse the same JSON response helper.
func WriteJSON(w http.ResponseWriter, status int, body any) {
	dashfake.WriteJSON(w, status, body)
}

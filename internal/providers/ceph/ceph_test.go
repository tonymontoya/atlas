package ceph

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/tonymontoya/ceph-atlas/internal/apperr"
	"github.com/tonymontoya/ceph-atlas/internal/inventory"
	"github.com/tonymontoya/ceph-atlas/internal/providers/ceph/dashtest"
)

func newTestProvider(t *testing.T, mode dashtest.Mode) (*Provider, *dashtest.Dashboard) {
	t.Helper()
	dashboard := dashtest.New(t, mode)
	provider, err := New(Config{
		BaseURL:  dashboard.URL(),
		Username: dashtest.Username,
		Password: dashtest.Password,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	return provider, dashboard
}

func TestClusterIdentity(t *testing.T) {
	provider, _ := newTestProvider(t, dashtest.ModeSuccess)
	identity, err := provider.ClusterIdentity(context.Background())
	if err != nil {
		t.Fatalf("ClusterIdentity returned error: %v", err)
	}
	if identity.FSID != dashtest.FSID {
		t.Errorf("FSID = %q, want %q", identity.FSID, dashtest.FSID)
	}
	if identity.CephVersion != dashtest.CephVersion {
		t.Errorf("CephVersion = %q, want %q", identity.CephVersion, dashtest.CephVersion)
	}
	if identity.Name != defaultName {
		t.Errorf("Name = %q, want default %q", identity.Name, defaultName)
	}
	if identity.Type != "bare-metal" {
		t.Errorf("Type = %q, want bare-metal", identity.Type)
	}
}

func TestClusterIdentityConfiguredName(t *testing.T) {
	dashboard := dashtest.New(t, dashtest.ModeSuccess)
	provider, err := New(Config{
		BaseURL:     dashboard.URL(),
		Username:    dashtest.Username,
		Password:    dashtest.Password,
		ClusterName: "reef-lab",
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	identity, err := provider.ClusterIdentity(context.Background())
	if err != nil {
		t.Fatalf("ClusterIdentity returned error: %v", err)
	}
	if identity.Name != "reef-lab" {
		t.Errorf("Name = %q, want reef-lab", identity.Name)
	}
}

func TestHealthNormalizesChecksAndSummary(t *testing.T) {
	provider, _ := newTestProvider(t, dashtest.ModeSuccess)
	health, err := provider.Health(context.Background())
	if err != nil {
		t.Fatalf("Health returned error: %v", err)
	}
	if health.Status != inventory.HealthOK {
		t.Errorf("Status = %q, want HEALTH_OK", health.Status)
	}
	if len(health.Checks) != 1 || health.Checks[0].Name != "OSD_DOWN" {
		t.Errorf("Checks = %+v, want one OSD_DOWN check", health.Checks)
	}
}

func TestOSDsNormalizesIntFlagsAndCrushHost(t *testing.T) {
	provider, _ := newTestProvider(t, dashtest.ModeSuccess)
	osds, err := provider.OSDs(context.Background())
	if err != nil {
		t.Fatalf("OSDs returned error: %v", err)
	}
	if len(osds) != 3 {
		t.Fatalf("len(OSDs) = %d, want 3", len(osds))
	}
	if osds[2].Up || !osds[2].In {
		t.Errorf("osd 2 flags = up %v in %v, want down and in", osds[2].Up, osds[2].In)
	}
	if osds[0].Host != dashtest.HostA {
		t.Errorf("osd 0 host = %q, want %q", osds[0].Host, dashtest.HostA)
	}
}

func TestOSDPagination(t *testing.T) {
	provider, dashboard := newTestProvider(t, dashtest.ModeSuccess)
	provider.pageSize = 2
	osds, err := provider.OSDs(context.Background())
	if err != nil {
		t.Fatalf("OSDs returned error: %v", err)
	}
	if len(osds) != 3 {
		t.Fatalf("len(OSDs) = %d, want 3 across pages", len(osds))
	}
	requests := dashboard.OSDRequests()
	if len(requests) != 2 {
		t.Fatalf("osd requests = %v, want two pages", requests)
	}
	if requests[0] != "0/2" || requests[1] != "2/2" {
		t.Errorf("osd request offsets = %v, want [0/2 2/2]", requests)
	}
}

func TestHosts(t *testing.T) {
	provider, _ := newTestProvider(t, dashtest.ModeSuccess)
	hosts, err := provider.Hosts(context.Background())
	if err != nil {
		t.Fatalf("Hosts returned error: %v", err)
	}
	if len(hosts) != 2 {
		t.Fatalf("len(Hosts) = %d, want 2", len(hosts))
	}
	if hosts[0].Name != dashtest.HostA || hosts[0].Address != "10.10.0.11" {
		t.Errorf("hosts[0] = %+v, want %s at 10.10.0.11", hosts[0], dashtest.HostA)
	}
}

func TestHostDevicesNormalizesIdentityAndOSDs(t *testing.T) {
	provider, _ := newTestProvider(t, dashtest.ModeSuccess)

	devicesA, err := provider.HostDevices(context.Background(), dashtest.HostA)
	if err != nil {
		t.Fatalf("HostDevices(%s) returned error: %v", dashtest.HostA, err)
	}
	if len(devicesA) != 1 {
		t.Fatalf("len(devicesA) = %d, want 1", len(devicesA))
	}
	if devicesA[0].Serial != "nvme-serial-a1" || devicesA[0].OSDID == nil || *devicesA[0].OSDID != 0 {
		t.Errorf("devicesA[0] = %+v, want nvme-serial-a1 with osd id 0", devicesA[0])
	}

	devicesB, err := provider.HostDevices(context.Background(), dashtest.HostB)
	if err != nil {
		t.Fatalf("HostDevices(%s) returned error: %v", dashtest.HostB, err)
	}
	if len(devicesB) != 2 {
		t.Fatalf("len(devicesB) = %d, want 2", len(devicesB))
	}
	if devicesB[0].Serial != "ata-serial-b1" || devicesB[0].OSDID == nil || *devicesB[0].OSDID != 1 {
		t.Errorf("devicesB[0] = %+v, want ata-serial-b1 with lvm osd id 1", devicesB[0])
	}
	if devicesB[1].OSDID != nil {
		t.Errorf("devicesB[1] = %+v, want no osd id", devicesB[1])
	}
}

func TestHostDevicesUnknownHostIsNotFound(t *testing.T) {
	provider, _ := newTestProvider(t, dashtest.ModeSuccess)
	_, err := provider.HostDevices(context.Background(), "host-device-probe.example.invalid")
	assertErrorClass(t, err, apperr.NotFound)
}

func TestDaemonsNormalizesStatusEnum(t *testing.T) {
	provider, _ := newTestProvider(t, dashtest.ModeSuccess)
	daemons, err := provider.Daemons(context.Background())
	if err != nil {
		t.Fatalf("Daemons returned error: %v", err)
	}
	if len(daemons) != 5 {
		t.Fatalf("len(Daemons) = %d, want 5", len(daemons))
	}
	byName := map[string]inventory.DaemonStatus{}
	for _, daemon := range daemons {
		byName[daemon.Name] = daemon.Status
	}
	if byName["mgr.a"] != "starting" {
		t.Errorf("mgr.a status = %q, want starting", byName["mgr.a"])
	}
	if byName["osd.1"] != "stopped" {
		t.Errorf("osd.1 status = %q, want stopped", byName["osd.1"])
	}
}

func TestPools(t *testing.T) {
	provider, _ := newTestProvider(t, dashtest.ModeSuccess)
	pools, err := provider.Pools(context.Background())
	if err != nil {
		t.Fatalf("Pools returned error: %v", err)
	}
	if len(pools) != 2 {
		t.Fatalf("len(Pools) = %d, want 2", len(pools))
	}
	if pools[0].ID != 1 || pools[0].Name != "device_health_metrics" || pools[0].Type != "replicated" {
		t.Errorf("pools[0] = %+v, want pool 1 device_health_metrics replicated", pools[0])
	}
	if pools[0].Size == nil || *pools[0].Size != 3 || pools[0].MinSize == nil || *pools[0].MinSize != 2 {
		t.Errorf("pools[0] size/min_size = %v/%v, want 3/2", pools[0].Size, pools[0].MinSize)
	}
}

func TestUnavailableWhenServerIsDown(t *testing.T) {
	provider, dashboard := newTestProvider(t, dashtest.ModeSuccess)
	dashboard.Close()
	_, err := provider.OSDs(context.Background())
	assertErrorClass(t, err, apperr.Unavailable)
}

func TestReauthOnExpiredToken(t *testing.T) {
	var mu sync.Mutex
	logins := 0
	current := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Method == http.MethodPost && r.URL.Path == "/api/auth" {
			logins++
			current = fmt.Sprintf("token-%d", logins)
			dashtest.WriteJSON(w, http.StatusCreated, loginResponse{Token: current})
			return
		}
		if logins < 2 || r.Header.Get("Authorization") != "Bearer "+current {
			http.Error(w, "stale token", http.StatusUnauthorized)
			return
		}
		dashtest.WriteJSON(w, http.StatusOK, []any{})
	}))
	defer server.Close()

	provider, err := New(Config{BaseURL: server.URL, Username: dashtest.Username, Password: dashtest.Password})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if _, err := provider.Daemons(context.Background()); err != nil {
		t.Fatalf("Daemons returned error: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if logins != 2 {
		t.Errorf("logins = %d, want 2 (initial plus one re-auth)", logins)
	}
}

func TestForbiddenDoesNotReauth(t *testing.T) {
	var mu sync.Mutex
	logins := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Method == http.MethodPost && r.URL.Path == "/api/auth" {
			logins++
			dashtest.WriteJSON(w, http.StatusCreated, loginResponse{Token: "valid-token"})
			return
		}
		http.Error(w, "permission denied", http.StatusForbidden)
	}))
	defer server.Close()

	provider, err := New(Config{BaseURL: server.URL, Username: dashtest.Username, Password: dashtest.Password})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	_, err = provider.Daemons(context.Background())
	assertErrorClass(t, err, apperr.Unauthorized)
	mu.Lock()
	defer mu.Unlock()
	if logins != 1 {
		t.Errorf("logins = %d, want 1 (a 403 must not trigger re-auth)", logins)
	}
}

func TestConfigValidation(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
	}{
		{"missing base URL", Config{Username: "u", Password: "p"}},
		{"missing username", Config{BaseURL: "https://mon.example:8443", Password: "p"}},
		{"missing password", Config{BaseURL: "https://mon.example:8443", Username: "u"}},
		{"schemeless base URL", Config{BaseURL: "mon.example:8443", Username: "u", Password: "p"}},
		{"hostless base URL", Config{BaseURL: "https://", Username: "u", Password: "p"}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := New(test.cfg); err == nil {
				t.Fatal("New returned nil error for invalid config")
			}
		})
	}
}

func assertErrorClass(t *testing.T, err error, want apperr.Class) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with class %q, got nil", want)
	}
	var appErr apperr.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("error type = %T, want apperr.Error", err)
	}
	if appErr.Class != want {
		t.Fatalf("error class = %q, want %q (message: %s)", appErr.Class, want, appErr.Message)
	}
}

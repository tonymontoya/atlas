package ceph

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/tonymontoya/ceph-atlas/internal/inventory"
	"github.com/tonymontoya/ceph-atlas/internal/providers"
)

const (
	testFSID     = "00000000-0000-4000-8000-000000000201"
	testVersion  = "18.2.4"
	testUsername = "atlas-reader"
	testPassword = "atlas-reader-password"
	testToken    = "dashboard-test-token"

	hostA = "host-a.example.invalid"
	hostB = "host-b.example.invalid"
)

type fakeMode string

const (
	modeSuccess      fakeMode = "success"
	modeUnavailable  fakeMode = "unavailable"
	modeUnauthorized fakeMode = "unauthorized"
	modeMalformed    fakeMode = "malformed"
)

type fakeDashboard struct {
	t       *testing.T
	mode    fakeMode
	server  *httptest.Server
	mu      sync.Mutex
	logins  int
	osdReqs []string
}

func newFakeDashboard(t *testing.T, mode fakeMode) *fakeDashboard {
	fake := &fakeDashboard{t: t, mode: mode}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/auth", fake.handleAuth)
	mux.HandleFunc("GET /api/health/get_cluster_fsid", fake.requireToken(fake.serveString(testFSID)))
	mux.HandleFunc("GET /api/summary", fake.requireToken(fake.handleSummary))
	mux.HandleFunc("GET /api/health/full", fake.requireToken(fake.handleHealthFull))
	mux.HandleFunc("GET /api/osd", fake.requireToken(fake.handleOSDs))
	mux.HandleFunc("GET /api/host", fake.requireToken(fake.handleHosts))
	mux.HandleFunc("GET /api/daemon", fake.requireToken(fake.handleDaemons))
	mux.HandleFunc("GET /api/pool", fake.requireToken(fake.handlePools))
	mux.HandleFunc("GET /api/host/", fake.requireToken(fake.handleHostItem))
	fake.server = httptest.NewServer(mux)
	t.Cleanup(fake.server.Close)
	return fake
}

func (f *fakeDashboard) provider(t *testing.T) *Provider {
	t.Helper()
	provider, err := New(Config{
		BaseURL:  f.server.URL,
		Username: testUsername,
		Password: testPassword,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	return provider
}

func (f *fakeDashboard) handleAuth(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.logins++
	f.mu.Unlock()
	switch f.mode {
	case modeUnavailable:
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	case modeUnauthorized:
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
	default:
		writeJSON(w, http.StatusCreated, loginResponse{Token: testToken})
	}
}

func (f *fakeDashboard) requireToken(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+testToken {
			http.Error(w, "unauthenticated", http.StatusUnauthorized)
			return
		}
		switch f.mode {
		case modeUnavailable:
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
		case modeMalformed:
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"not json`)
		default:
			next(w, r)
		}
	}
}

func (f *fakeDashboard) serveString(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, body)
	}
}

func (f *fakeDashboard) handleSummary(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"version":         testVersion,
		"health_status":   "HEALTH_OK",
		"mgr_id":          "x",
		"executing_tasks": []any{},
	})
}

func (f *fakeDashboard) handleHealthFull(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
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
}

type fakeOSD struct {
	ID   int `json:"id"`
	Up   int `json:"up"`
	In   int `json:"in"`
	Host struct {
		Name string `json:"name"`
	} `json:"host"`
}

func (f *fakeDashboard) osds() []fakeOSD {
	return []fakeOSD{
		{ID: 0, Up: 1, In: 1, Host: hostNode(hostA)},
		{ID: 1, Up: 1, In: 1, Host: hostNode(hostB)},
		{ID: 2, Up: 0, In: 1, Host: hostNode(hostB)},
	}
}

func hostNode(name string) struct {
	Name string `json:"name"`
} {
	return struct {
		Name string `json:"name"`
	}{Name: name}
}

func (f *fakeDashboard) handleOSDs(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.osdReqs = append(f.osdReqs, r.URL.Query().Get("offset")+"/"+r.URL.Query().Get("limit"))
	f.mu.Unlock()
	osds := f.osds()
	items := make([]any, 0, len(osds))
	for _, osd := range osds {
		items = append(items, osd)
	}
	paginate(w, r, items)
}

func (f *fakeDashboard) handleHosts(w http.ResponseWriter, r *http.Request) {
	paginate(w, r, []any{
		map[string]any{"hostname": hostA, "addr": "10.10.0.11", "labels": []any{}, "ceph_version": testVersion},
		map[string]any{"hostname": hostB, "addr": "10.10.0.12", "labels": []any{}, "ceph_version": testVersion},
	})
}

func (f *fakeDashboard) handleHostItem(w http.ResponseWriter, r *http.Request) {
	rest := r.URL.Path[len("/api/host/"):]
	host, subpath, _ := strings.Cut(rest, "/")
	switch host {
	case hostA, hostB:
	default:
		http.Error(w, "host not found", http.StatusNotFound)
		return
	}
	if subpath == "inventory" {
		f.handleHostInventory(w, r, host)
		return
	}
	if subpath != "" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"hostname": host, "labels": []any{}})
}

func (f *fakeDashboard) handleHostInventory(w http.ResponseWriter, r *http.Request, host string) {
	devices := []any{}
	switch host {
	case hostA:
		devices = append(devices, map[string]any{
			"path":                "/dev/nvme0n1",
			"device_id":           "nvme-serial-a1",
			"human_readable_type": "ssd",
			"available":           false,
			"osd_ids":             []int{0},
		})
	case hostB:
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
	writeJSON(w, http.StatusOK, map[string]any{"name": host, "devices": devices})
}

func (f *fakeDashboard) handleDaemons(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []any{
		map[string]any{"daemon_type": "mon", "daemon_id": "a", "daemon_name": "mon.a", "hostname": hostA, "status": 1, "version": testVersion},
		map[string]any{"daemon_type": "mon", "daemon_id": "b", "daemon_name": "mon.b", "hostname": hostB, "status": 1, "version": testVersion},
		map[string]any{"daemon_type": "mgr", "daemon_id": "a", "daemon_name": "mgr.a", "hostname": hostA, "status": 2, "version": testVersion},
		map[string]any{"daemon_type": "osd", "daemon_id": "0", "daemon_name": "osd.0", "hostname": hostA, "status": 1, "version": testVersion},
		map[string]any{"daemon_type": "osd", "daemon_id": "1", "daemon_name": "osd.1", "hostname": hostB, "status": 0, "version": testVersion},
	})
}

func (f *fakeDashboard) handlePools(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []any{
		map[string]any{"pool": 1, "pool_name": "device_health_metrics", "type": "replicated", "size": 3, "min_size": 2},
		map[string]any{"pool": 2, "pool_name": ".mgr", "type": "replicated", "size": 3, "min_size": 2},
	})
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
	writeJSON(w, http.StatusOK, items[offset:end])
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func TestClusterIdentity(t *testing.T) {
	fake := newFakeDashboard(t, modeSuccess)
	identity, err := fake.provider(t).ClusterIdentity(context.Background())
	if err != nil {
		t.Fatalf("ClusterIdentity returned error: %v", err)
	}
	if identity.FSID != testFSID {
		t.Errorf("FSID = %q, want %q", identity.FSID, testFSID)
	}
	if identity.CephVersion != testVersion {
		t.Errorf("CephVersion = %q, want %q", identity.CephVersion, testVersion)
	}
	if identity.Name != defaultName {
		t.Errorf("Name = %q, want default %q", identity.Name, defaultName)
	}
	if identity.Type != "bare-metal" {
		t.Errorf("Type = %q, want bare-metal", identity.Type)
	}
}

func TestClusterIdentityConfiguredName(t *testing.T) {
	fake := newFakeDashboard(t, modeSuccess)
	provider, err := New(Config{
		BaseURL:     fake.server.URL,
		Username:    testUsername,
		Password:    testPassword,
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
	fake := newFakeDashboard(t, modeSuccess)
	health, err := fake.provider(t).Health(context.Background())
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
	fake := newFakeDashboard(t, modeSuccess)
	osds, err := fake.provider(t).OSDs(context.Background())
	if err != nil {
		t.Fatalf("OSDs returned error: %v", err)
	}
	if len(osds) != 3 {
		t.Fatalf("len(OSDs) = %d, want 3", len(osds))
	}
	if osds[2].Up || !osds[2].In {
		t.Errorf("osd 2 flags = up %v in %v, want down and in", osds[2].Up, osds[2].In)
	}
	if osds[0].Host != hostA {
		t.Errorf("osd 0 host = %q, want %q", osds[0].Host, hostA)
	}
}

func TestOSDPagination(t *testing.T) {
	fake := newFakeDashboard(t, modeSuccess)
	provider := fake.provider(t)
	provider.pageSize = 2
	osds, err := provider.OSDs(context.Background())
	if err != nil {
		t.Fatalf("OSDs returned error: %v", err)
	}
	if len(osds) != 3 {
		t.Fatalf("len(OSDs) = %d, want 3 across pages", len(osds))
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.osdReqs) != 2 {
		t.Fatalf("osd requests = %v, want two pages", fake.osdReqs)
	}
	if fake.osdReqs[0] != "0/2" || fake.osdReqs[1] != "2/2" {
		t.Errorf("osd request offsets = %v, want [0/2 2/2]", fake.osdReqs)
	}
}

func TestHosts(t *testing.T) {
	fake := newFakeDashboard(t, modeSuccess)
	hosts, err := fake.provider(t).Hosts(context.Background())
	if err != nil {
		t.Fatalf("Hosts returned error: %v", err)
	}
	if len(hosts) != 2 {
		t.Fatalf("len(Hosts) = %d, want 2", len(hosts))
	}
	if hosts[0].Name != hostA || hosts[0].Address != "10.10.0.11" {
		t.Errorf("hosts[0] = %+v, want %s at 10.10.0.11", hosts[0], hostA)
	}
}

func TestHostDevicesNormalizesIdentityAndOSDs(t *testing.T) {
	fake := newFakeDashboard(t, modeSuccess)
	provider := fake.provider(t)

	devicesA, err := provider.HostDevices(context.Background(), hostA)
	if err != nil {
		t.Fatalf("HostDevices(%s) returned error: %v", hostA, err)
	}
	if len(devicesA) != 1 {
		t.Fatalf("len(devicesA) = %d, want 1", len(devicesA))
	}
	if devicesA[0].Serial != "nvme-serial-a1" || devicesA[0].OSDID == nil || *devicesA[0].OSDID != 0 {
		t.Errorf("devicesA[0] = %+v, want nvme-serial-a1 with osd id 0", devicesA[0])
	}

	devicesB, err := provider.HostDevices(context.Background(), hostB)
	if err != nil {
		t.Fatalf("HostDevices(%s) returned error: %v", hostB, err)
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
	fake := newFakeDashboard(t, modeSuccess)
	_, err := fake.provider(t).HostDevices(context.Background(), "host-device-probe.example.invalid")
	assertProviderErrorClass(t, err, providers.ErrorNotFound)
}

func TestDaemonsNormalizesStatusEnum(t *testing.T) {
	fake := newFakeDashboard(t, modeSuccess)
	daemons, err := fake.provider(t).Daemons(context.Background())
	if err != nil {
		t.Fatalf("Daemons returned error: %v", err)
	}
	if len(daemons) != 5 {
		t.Fatalf("len(Daemons) = %d, want 5", len(daemons))
	}
	byName := map[string]string{}
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
	fake := newFakeDashboard(t, modeSuccess)
	pools, err := fake.provider(t).Pools(context.Background())
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
	fake := newFakeDashboard(t, modeSuccess)
	url := fake.server.URL
	fake.server.Close()
	provider, err := New(Config{BaseURL: url, Username: testUsername, Password: testPassword})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	_, err = provider.OSDs(context.Background())
	assertProviderErrorClass(t, err, providers.ErrorUnavailable)
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
			writeJSON(w, http.StatusCreated, loginResponse{Token: current})
			return
		}
		if logins < 2 || r.Header.Get("Authorization") != "Bearer "+current {
			http.Error(w, "stale token", http.StatusUnauthorized)
			return
		}
		writeJSON(w, http.StatusOK, []any{})
	}))
	defer server.Close()

	provider, err := New(Config{BaseURL: server.URL, Username: testUsername, Password: testPassword})
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

func TestConfigValidation(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
	}{
		{"missing base URL", Config{Username: "u", Password: "p"}},
		{"missing username", Config{BaseURL: "https://mon.example:8443", Password: "p"}},
		{"missing password", Config{BaseURL: "https://mon.example:8443", Username: "u"}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := New(test.cfg); err == nil {
				t.Fatal("New returned nil error for invalid config")
			}
		})
	}
}

func assertProviderErrorClass(t *testing.T, err error, want providers.ErrorClass) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with class %q, got nil", want)
	}
	var providerErr providers.ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("error type = %T, want ProviderError", err)
	}
	if providerErr.Class != want {
		t.Fatalf("error class = %q, want %q (message: %s)", providerErr.Class, want, providerErr.Message)
	}
}

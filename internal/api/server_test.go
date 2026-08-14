package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tonymontoya/ceph-atlas/internal/app"
	"github.com/tonymontoya/ceph-atlas/internal/config"
	"github.com/tonymontoya/ceph-atlas/internal/fleet"
	"github.com/tonymontoya/ceph-atlas/internal/inventory"
	"github.com/tonymontoya/ceph-atlas/internal/providers"
)

func TestHealthz(t *testing.T) {
	server := NewServer(app.New(config.Config{FakeScenario: "reef-healthy-baremetal"}))
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()

	server.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
}

func TestClusterEndpointUsesFakeProvider(t *testing.T) {
	server := NewServer(app.New(config.Config{FakeScenario: "reef-healthy-baremetal"}))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/current", nil)
	response := httptest.NewRecorder()

	server.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	var identity fleet.ClusterIdentity
	if err := json.NewDecoder(response.Body).Decode(&identity); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if identity.Type != fleet.ClusterTypeBareMetal {
		t.Fatalf("cluster type = %q, want %q", identity.Type, fleet.ClusterTypeBareMetal)
	}
}

func TestClusterEndpointDefaultsToFakeProviderThroughConfig(t *testing.T) {
	application, err := app.NewFromConfig(t.Context(), config.Config{
		FakeScenario: "reef-healthy-baremetal",
	})
	if err != nil {
		t.Fatalf("NewFromConfig returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = application.Close()
	})
	server := NewServer(application)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/current", nil)
	response := httptest.NewRecorder()

	server.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	var identity fleet.ClusterIdentity
	if err := json.NewDecoder(response.Body).Decode(&identity); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if identity.FSID == "" {
		t.Fatal("expected fake provider cluster identity")
	}
}

func TestClusterHealthEndpointUsesFakeProvider(t *testing.T) {
	server := NewServer(app.New(config.Config{FakeScenario: "reef-osd-down-baremetal"}))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/current/health", nil)
	response := httptest.NewRecorder()

	server.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	var health inventory.Health
	if err := json.NewDecoder(response.Body).Decode(&health); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if health.Status != inventory.HealthWarn {
		t.Fatalf("health status = %q, want %q", health.Status, inventory.HealthWarn)
	}
}

func TestOSDsEndpointUsesFakeProvider(t *testing.T) {
	server := NewServer(app.New(config.Config{FakeScenario: "reef-healthy-baremetal"}))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/current/osds", nil)
	response := httptest.NewRecorder()

	server.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	var osds []inventory.OSD
	if err := json.NewDecoder(response.Body).Decode(&osds); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(osds) == 0 {
		t.Fatal("expected OSD inventory")
	}
}

func TestHostsEndpointUsesFakeProvider(t *testing.T) {
	server := NewServer(app.New(config.Config{FakeScenario: "reef-healthy-baremetal"}))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/current/hosts", nil)
	response := httptest.NewRecorder()

	server.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	var hosts []inventory.Host
	if err := json.NewDecoder(response.Body).Decode(&hosts); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(hosts) == 0 {
		t.Fatal("expected Host inventory")
	}
}

func TestStorageDevicesEndpointListsAllHostsDevices(t *testing.T) {
	server := NewServer(app.New(config.Config{FakeScenario: "reef-healthy-baremetal"}))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/current/storage-devices", nil)
	response := httptest.NewRecorder()

	server.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	var devices []inventory.StorageDevice
	if err := json.NewDecoder(response.Body).Decode(&devices); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(devices) != 3 {
		t.Fatalf("Storage Device count = %d, want 3 across all Hosts", len(devices))
	}
	withoutOSD := 0
	for _, device := range devices {
		if device.OSDID == nil {
			withoutOSD++
		}
	}
	if withoutOSD != 1 {
		t.Fatalf("Storage Devices without an OSD link = %d, want 1", withoutOSD)
	}
}

func TestDaemonsEndpointUsesFakeProvider(t *testing.T) {
	server := NewServer(app.New(config.Config{FakeScenario: "reef-osd-down-baremetal"}))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/current/daemons", nil)
	response := httptest.NewRecorder()

	server.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	var daemons []inventory.Daemon
	if err := json.NewDecoder(response.Body).Decode(&daemons); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(daemons) == 0 {
		t.Fatal("expected Ceph Daemon inventory")
	}
	stopped := 0
	for _, daemon := range daemons {
		if daemon.Status == "stopped" {
			stopped++
		}
	}
	if stopped != 1 {
		t.Fatalf("stopped Ceph Daemon count = %d, want 1", stopped)
	}
}

func TestPoolsEndpointUsesFakeProvider(t *testing.T) {
	server := NewServer(app.New(config.Config{FakeScenario: "reef-healthy-baremetal"}))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/current/pools", nil)
	response := httptest.NewRecorder()

	server.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	var pools []inventory.Pool
	if err := json.NewDecoder(response.Body).Decode(&pools); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(pools) == 0 {
		t.Fatal("expected Pool inventory")
	}
}

func TestCasesEndpointRequiresPostgresReadSource(t *testing.T) {
	server := NewServer(app.New(config.Config{FakeScenario: "reef-healthy-baremetal"}))

	for _, path := range []string{"/api/v1/cases", "/api/v1/cases/1/timeline"} {
		t.Run(path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, path, nil)
			response := httptest.NewRecorder()

			server.Routes().ServeHTTP(response, request)

			if response.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusUnprocessableEntity, response.Body.String())
			}
			var body struct {
				Error struct {
					Class string `json:"class"`
				} `json:"error"`
			}
			if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
				t.Fatalf("decode error: %v", err)
			}
			if body.Error.Class != string(providers.ErrorUnsupported) {
				t.Fatalf("error class = %q, want %q", body.Error.Class, providers.ErrorUnsupported)
			}
		})
	}
}

func TestProviderErrorsUseStructuredEnvelope(t *testing.T) {
	server := NewServer(app.New(config.Config{FakeScenario: "missing"}))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/current/health", nil)
	response := httptest.NewRecorder()

	server.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusServiceUnavailable, response.Body.String())
	}
	var body struct {
		Error struct {
			Class   string `json:"class"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Error.Class != string(providers.ErrorUnavailable) {
		t.Fatalf("error class = %q, want %q", body.Error.Class, providers.ErrorUnavailable)
	}
	if body.Error.Message == "" {
		t.Fatal("expected non-empty error message")
	}
}

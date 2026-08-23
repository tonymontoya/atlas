package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tonymontoya/ceph-atlas/internal/app"
	"github.com/tonymontoya/ceph-atlas/internal/apperr"
	"github.com/tonymontoya/ceph-atlas/internal/config"
	"github.com/tonymontoya/ceph-atlas/internal/inventory"
)

const (
	fakeHealthyFSID = "00000000-0000-4000-8000-000000000101"
	fakeOSDDownFSID = "00000000-0000-4000-8000-000000000102"
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

func TestClusterIndexUsesFakeProvider(t *testing.T) {
	server := NewServer(app.New(config.Config{FakeScenario: "reef-healthy-baremetal"}))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/clusters", nil)
	response := httptest.NewRecorder()

	server.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	var index struct {
		Total    int `json:"total"`
		Clusters []struct {
			FSID         *string `json:"fsid"`
			Name         string  `json:"name"`
			ClusterType  string  `json:"clusterType"`
			HealthStatus *string `json:"healthStatus"`
		} `json:"clusters"`
	}
	if err := json.NewDecoder(response.Body).Decode(&index); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if index.Total != 1 || len(index.Clusters) != 1 {
		t.Fatalf("index = %+v, want exactly the provider's cluster", index)
	}
	summary := index.Clusters[0]
	if summary.FSID == nil || *summary.FSID != fakeHealthyFSID {
		t.Fatalf("summary fsid = %v, want %s", summary.FSID, fakeHealthyFSID)
	}
	if summary.Name != "reef-baremetal-healthy" || summary.ClusterType != "bare-metal" {
		t.Fatalf("summary = %+v, want the fixture identity", summary)
	}
	if summary.HealthStatus == nil || *summary.HealthStatus != "HEALTH_OK" {
		t.Fatalf("health status = %v, want HEALTH_OK", summary.HealthStatus)
	}
}

func TestClusterScopedReadsUseFakeProvider(t *testing.T) {
	healthy := NewServer(app.New(config.Config{FakeScenario: "reef-healthy-baremetal"}))
	osdDown := NewServer(app.New(config.Config{FakeScenario: "reef-osd-down-baremetal"}))

	healthResponse := serve(healthy, http.MethodGet, "/api/v1/clusters/"+fakeHealthyFSID+"/health")
	if healthResponse.Code != http.StatusOK {
		t.Fatalf("health status = %d, want %d; body=%s", healthResponse.Code, http.StatusOK, healthResponse.Body.String())
	}
	var health inventory.Health
	if err := json.NewDecoder(healthResponse.Body).Decode(&health); err != nil {
		t.Fatalf("decode health: %v", err)
	}
	if health.Status != inventory.HealthOK {
		t.Fatalf("health status = %q, want %q", health.Status, inventory.HealthOK)
	}

	warnResponse := serve(osdDown, http.MethodGet, "/api/v1/clusters/"+fakeOSDDownFSID+"/health")
	if warnResponse.Code != http.StatusOK {
		t.Fatalf("osd-down health status = %d, want %d", warnResponse.Code, http.StatusOK)
	}
	var warn inventory.Health
	if err := json.NewDecoder(warnResponse.Body).Decode(&warn); err != nil {
		t.Fatalf("decode osd-down health: %v", err)
	}
	if warn.Status != inventory.HealthWarn {
		t.Fatalf("osd-down health status = %q, want %q", warn.Status, inventory.HealthWarn)
	}

	osdsResponse := serve(healthy, http.MethodGet, "/api/v1/clusters/"+fakeHealthyFSID+"/osds")
	if osdsResponse.Code != http.StatusOK {
		t.Fatalf("osds status = %d, want %d", osdsResponse.Code, http.StatusOK)
	}
	var osds []inventory.OSD
	if err := json.NewDecoder(osdsResponse.Body).Decode(&osds); err != nil {
		t.Fatalf("decode osds: %v", err)
	}
	if len(osds) == 0 {
		t.Fatal("expected OSD inventory")
	}

	hostsResponse := serve(healthy, http.MethodGet, "/api/v1/clusters/"+fakeHealthyFSID+"/hosts")
	if hostsResponse.Code != http.StatusOK {
		t.Fatalf("hosts status = %d, want %d", hostsResponse.Code, http.StatusOK)
	}
	var hosts []inventory.Host
	if err := json.NewDecoder(hostsResponse.Body).Decode(&hosts); err != nil {
		t.Fatalf("decode hosts: %v", err)
	}
	if len(hosts) == 0 {
		t.Fatal("expected Host inventory")
	}

	devicesResponse := serve(healthy, http.MethodGet, "/api/v1/clusters/"+fakeHealthyFSID+"/storage-devices")
	if devicesResponse.Code != http.StatusOK {
		t.Fatalf("storage devices status = %d, want %d", devicesResponse.Code, http.StatusOK)
	}
	var devices []inventory.StorageDevice
	if err := json.NewDecoder(devicesResponse.Body).Decode(&devices); err != nil {
		t.Fatalf("decode devices: %v", err)
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

	daemonsResponse := serve(osdDown, http.MethodGet, "/api/v1/clusters/"+fakeOSDDownFSID+"/daemons")
	if daemonsResponse.Code != http.StatusOK {
		t.Fatalf("daemons status = %d, want %d", daemonsResponse.Code, http.StatusOK)
	}
	var daemons []inventory.Daemon
	if err := json.NewDecoder(daemonsResponse.Body).Decode(&daemons); err != nil {
		t.Fatalf("decode daemons: %v", err)
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

	poolsResponse := serve(healthy, http.MethodGet, "/api/v1/clusters/"+fakeHealthyFSID+"/pools")
	if poolsResponse.Code != http.StatusOK {
		t.Fatalf("pools status = %d, want %d", poolsResponse.Code, http.StatusOK)
	}
	var pools []inventory.Pool
	if err := json.NewDecoder(poolsResponse.Body).Decode(&pools); err != nil {
		t.Fatalf("decode pools: %v", err)
	}
	if len(pools) == 0 {
		t.Fatal("expected Pool inventory")
	}
}

func TestClusterScopedReadsRejectUnknownFSID(t *testing.T) {
	server := NewServer(app.New(config.Config{FakeScenario: "reef-healthy-baremetal"}))

	response := serve(server, http.MethodGet, "/api/v1/clusters/00000000-0000-4000-8000-0000000009ff/health")
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusNotFound, response.Body.String())
	}
	if class := decodeErrorClass(t, response); class != "NotFound" {
		t.Fatalf("error class = %q, want NotFound", class)
	}
}

func TestClusterIndexRejectsBadPaging(t *testing.T) {
	server := NewServer(app.New(config.Config{FakeScenario: "reef-healthy-baremetal"}))

	response := serve(server, http.MethodGet, "/api/v1/clusters?limit=-3")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusBadRequest, response.Body.String())
	}
	response = serve(server, http.MethodGet, "/api/v1/clusters?offset=abc")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusBadRequest, response.Body.String())
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
			if body.Error.Class != string(apperr.Unsupported) {
				t.Fatalf("error class = %q, want %q", body.Error.Class, apperr.Unsupported)
			}
		})
	}
}

func TestProviderErrorsUseStructuredEnvelope(t *testing.T) {
	server := NewServer(app.New(config.Config{FakeScenario: "missing"}))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/clusters/"+fakeHealthyFSID+"/health", nil)
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
	if body.Error.Class != string(apperr.Unavailable) {
		t.Fatalf("error class = %q, want %q", body.Error.Class, apperr.Unavailable)
	}
	if body.Error.Message == "" {
		t.Fatal("expected non-empty error message")
	}
}

func TestWriteErrorMapsInvalidRequestToBadRequest(t *testing.T) {
	response := httptest.NewRecorder()

	writeError(response, apperr.Error{Class: apperr.InvalidRequest, Message: "title is required"})

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusBadRequest, response.Body.String())
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
	if body.Error.Class != "InvalidRequest" {
		t.Fatalf("error class = %q, want InvalidRequest", body.Error.Class)
	}
	if body.Error.Message != "title is required" {
		t.Fatalf("error message = %q, want the store message", body.Error.Message)
	}
}

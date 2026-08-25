package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/tonymontoya/ceph-atlas/internal/app"
	"github.com/tonymontoya/ceph-atlas/internal/config"
	"github.com/tonymontoya/ceph-atlas/internal/fleet"
	"github.com/tonymontoya/ceph-atlas/internal/testdb"
)

type registrationResponse struct {
	Cluster struct {
		ID             int64   `json:"id"`
		FSID           *string `json:"fsid"`
		Name           string  `json:"name"`
		ClusterType    string  `json:"clusterType"`
		DeregisteredAt *string `json:"deregisteredAt"`
	} `json:"cluster"`
	EnrollmentCredential struct {
		Token     string `json:"token"`
		ExpiresAt string `json:"expiresAt"`
	} `json:"enrollmentCredential"`
}

func cleanupRegistrationRows(t *testing.T) {
	t.Helper()
	db, _ := testdb.Open(t)
	testdb.DeleteClusters(t, db, "name LIKE 'api-registration-test%'")
}

func TestCreateClusterRegistrationRequiresAuthentication(t *testing.T) {
	harness := newWriteHarness(t)

	response := harness.do(t, http.MethodPost, "/api/v1/clusters", map[string]string{
		"name":        "api-registration-test-unauth",
		"clusterType": "bare-metal",
	}, false)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
	if decodeErrorClass(t, response) != "Unauthorized" {
		t.Fatalf("error class = %q, want Unauthorized", decodeErrorClass(t, response))
	}
}

func TestClusterRegistrationLifecycleOverAPI(t *testing.T) {
	harness := newWriteHarness(t)
	cleanupRegistrationRows(t)
	defer cleanupRegistrationRows(t)

	response := harness.do(t, http.MethodPost, "/api/v1/clusters", map[string]string{
		"name":        "api-registration-test-lifecycle",
		"clusterType": "bare-metal",
	}, true)
	if response.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", response.Code, response.Body.String())
	}
	var created registrationResponse
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.Cluster.ID <= 0 {
		t.Fatalf("cluster id = %d, want positive", created.Cluster.ID)
	}
	if created.Cluster.FSID != nil {
		t.Fatalf("cluster fsid = %v, want nil until enrollment", created.Cluster.FSID)
	}
	if created.EnrollmentCredential.Token == "" {
		t.Fatal("create response must carry the one-time enrollment credential")
	}

	// The credential never appears again: GET returns the registration only.
	getResponse := harness.do(t, http.MethodGet, "/api/v1/clusters/"+strconv.FormatInt(created.Cluster.ID, 10), nil, true)
	if getResponse.Code != http.StatusOK {
		t.Fatalf("get status = %d, body = %s", getResponse.Code, getResponse.Body.String())
	}
	var fetched struct {
		ID                   int64  `json:"id"`
		Name                 string `json:"name"`
		ClusterType          string `json:"clusterType"`
		EnrollmentCredential *struct {
			Token string `json:"token"`
		} `json:"enrollmentCredential"`
	}
	if err := json.Unmarshal(getResponse.Body.Bytes(), &fetched); err != nil {
		t.Fatalf("decode get response: %v", err)
	}
	if fetched.ID != created.Cluster.ID || fetched.Name != "api-registration-test-lifecycle" {
		t.Fatalf("fetched = %+v", fetched)
	}
	if fetched.ClusterType != string(fleet.ClusterTypeBareMetal) {
		t.Fatalf("clusterType = %q, want bare-metal", fetched.ClusterType)
	}
	if fetched.EnrollmentCredential != nil {
		t.Fatal("get response must never re-expose the enrollment credential")
	}

	deleteResponse := harness.do(t, http.MethodDelete, "/api/v1/clusters/"+strconv.FormatInt(created.Cluster.ID, 10), nil, true)
	if deleteResponse.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body = %s", deleteResponse.Code, deleteResponse.Body.String())
	}
	var deregistered struct {
		DeregisteredAt *string `json:"deregisteredAt"`
	}
	if err := json.Unmarshal(deleteResponse.Body.Bytes(), &deregistered); err != nil {
		t.Fatalf("decode delete response: %v", err)
	}
	if deregistered.DeregisteredAt == nil {
		t.Fatal("deregistered response should carry deregisteredAt")
	}

	// Deleting again conflicts; deleting without a token is still 401.
	again := harness.do(t, http.MethodDelete, "/api/v1/clusters/"+strconv.FormatInt(created.Cluster.ID, 10), nil, true)
	if again.Code != http.StatusConflict {
		t.Fatalf("second delete status = %d, want 409", again.Code)
	}
	unauthenticated := harness.do(t, http.MethodDelete, "/api/v1/clusters/"+strconv.FormatInt(created.Cluster.ID, 10), nil, false)
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated delete status = %d, want 401", unauthenticated.Code)
	}
	unauthenticatedGet := harness.do(t, http.MethodGet, "/api/v1/clusters/"+strconv.FormatInt(created.Cluster.ID, 10), nil, false)
	if unauthenticatedGet.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated get status = %d, want 401", unauthenticatedGet.Code)
	}
}

func TestClusterRegistrationValidationOverAPI(t *testing.T) {
	harness := newWriteHarness(t)
	cleanupRegistrationRows(t)
	defer cleanupRegistrationRows(t)

	response := harness.do(t, http.MethodPost, "/api/v1/clusters", map[string]string{
		"name":        "",
		"clusterType": "bare-metal",
	}, true)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("missing-name status = %d, want 400", response.Code)
	}

	rook := harness.do(t, http.MethodPost, "/api/v1/clusters", map[string]string{
		"name":        "api-registration-test-rook",
		"clusterType": "rook",
	}, true)
	if rook.Code != http.StatusBadRequest {
		t.Fatalf("rook status = %d, want 400 (bare-metal only in v0.7)", rook.Code)
	}

	missing := harness.do(t, http.MethodGet, "/api/v1/clusters/999999999", nil, true)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("unknown id status = %d, want 404", missing.Code)
	}
}

func TestClusterRegistrationUnsupportedWithoutPostgresReadSource(t *testing.T) {
	server := NewServer(app.New(config.Config{FakeScenario: "reef-healthy-baremetal"}))

	// Registration management is bearer-authenticated like the other
	// operator endpoints (RBAC lands in v0.8); the unauthenticated GET
	// reports 401 before the unsupported read source is reachable.
	get := httptestRequest(t, server, http.MethodGet, "/api/v1/clusters/1", nil)
	if get.Code != http.StatusUnauthorized {
		t.Fatalf("get status = %d, want 401", get.Code)
	}

	// Writes run behind requireIdentity, so an unauthenticated call
	// reports 401 before the unsupported read source is reachable.
	post := httptestRequest(t, server, http.MethodPost, "/api/v1/clusters", map[string]string{
		"name":        "api-registration-test-provider-mode",
		"clusterType": "bare-metal",
	})
	if post.Code != http.StatusUnauthorized {
		t.Fatalf("post status = %d, want 401", post.Code)
	}
	deleteResponse := httptestRequest(t, server, http.MethodDelete, "/api/v1/clusters/1", nil)
	if deleteResponse.Code != http.StatusUnauthorized {
		t.Fatalf("delete status = %d, want 401", deleteResponse.Code)
	}
}

func httptestRequest(t *testing.T, server *Server, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("encode body: %v", err)
		}
		reader = bytes.NewReader(encoded)
	} else {
		reader = bytes.NewReader(nil)
	}
	request := httptest.NewRequest(method, path, reader)
	response := httptest.NewRecorder()
	server.Routes().ServeHTTP(response, request)
	return response
}

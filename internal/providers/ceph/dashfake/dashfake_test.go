package dashfake

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHandlerServesDashboardContract pins the pure handler's contract
// standalone — the login flow, the bearer guard, and one data route —
// so the dev-stack container serves the same fixture shape the
// dashtest-wrapped provider tests exercise.
func TestHandlerServesDashboardContract(t *testing.T) {
	dashboard := NewDashboard(ModeSuccess)
	server := httptest.NewServer(dashboard.Handler())
	t.Cleanup(server.Close)

	loginResponse, err := http.Post(server.URL+"/api/auth", "application/json", nil)
	if err != nil {
		t.Fatalf("post login: %v", err)
	}
	defer func() { _ = loginResponse.Body.Close() }()
	if loginResponse.StatusCode != http.StatusCreated {
		t.Fatalf("login status = %d, want %d", loginResponse.StatusCode, http.StatusCreated)
	}
	var login struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(loginResponse.Body).Decode(&login); err != nil {
		t.Fatalf("decode login: %v", err)
	}
	if login.Token != Token {
		t.Fatalf("login token = %q, want %q", login.Token, Token)
	}

	request, _ := http.NewRequest(http.MethodGet, server.URL+"/api/health/get_cluster_fsid", nil)
	request.Header.Set("Authorization", "Bearer "+login.Token)
	fsIDResponse, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("get cluster fsid: %v", err)
	}
	defer func() { _ = fsIDResponse.Body.Close() }()
	if fsIDResponse.StatusCode != http.StatusOK {
		t.Fatalf("cluster fsid status = %d, want %d", fsIDResponse.StatusCode, http.StatusOK)
	}
	var fsid string
	if err := json.NewDecoder(fsIDResponse.Body).Decode(&fsid); err != nil {
		t.Fatalf("decode cluster fsid: %v", err)
	}
	if fsid != FSID {
		t.Fatalf("cluster fsid = %q, want %q", fsid, FSID)
	}

	if dashboard.Logins() != 1 {
		t.Fatalf("logins = %d, want 1", dashboard.Logins())
	}

	unauthenticated, err := http.Get(server.URL + "/api/summary")
	if err != nil {
		t.Fatalf("get summary without token: %v", err)
	}
	defer func() { _ = unauthenticated.Body.Close() }()
	if unauthenticated.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated summary status = %d, want %d", unauthenticated.StatusCode, http.StatusUnauthorized)
	}
}

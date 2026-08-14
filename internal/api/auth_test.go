package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tonymontoya/ceph-atlas/internal/app"
	"github.com/tonymontoya/ceph-atlas/internal/config"
	"github.com/tonymontoya/ceph-atlas/internal/identity"
	"github.com/tonymontoya/ceph-atlas/internal/identity/devissuer"
	"github.com/tonymontoya/ceph-atlas/internal/providers"
)

type authTestHarness struct {
	server *Server
	token  string
}

func newAuthTestHarness(t *testing.T) *authTestHarness {
	t.Helper()
	issuer, err := devissuer.New("https://atlas-dev-issuer.local", "atlas-api")
	if err != nil {
		t.Fatalf("create dev issuer: %v", err)
	}
	jwks := httptest.NewServer(issuer.Handler())
	t.Cleanup(jwks.Close)
	token, err := issuer.IssueToken("operator-1", "Storage Operator", 15*time.Minute)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	verifier := identity.NewVerifier(identity.Config{
		Issuer:   "https://atlas-dev-issuer.local",
		Audience: "atlas-api",
		JWKSURL:  jwks.URL + "/.well-known/jwks.json",
	})
	return &authTestHarness{
		server: NewServer(&app.App{
			Config:   config.Config{FakeScenario: "reef-healthy-baremetal"},
			Verifier: verifier,
		}),
		token: token,
	}
}

func (h *authTestHarness) get(t *testing.T, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	h.server.Routes().ServeHTTP(response, request)
	return response
}

func TestMeReturnsAuthenticatedOperator(t *testing.T) {
	harness := newAuthTestHarness(t)

	response := harness.get(t, "/api/v1/me", harness.token)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	var body struct {
		Subject     string `json:"subject"`
		DisplayName string `json:"displayName"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Subject != "operator-1" {
		t.Errorf("subject = %q, want %q", body.Subject, "operator-1")
	}
	if body.DisplayName != "Storage Operator" {
		t.Errorf("displayName = %q, want %q", body.DisplayName, "Storage Operator")
	}
}

func TestMeRequiresBearerToken(t *testing.T) {
	harness := newAuthTestHarness(t)

	response := harness.get(t, "/api/v1/me", "")

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusUnauthorized, response.Body.String())
	}
	var body struct {
		Error struct {
			Class   string `json:"class"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if body.Error.Class != string(providers.ErrorUnauthorized) {
		t.Fatalf("error class = %q, want %q", body.Error.Class, providers.ErrorUnauthorized)
	}
	if body.Error.Message == "" {
		t.Fatal("expected non-empty error message")
	}
}

func TestMeRejectsInvalidToken(t *testing.T) {
	harness := newAuthTestHarness(t)

	response := harness.get(t, "/api/v1/me", "forged-token-value")

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusUnauthorized, response.Body.String())
	}
}

func TestMeFailsClosedWithoutVerifier(t *testing.T) {
	server := NewServer(app.New(config.Config{FakeScenario: "reef-healthy-baremetal"}))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	request.Header.Set("Authorization", "Bearer anything")
	response := httptest.NewRecorder()

	server.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusUnauthorized, response.Body.String())
	}
}

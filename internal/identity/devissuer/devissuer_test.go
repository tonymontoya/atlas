package devissuer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/tonymontoya/ceph-atlas/internal/identity"
)

func TestDevIssuerTokenVerifies(t *testing.T) {
	issuer, err := New("https://atlas-dev-issuer.local", "atlas-api")
	if err != nil {
		t.Fatalf("create dev issuer: %v", err)
	}
	server := httptest.NewServer(issuer.Handler())
	t.Cleanup(server.Close)
	verifier := identity.NewVerifier(identity.Config{
		Issuer:   "https://atlas-dev-issuer.local",
		Audience: "atlas-api",
		JWKSURL:  server.URL + "/.well-known/jwks.json",
	})

	token, err := issuer.IssueToken("dev-operator", "Dev Operator", 15*time.Minute)
	if err != nil {
		t.Fatalf("issue dev token: %v", err)
	}
	id, err := verifier.Verify(context.Background(), token)
	if err != nil {
		t.Fatalf("verify dev-issued token: %v", err)
	}
	if id.Subject != "dev-operator" {
		t.Errorf("Subject = %q, want %q", id.Subject, "dev-operator")
	}
	if id.DisplayName != "Dev Operator" {
		t.Errorf("DisplayName = %q, want %q", id.DisplayName, "Dev Operator")
	}
}

func TestDevIssuerJWKSIsServed(t *testing.T) {
	issuer, err := New("https://atlas-dev-issuer.local", "atlas-api")
	if err != nil {
		t.Fatalf("create dev issuer: %v", err)
	}
	server := httptest.NewServer(issuer.Handler())
	t.Cleanup(server.Close)

	resp, err := http.Get(server.URL + "/.well-known/jwks.json")
	if err != nil {
		t.Fatalf("get jwks: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("jwks status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if contentType := resp.Header.Get("Content-Type"); contentType != "application/json" {
		t.Errorf("jwks content type = %q, want application/json", contentType)
	}
}

func TestDevIssuerTokenEndpointReturnsToken(t *testing.T) {
	issuer, err := New("https://atlas-dev-issuer.local", "atlas-api")
	if err != nil {
		t.Fatalf("create dev issuer: %v", err)
	}
	server := httptest.NewServer(issuer.Handler())
	t.Cleanup(server.Close)

	resp, err := http.Post(server.URL+"/token", "application/json", nil)
	if err != nil {
		t.Fatalf("post token endpoint: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("token status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	var body struct {
		Token       string `json:"token"`
		ExpiresIn   int    `json:"expiresIn"`
		TokenType   string `json:"tokenType"`
		DisplayName string `json:"displayName"`
		Subject     string `json:"subject"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode token response: %v", err)
	}
	if body.Token == "" || body.TokenType != "Bearer" || body.ExpiresIn <= 0 {
		t.Fatalf("token response incomplete: %+v", body)
	}
	claims := jwt.MapClaims{}
	if _, _, err := jwt.NewParser().ParseUnverified(body.Token, claims); err != nil {
		t.Fatalf("parse issued token: %v", err)
	}
	if sub, _ := claims["sub"].(string); sub != body.Subject || sub == "" {
		t.Errorf("token sub = %q, response subject = %q", sub, body.Subject)
	}
}

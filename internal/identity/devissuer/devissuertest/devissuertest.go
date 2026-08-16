// Package devissuertest serves a dev issuer over an in-process HTTP
// server for tests that need OIDC bearer tokens, mirroring the dashtest
// pattern for the ceph provider. Test-only; production code must not
// import it.
package devissuertest

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tonymontoya/ceph-atlas/internal/identity/devissuer"
)

const (
	// IssuerURL is the issuer identifier to configure on verifiers.
	IssuerURL = "https://atlas-dev-issuer.local"

	// Audience is the token audience to configure on verifiers.
	Audience = "atlas-api"
)

// Server serves a fresh dev issuer's endpoints and closes with the test.
type Server struct {
	issuer  *devissuer.Issuer
	jwksURL string
}

// Start serves a fresh dev issuer and registers its shutdown.
func Start(t testing.TB) *Server {
	t.Helper()
	issuer, err := devissuer.New(IssuerURL, Audience)
	if err != nil {
		t.Fatalf("create dev issuer: %v", err)
	}
	jwks := httptest.NewServer(issuer.Handler())
	t.Cleanup(jwks.Close)
	return &Server{issuer: issuer, jwksURL: jwks.URL + issuer.JWKSPath()}
}

// JWKSURL returns the well-known JWKS endpoint of the served issuer.
func (s *Server) JWKSURL() string {
	return s.jwksURL
}

// Token issues a 15-minute bearer token for the given operator.
func (s *Server) Token(t testing.TB, subject, displayName string) string {
	t.Helper()
	token, err := s.issuer.IssueToken(subject, displayName, 15*time.Minute)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	return token
}

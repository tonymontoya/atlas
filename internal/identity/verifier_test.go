package identity

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	testIssuer   = "https://issuer.example.com"
	testAudience = "atlas-api"
	testKeyID    = "test-key-1"
)

type testIssuerKeys struct {
	key     *rsa.PrivateKey
	kid     string
	jwksURL string
}

func (k *testIssuerKeys) rotate(t *testing.T) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rotation key: %v", err)
	}
	k.key = key
	k.kid = "test-key-2"
}

func newTestIssuerKeys(t *testing.T) *testIssuerKeys {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	keys := &testIssuerKeys{key: key, kid: testKeyID}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/jwks.json", func(w http.ResponseWriter, r *http.Request) {
		writeJWKS(t, w, keys.kid, &keys.key.PublicKey)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	keys.jwksURL = server.URL + "/.well-known/jwks.json"
	return keys
}

func writeJWKS(t *testing.T, w http.ResponseWriter, kid string, key *rsa.PublicKey) {
	t.Helper()
	response := map[string]any{
		"keys": []map[string]string{{
			"kty": "RSA",
			"kid": kid,
			"alg": "RS256",
			"use": "sig",
			"n":   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString(bigEndianExponent()),
		}},
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		t.Fatalf("encode jwks: %v", err)
	}
}

func bigEndianExponent() []byte {
	return []byte{0x01, 0x00, 0x01}
}

func (k *testIssuerKeys) issueToken(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	now := time.Now()
	if _, ok := claims["iat"]; !ok {
		claims["iat"] = now.Unix()
	}
	if _, ok := claims["exp"]; !ok {
		claims["exp"] = now.Add(15 * time.Minute).Unix()
	}
	if _, ok := claims["iss"]; !ok {
		claims["iss"] = testIssuer
	}
	if _, ok := claims["aud"]; !ok {
		claims["aud"] = testAudience
	}
	if _, ok := claims["sub"]; !ok {
		claims["sub"] = "operator-1"
	}
	if _, ok := claims["name"]; !ok {
		claims["name"] = "Storage Operator"
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = k.kid
	signed, err := token.SignedString(k.key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

func newTestVerifier(keys *testIssuerKeys) *Verifier {
	return NewVerifier(Config{
		Issuer:   testIssuer,
		Audience: testAudience,
		JWKSURL:  keys.jwksURL,
	})
}

func TestVerifierAcceptsValidToken(t *testing.T) {
	keys := newTestIssuerKeys(t)
	verifier := newTestVerifier(keys)

	id, err := verifier.Verify(context.Background(), keys.issueToken(t, jwt.MapClaims{}))
	if err != nil {
		t.Fatalf("Verify returned error: %v", err)
	}
	if id.Subject != "operator-1" {
		t.Errorf("Subject = %q, want %q", id.Subject, "operator-1")
	}
	if id.DisplayName != "Storage Operator" {
		t.Errorf("DisplayName = %q, want %q", id.DisplayName, "Storage Operator")
	}
}

func TestVerifierRejectsExpiredToken(t *testing.T) {
	keys := newTestIssuerKeys(t)
	verifier := newTestVerifier(keys)
	token := keys.issueToken(t, jwt.MapClaims{
		"iat": time.Now().Add(-2 * time.Hour).Unix(),
		"exp": time.Now().Add(-1 * time.Hour).Unix(),
	})

	if _, err := verifier.Verify(context.Background(), token); err == nil {
		t.Fatal("Verify accepted an expired token")
	}
}

func TestVerifierRejectsWrongAudience(t *testing.T) {
	keys := newTestIssuerKeys(t)
	verifier := newTestVerifier(keys)
	token := keys.issueToken(t, jwt.MapClaims{"aud": "some-other-api"})

	if _, err := verifier.Verify(context.Background(), token); err == nil {
		t.Fatal("Verify accepted a token for another audience")
	}
}

func TestVerifierRejectsWrongIssuer(t *testing.T) {
	keys := newTestIssuerKeys(t)
	verifier := newTestVerifier(keys)
	token := keys.issueToken(t, jwt.MapClaims{"iss": "https://evil.example.com"})

	if _, err := verifier.Verify(context.Background(), token); err == nil {
		t.Fatal("Verify accepted a token from another issuer")
	}
}

func TestVerifierRejectsBadSignature(t *testing.T) {
	keys := newTestIssuerKeys(t)
	verifier := newTestVerifier(keys)

	other, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate attacker key: %v", err)
	}
	forged := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": testIssuer,
		"aud": testAudience,
		"sub": "attacker",
		"exp": time.Now().Add(15 * time.Minute).Unix(),
	})
	forged.Header["kid"] = testKeyID
	signed, err := forged.SignedString(other)
	if err != nil {
		t.Fatalf("sign forged token: %v", err)
	}

	if _, err := verifier.Verify(context.Background(), signed); err == nil {
		t.Fatal("Verify accepted a token signed by an unknown key")
	}
}

func TestVerifierRejectsMissingSubject(t *testing.T) {
	keys := newTestIssuerKeys(t)
	verifier := newTestVerifier(keys)
	token := keys.issueToken(t, jwt.MapClaims{"sub": ""})

	if _, err := verifier.Verify(context.Background(), token); err == nil {
		t.Fatal("Verify accepted a token without a subject")
	}
}

func TestVerifierRejectsGarbageToken(t *testing.T) {
	keys := newTestIssuerKeys(t)
	verifier := newTestVerifier(keys)

	if _, err := verifier.Verify(context.Background(), "not-a-token"); err == nil {
		t.Fatal("Verify accepted a non-token string")
	}
}

func TestVerifierRefreshesKeysAfterRotation(t *testing.T) {
	keys := newTestIssuerKeys(t)
	verifier := newTestVerifier(keys)

	before := keys.issueToken(t, jwt.MapClaims{})
	if _, err := verifier.Verify(context.Background(), before); err != nil {
		t.Fatalf("Verify before rotation returned error: %v", err)
	}

	keys.rotate(t)
	after := keys.issueToken(t, jwt.MapClaims{"sub": "operator-2"})
	id, err := verifier.Verify(context.Background(), after)
	if err != nil {
		t.Fatalf("Verify after rotation returned error: %v", err)
	}
	if id.Subject != "operator-2" {
		t.Errorf("Subject = %q, want %q", id.Subject, "operator-2")
	}
}

func TestVerifierUsesDisplayNameFallbacks(t *testing.T) {
	keys := newTestIssuerKeys(t)
	verifier := newTestVerifier(keys)

	token := keys.issueToken(t, jwt.MapClaims{
		"name":               "",
		"preferred_username": "preferred-op",
	})
	id, err := verifier.Verify(context.Background(), token)
	if err != nil {
		t.Fatalf("Verify returned error: %v", err)
	}
	if id.DisplayName != "preferred-op" {
		t.Errorf("DisplayName = %q, want preferred_username fallback", id.DisplayName)
	}

	token = keys.issueToken(t, jwt.MapClaims{"name": "", "preferred_username": ""})
	id, err = verifier.Verify(context.Background(), token)
	if err != nil {
		t.Fatalf("Verify returned error: %v", err)
	}
	if id.DisplayName != "operator-1" {
		t.Errorf("DisplayName = %q, want subject fallback", id.DisplayName)
	}
}

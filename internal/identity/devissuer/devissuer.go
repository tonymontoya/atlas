package devissuer

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/tonymontoya/ceph-atlas/internal/apperr"
)

const (
	keyID           = "atlas-dev-issuer-key-1"
	defaultTokenTTL = 15 * time.Minute
	defaultSubject  = "dev-operator"
	defaultName     = "Dev Operator"
)

type Issuer struct {
	issuerURL string
	audience  string
	key       *rsa.PrivateKey
	now       func() time.Time
}

func New(issuerURL, audience string) (*Issuer, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generate dev issuer key: %w", err)
	}
	return &Issuer{
		issuerURL: issuerURL,
		audience:  audience,
		key:       key,
		now:       time.Now,
	}, nil
}

func (i *Issuer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/jwks.json", i.serveJWKS)
	mux.HandleFunc("POST /token", i.serveToken)
	mux.HandleFunc("GET /token", i.serveToken)
	return mux
}

func (i *Issuer) JWKSPath() string  { return "/.well-known/jwks.json" }
func (i *Issuer) TokenPath() string { return "/token" }

func (i *Issuer) IssueToken(subject, displayName string, ttl time.Duration) (string, error) {
	subject, displayName = resolveDefaults(subject, displayName)
	if ttl <= 0 {
		ttl = defaultTokenTTL
	}
	now := i.now()
	claims := jwt.MapClaims{
		"iss":  i.issuerURL,
		"aud":  i.audience,
		"sub":  subject,
		"name": displayName,
		"iat":  now.Unix(),
		"exp":  now.Add(ttl).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = keyID
	signed, err := token.SignedString(i.key)
	if err != nil {
		return "", fmt.Errorf("sign dev token: %w", err)
	}
	return signed, nil
}

func (i *Issuer) serveJWKS(w http.ResponseWriter, r *http.Request) {
	public := &i.key.PublicKey
	response := map[string]any{
		"keys": []map[string]string{{
			"kty": "RSA",
			"kid": keyID,
			"alg": "RS256",
			"use": "sig",
			"n":   base64.RawURLEncoding.EncodeToString(public.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString([]byte{0x01, 0x00, 0x01}),
		}},
	}
	writeJSON(w, http.StatusOK, response)
}

func (i *Issuer) serveToken(w http.ResponseWriter, r *http.Request) {
	subject := r.FormValue("subject")
	displayName := r.FormValue("displayName")
	ttl := defaultTokenTTL
	if requested := r.FormValue("ttl"); requested != "" {
		if parsed, err := time.ParseDuration(requested); err == nil && parsed > 0 {
			ttl = parsed
		}
	}
	token, err := i.IssueToken(subject, displayName, ttl)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": map[string]string{"class": string(apperr.Internal), "message": err.Error()},
		})
		return
	}
	resolvedSubject, resolvedName := resolveDefaults(subject, displayName)
	writeJSON(w, http.StatusOK, map[string]any{
		"token":       token,
		"tokenType":   "Bearer",
		"expiresIn":   int(ttl.Seconds()),
		"subject":     resolvedSubject,
		"displayName": resolvedName,
	})
}

func resolveDefaults(subject, displayName string) (string, string) {
	if subject == "" {
		subject = defaultSubject
	}
	if displayName == "" {
		displayName = defaultName
	}
	return subject, displayName
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

package identity

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	clockSkewLeeway  = 30 * time.Second
	jwksFetchTimeout = 10 * time.Second
)

type Config struct {
	Issuer   string
	Audience string
	JWKSURL  string
}

type Identity struct {
	Subject     string
	DisplayName string
}

type Verifier struct {
	config Config
	client *http.Client

	mu   sync.RWMutex
	keys map[string]*rsa.PublicKey
}

func NewVerifier(cfg Config) *Verifier {
	return &Verifier{
		config: cfg,
		client: &http.Client{Timeout: jwksFetchTimeout},
	}
}

func (v *Verifier) Verify(ctx context.Context, rawToken string) (Identity, error) {
	claims := jwt.MapClaims{}
	_, err := jwt.ParseWithClaims(rawToken, claims, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method %v", token.Header["alg"])
		}
		kid, _ := token.Header["kid"].(string)
		key, err := v.key(ctx, kid)
		if err != nil {
			return nil, err
		}
		return key, nil
	}, jwt.WithValidMethods([]string{"RS256"}), jwt.WithIssuer(v.config.Issuer),
		jwt.WithAudience(v.config.Audience), jwt.WithExpirationRequired(),
		jwt.WithLeeway(clockSkewLeeway))
	if err != nil {
		return Identity{}, fmt.Errorf("verify token: %w", err)
	}
	subject, err := claims.GetSubject()
	if err != nil || subject == "" {
		return Identity{}, errors.New("token has no subject claim")
	}
	return Identity{
		Subject:     subject,
		DisplayName: displayName(claims, subject),
	}, nil
}

func displayName(claims jwt.MapClaims, subject string) string {
	if name, ok := claims["name"].(string); ok && name != "" {
		return name
	}
	if name, ok := claims["preferred_username"].(string); ok && name != "" {
		return name
	}
	return subject
}

func (v *Verifier) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	if kid == "" {
		return nil, errors.New("token header has no kid")
	}
	v.mu.RLock()
	key, ok := v.keys[kid]
	v.mu.RUnlock()
	if ok {
		return key, nil
	}
	if err := v.refreshKeys(ctx); err != nil {
		return nil, err
	}
	v.mu.RLock()
	defer v.mu.RUnlock()
	if key, ok := v.keys[kid]; ok {
		return key, nil
	}
	return nil, fmt.Errorf("no jwks key matches kid %q", kid)
}

func (v *Verifier) refreshKeys(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.config.JWKSURL, nil)
	if err != nil {
		return fmt.Errorf("build jwks request: %w", err)
	}
	resp, err := v.client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch jwks: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch jwks: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read jwks body: %w", err)
	}
	var jwks struct {
		Keys []struct {
			Kty string `json:"kty"`
			Kid string `json:"kid"`
			Alg string `json:"alg"`
			Use string `json:"use"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(body, &jwks); err != nil {
		return fmt.Errorf("parse jwks: %w", err)
	}
	keys := make(map[string]*rsa.PublicKey)
	for _, entry := range jwks.Keys {
		if entry.Kty != "RSA" || entry.Use != "sig" || entry.N == "" || entry.E == "" {
			continue
		}
		if entry.Alg != "" && entry.Alg != "RS256" {
			continue
		}
		n, err := decodeBase64URLBigInt(entry.N)
		if err != nil {
			continue
		}
		e, err := decodeBase64URLInt(entry.E)
		if err != nil {
			continue
		}
		if entry.Kid == "" {
			continue
		}
		keys[entry.Kid] = &rsa.PublicKey{N: n, E: e}
	}
	if len(keys) == 0 {
		return errors.New("jwks contained no usable RSA signing keys")
	}
	v.mu.Lock()
	v.keys = keys
	v.mu.Unlock()
	return nil
}

func decodeBase64URLBigInt(value string) (*big.Int, error) {
	bytes, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, err
	}
	return new(big.Int).SetBytes(bytes), nil
}

func decodeBase64URLInt(value string) (int, error) {
	bytes, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return 0, err
	}
	var result int
	for _, b := range bytes {
		if result > (1<<62)/256 {
			return 0, errors.New("exponent overflow")
		}
		result = result<<8 | int(b)
	}
	return result, nil
}

package ceph

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tonymontoya/ceph-atlas/internal/providers"
)

const (
	requestTimeout  = 30 * time.Second
	defaultPageSize = 1000
	maxPages        = 100
	defaultName     = "ceph"
)

type doer interface {
	Do(r *http.Request) (*http.Response, error)
}

type Config struct {
	BaseURL            string
	Username           string
	Password           string
	ClusterName        string
	InsecureSkipVerify bool
	HTTPClient         *http.Client
}

func (c Config) withDefaults() Config {
	if c.ClusterName == "" {
		c.ClusterName = defaultName
	}
	return c
}

func (c Config) validate() error {
	if c.BaseURL == "" {
		return errors.New("base URL is required")
	}
	if _, err := url.Parse(c.BaseURL); err != nil {
		return fmt.Errorf("BaseURL %q is not a valid URL: %w", c.BaseURL, err)
	}
	if c.Username == "" {
		return errors.New("username is required")
	}
	if c.Password == "" {
		return errors.New("password is required")
	}
	return nil
}

type Provider struct {
	baseURL     *url.URL
	username    string
	password    string
	clusterName string
	client      doer
	pageSize    int

	mu    sync.Mutex
	token string
}

func New(cfg Config) (*Provider, error) {
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid ceph provider config: %w", err)
	}
	cfg = cfg.withDefaults()
	baseURL, err := url.Parse(strings.TrimRight(cfg.BaseURL, "/"))
	if err != nil {
		return nil, fmt.Errorf("invalid ceph provider config: %w", err)
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: requestTimeout}
		if cfg.InsecureSkipVerify {
			client.Transport = &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			}
		}
	}
	return &Provider{
		baseURL:     baseURL,
		username:    cfg.Username,
		password:    cfg.Password,
		clusterName: cfg.ClusterName,
		client:      client,
		pageSize:    defaultPageSize,
	}, nil
}

func providerErr(class providers.ErrorClass, format string, args ...any) error {
	return providers.ProviderError{Class: class, Message: fmt.Sprintf(format, args...)}
}

func ctxErr(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return providerErr(providers.ErrorTimeout, "context done before request: %v", err)
	}
	return nil
}

func classifyTransportErr(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return providerErr(providers.ErrorTimeout, "request context ended: %v", err)
	}
	return providerErr(providers.ErrorUnavailable, "dashboard request failed: %v", err)
}

func classifyStatus(status int, what string) error {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return providerErr(providers.ErrorUnauthorized, "%s returned HTTP %d", what, status)
	case status == http.StatusNotFound:
		return providerErr(providers.ErrorNotFound, "%s returned HTTP 404", what)
	case status >= 500:
		return providerErr(providers.ErrorUnavailable, "%s returned HTTP %d", what, status)
	default:
		return providerErr(providers.ErrorUnavailable, "%s returned unexpected HTTP %d", what, status)
	}
}

type loginResponse struct {
	Token string `json:"token"`
}

// tokenRejected marks a ProviderError caused by HTTP 401 specifically, so
// getJSON can distinguish "re-auth might help" (401) from "permission
// denied" (403, same Unauthorized class, no retry).
type tokenRejected struct {
	providers.ProviderError
}

func (p *Provider) authorize(ctx context.Context) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	body, err := json.Marshal(map[string]string{
		"username": p.username,
		"password": p.password,
	})
	if err != nil {
		return providerErr(providers.ErrorUnavailable, "encode login request: %v", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL.String()+"/api/auth", bytes.NewReader(body))
	if err != nil {
		return providerErr(providers.ErrorUnavailable, "build login request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return classifyTransportErr(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusBadRequest {
		return providerErr(providers.ErrorUnauthorized, "dashboard login rejected with HTTP %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusCreated {
		return classifyStatus(resp.StatusCode, "dashboard login")
	}
	var login loginResponse
	if err := json.NewDecoder(resp.Body).Decode(&login); err != nil {
		return providerErr(providers.ErrorMalformedResponse, "decode login response: %v", err)
	}
	if login.Token == "" {
		return providerErr(providers.ErrorMalformedResponse, "login response carried no token")
	}
	p.mu.Lock()
	p.token = login.Token
	p.mu.Unlock()
	return nil
}

func (p *Provider) currentToken() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.token
}

func (p *Provider) clearToken() {
	p.mu.Lock()
	p.token = ""
	p.mu.Unlock()
}

func (p *Provider) getJSON(ctx context.Context, path string, query url.Values, out any) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	if p.currentToken() == "" {
		if err := p.authorize(ctx); err != nil {
			return err
		}
	}
	err := p.getOnce(ctx, path, query, out)
	if err == nil {
		return nil
	}
	// Re-auth once when the Dashboard rejected the token itself (401).
	// A 403 means the token is valid but the user lacks permission;
	// re-logging in would be pointless and could trip the Dashboard's
	// account lockout on repeated attempts.
	var rejected tokenRejected
	if errors.As(err, &rejected) {
		p.clearToken()
		if authErr := p.authorize(ctx); authErr != nil {
			return authErr
		}
		return p.getOnce(ctx, path, query, out)
	}
	return err
}

func (p *Provider) getOnce(ctx context.Context, path string, query url.Values, out any) error {
	endpoint := *p.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + path
	if len(query) > 0 {
		endpoint.RawQuery = query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return providerErr(providers.ErrorUnavailable, "build request for %s: %v", path, err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.currentToken())
	resp, err := p.client.Do(req)
	if err != nil {
		return classifyTransportErr(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusUnauthorized {
			return tokenRejected{ProviderError: providers.ProviderError{Class: providers.ErrorUnauthorized, Message: fmt.Sprintf("%s returned HTTP 401", path)}}
		}
		return classifyStatus(resp.StatusCode, path)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return providerErr(providers.ErrorMalformedResponse, "decode %s response: %v", path, err)
	}
	return nil
}

func getPaged[T any](ctx context.Context, p *Provider, path string) ([]T, error) {
	var all []T
	offset := 0
	for page := 0; page < maxPages; page++ {
		query := url.Values{}
		query.Set("limit", strconv.Itoa(p.pageSize))
		query.Set("offset", strconv.Itoa(offset))
		var items []T
		if err := p.getJSON(ctx, path, query, &items); err != nil {
			return nil, err
		}
		all = append(all, items...)
		if len(items) < p.pageSize {
			return all, nil
		}
		offset += len(items)
	}
	return nil, providerErr(providers.ErrorUnavailable, "%s exceeded %d pages of %d items", path, maxPages, p.pageSize)
}

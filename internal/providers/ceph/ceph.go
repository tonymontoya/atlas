package ceph

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/tonymontoya/ceph-atlas/internal/apperr"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
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
	if cfg.BaseURL == "" {
		return nil, errors.New("invalid ceph provider config: BaseURL is required")
	}
	// One parse-and-normalize step: the value actually dialed is the
	// trailing-slash-trimmed one, so it is the one validated here.
	baseURL, err := url.Parse(strings.TrimRight(cfg.BaseURL, "/"))
	if err != nil || baseURL.Scheme == "" || baseURL.Host == "" {
		return nil, fmt.Errorf("invalid ceph provider config: BaseURL %q must be an absolute URL with a scheme", cfg.BaseURL)
	}
	if cfg.Username == "" {
		return nil, errors.New("invalid ceph provider config: Username is required")
	}
	if cfg.Password == "" {
		return nil, errors.New("invalid ceph provider config: Password is required")
	}
	if cfg.ClusterName == "" {
		cfg.ClusterName = defaultName
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

func providerErr(class apperr.Class, format string, args ...any) error {
	return apperr.Error{Class: class, Message: fmt.Sprintf(format, args...)}
}

func ctxErr(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return providerErr(apperr.Timeout, "context done before request: %v", err)
	}
	return nil
}

func classifyTransportErr(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return providerErr(apperr.Timeout, "request context ended: %v", err)
	}
	return providerErr(apperr.Unavailable, "dashboard request failed: %v", err)
}

func classifyStatus(status int, what string) error {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return providerErr(apperr.Unauthorized, "%s returned HTTP %d", what, status)
	case status == http.StatusNotFound:
		return providerErr(apperr.NotFound, "%s returned HTTP 404", what)
	case status >= 500:
		return providerErr(apperr.Unavailable, "%s returned HTTP %d", what, status)
	default:
		return providerErr(apperr.Unavailable, "%s returned unexpected HTTP %d", what, status)
	}
}

type loginResponse struct {
	Token string `json:"token"`
}

// tokenRejected marks an apperr.Error caused by HTTP 401 specifically, so
// getJSON can distinguish "re-auth might help" (401) from "permission
// denied" (403, same Unauthorized class, no retry). It cannot embed
// apperr.Error: the embedded field would be named Error and shadow the
// promoted Error method, so it wraps instead.
type tokenRejected struct {
	err apperr.Error
}

func (t tokenRejected) Error() string { return t.err.Error() }

func (t tokenRejected) Unwrap() error { return t.err }

func (p *Provider) authorize(ctx context.Context) error {
	if err := ctxErr(ctx); err != nil {
		return err
	}
	body, err := json.Marshal(map[string]string{
		"username": p.username,
		"password": p.password,
	})
	if err != nil {
		return providerErr(apperr.Unavailable, "encode login request: %v", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL.String()+"/api/auth", bytes.NewReader(body))
	if err != nil {
		return providerErr(apperr.Unavailable, "build login request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return classifyTransportErr(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusBadRequest {
		return providerErr(apperr.Unauthorized, "dashboard login rejected with HTTP %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusCreated {
		return classifyStatus(resp.StatusCode, "dashboard login")
	}
	var login loginResponse
	if err := json.NewDecoder(resp.Body).Decode(&login); err != nil {
		return providerErr(apperr.MalformedResponse, "decode login response: %v", err)
	}
	if login.Token == "" {
		return providerErr(apperr.MalformedResponse, "login response carried no token")
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
		return providerErr(apperr.Unavailable, "build request for %s: %v", path, err)
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
			return tokenRejected{err: apperr.Error{Class: apperr.Unauthorized, Message: fmt.Sprintf("%s returned HTTP 401", path)}}
		}
		return classifyStatus(resp.StatusCode, path)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return providerErr(apperr.MalformedResponse, "decode %s response: %v", path, err)
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
	return nil, providerErr(apperr.Unavailable, "%s exceeded %d pages of %d items", path, maxPages, p.pageSize)
}

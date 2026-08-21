// Package prometheus implements the observability provider contract
// against a real Prometheus HTTP API. Per ADR-0027, third-party
// observability integrations are pull: one environment-level Prometheus
// per Atlas deployment, reached with environment-configured credentials,
// never per-cluster configuration.
package prometheus

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tonymontoya/ceph-atlas/internal/apperr"
	"github.com/tonymontoya/ceph-atlas/internal/observability"
)

const (
	requestTimeout = 30 * time.Second
	alertsPath     = "/api/v1/alerts"
	sourceName     = "prometheus"
)

type doer interface {
	Do(r *http.Request) (*http.Response, error)
}

type Config struct {
	BaseURL            string
	BearerToken        string
	InsecureSkipVerify bool
	HTTPClient         *http.Client
}

type Provider struct {
	baseURL     *url.URL
	bearerToken string
	client      doer
}

func New(cfg Config) (*Provider, error) {
	if cfg.BaseURL == "" {
		return nil, errors.New("invalid prometheus provider config: BaseURL is required")
	}
	// One parse-and-normalize step: the value actually dialed is the
	// trailing-slash-trimmed one, so it is the one validated here.
	baseURL, err := url.Parse(strings.TrimRight(cfg.BaseURL, "/"))
	if err != nil || baseURL.Scheme == "" || baseURL.Host == "" {
		return nil, fmt.Errorf("invalid prometheus provider config: BaseURL %q must be an absolute URL with a scheme", cfg.BaseURL)
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
		bearerToken: cfg.BearerToken,
		client:      client,
	}, nil
}

// alert is one entry of the Prometheus /api/v1/alerts payload. The
// alert name and severity live inside the label set (alertname,
// severity); Atlas maps them onto the normalized observability.Alert.
type alert struct {
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	State       string            `json:"state"`
	ActiveAt    time.Time         `json:"activeAt"`
}

type alertsResponse struct {
	Status string `json:"status"`
	Data   struct {
		Alerts []alert `json:"alerts"`
	} `json:"data"`
}

func (p *Provider) CurrentAlerts(ctx context.Context) ([]observability.Alert, error) {
	if err := ctxErr(ctx); err != nil {
		return nil, err
	}
	endpoint := *p.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + alertsPath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, providerErr(apperr.Unavailable, "build request for %s: %v", alertsPath, err)
	}
	req.Header.Set("Accept", "application/json")
	if p.bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+p.bearerToken)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, classifyTransportErr(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, classifyStatus(resp.StatusCode, alertsPath)
	}
	var payload alertsResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, providerErr(apperr.MalformedResponse, "decode %s response: %v", alertsPath, err)
	}
	if payload.Status != "success" {
		return nil, providerErr(apperr.MalformedResponse, "%s response status = %q, want success", alertsPath, payload.Status)
	}
	alerts := make([]observability.Alert, 0, len(payload.Data.Alerts))
	for _, raw := range payload.Data.Alerts {
		state, err := mapState(raw.State, raw.Labels["alertname"])
		if err != nil {
			return nil, err
		}
		alerts = append(alerts, observability.Alert{
			Name:        raw.Labels["alertname"],
			Severity:    raw.Labels["severity"],
			Labels:      labelsWithout(raw.Labels, "alertname", "severity"),
			Annotations: raw.Annotations,
			StartedAt:   raw.ActiveAt,
			State:       state,
			Source:      sourceName,
		})
	}
	return alerts, nil
}

// mapState accepts the states /api/v1/alerts actually reports. Atlas
// resolves alerts, so inactive never appears; anything beyond firing
// and pending means the payload is not Prometheus-shaped.
func mapState(state, alertName string) (observability.AlertState, error) {
	switch state {
	case string(observability.AlertStateFiring):
		return observability.AlertStateFiring, nil
	case string(observability.AlertStatePending):
		return observability.AlertStatePending, nil
	default:
		return "", providerErr(apperr.MalformedResponse, "alert %q has unknown state %q (want firing or pending)", alertName, state)
	}
}

func labelsWithout(labels map[string]string, excluded ...string) map[string]string {
	if labels == nil {
		return nil
	}
	filtered := make(map[string]string, len(labels))
	for key, value := range labels {
		drop := false
		for _, exclude := range excluded {
			if key == exclude {
				drop = true
				break
			}
		}
		if !drop {
			filtered[key] = value
		}
	}
	return filtered
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
	return providerErr(apperr.Unavailable, "prometheus request failed: %v", err)
}

func classifyStatus(status int, what string) error {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return providerErr(apperr.Unauthorized, "%s returned HTTP %d", what, status)
	case status >= 500:
		return providerErr(apperr.Unavailable, "%s returned HTTP %d", what, status)
	default:
		return providerErr(apperr.Unavailable, "%s returned unexpected HTTP %d", what, status)
	}
}

package prometheus

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tonymontoya/ceph-atlas/internal/apperr"
	"github.com/tonymontoya/ceph-atlas/internal/providers/prometheus/promtest"
)

func newTestProvider(t *testing.T, mode promtest.Mode) *Provider {
	t.Helper()
	server := promtest.New(t, mode)
	provider, err := New(Config{
		BaseURL:     server.URL(),
		BearerToken: promtest.Token,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	return provider
}

func TestNewRejectsInvalidConfig(t *testing.T) {
	cases := []struct {
		name        string
		baseURL     string
		wantWording string
	}{
		{name: "empty", baseURL: "", wantWording: "BaseURL is required"},
		{name: "schemeless", baseURL: "prometheus.example.invalid:9090", wantWording: "absolute URL"},
		{name: "path-only", baseURL: "prometheus", wantWording: "absolute URL"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(Config{BaseURL: tc.baseURL})
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.wantWording) {
				t.Fatalf("error %q does not contain %q", err, tc.wantWording)
			}
		})
	}
}

func TestCurrentAlertsMapsPrometheusPayload(t *testing.T) {
	provider := newTestProvider(t, promtest.ModeSuccess)

	alerts, err := provider.CurrentAlerts(context.Background())
	if err != nil {
		t.Fatalf("CurrentAlerts returned error: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("CurrentAlerts returned %d alerts, want 1", len(alerts))
	}
	alert := alerts[0]
	if alert.Name != promtest.AlertName {
		t.Fatalf("alert name = %q, want %q", alert.Name, promtest.AlertName)
	}
	if alert.Severity != promtest.Severity {
		t.Fatalf("alert severity = %q, want %q", alert.Severity, promtest.Severity)
	}
	if alert.Labels["cluster"] != promtest.ClusterLabel || alert.Labels["osd"] != promtest.OSDLabel {
		t.Fatalf("alert labels = %v, want the cluster and osd labels", alert.Labels)
	}
	if _, ok := alert.Labels["alertname"]; ok {
		t.Fatalf("alert labels = %v, want alertname folded into Name", alert.Labels)
	}
	if _, ok := alert.Labels["severity"]; ok {
		t.Fatalf("alert labels = %v, want severity folded into Severity", alert.Labels)
	}
	if alert.Annotations["summary"] != "OSD 1 is down (promtest)" {
		t.Fatalf("alert annotations = %v, want the promtest summary", alert.Annotations)
	}
	wantActiveAt, err := time.Parse(time.RFC3339, promtest.ActiveAt)
	if err != nil {
		t.Fatalf("parse promtest ActiveAt: %v", err)
	}
	if !alert.StartedAt.Equal(wantActiveAt) {
		t.Fatalf("alert StartedAt = %v, want %v", alert.StartedAt, wantActiveAt)
	}
	if alert.State != "firing" {
		t.Fatalf("alert state = %q, want firing", alert.State)
	}
	if alert.Source != "prometheus" {
		t.Fatalf("alert source = %q, want prometheus", alert.Source)
	}
}

func TestCurrentAlertsSendsConfiguredBearerToken(t *testing.T) {
	server := promtest.New(t, promtest.ModeSuccess)
	provider, err := New(Config{BaseURL: server.URL(), BearerToken: promtest.Token})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if _, err := provider.CurrentAlerts(context.Background()); err != nil {
		t.Fatalf("CurrentAlerts returned error: %v", err)
	}
	authorizations := server.Authorizations()
	if len(authorizations) != 1 || authorizations[0] != "Bearer "+promtest.Token {
		t.Fatalf("Authorization headers = %v, want one bearer token", authorizations)
	}
}

func TestCurrentAlertsOmitsAuthorizationWithoutToken(t *testing.T) {
	var receivedAuthorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuthorization = r.Header.Get("Authorization")
		promtest.WriteJSON(w, http.StatusOK, map[string]any{
			"status": "success",
			"data":   map[string]any{"alerts": []any{}},
		})
	}))
	t.Cleanup(server.Close)
	provider, err := New(Config{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	if _, err := provider.CurrentAlerts(context.Background()); err != nil {
		t.Fatalf("CurrentAlerts returned error: %v", err)
	}
	if receivedAuthorization != "" {
		t.Fatalf("Authorization header = %q, want none without a configured token", receivedAuthorization)
	}
}

func TestCurrentAlertsRejectsUnknownAlertState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		promtest.WriteJSON(w, http.StatusOK, map[string]any{
			"status": "success",
			"data": map[string]any{
				"alerts": []any{
					map[string]any{
						"labels":   map[string]any{"alertname": "CephOSDDown", "severity": "warning"},
						"state":    "sideways",
						"activeAt": promtest.ActiveAt,
					},
				},
			},
		})
	}))
	t.Cleanup(server.Close)
	provider, err := New(Config{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	_, err = provider.CurrentAlerts(context.Background())
	assertErrorClass(t, err, apperr.MalformedResponse)
	if got := err.Error(); !strings.Contains(got, "sideways") {
		t.Fatalf("error %q does not name the rejected state", got)
	}
}

func TestCurrentAlertsRejectsErrorStatusPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		promtest.WriteJSON(w, http.StatusOK, map[string]any{
			"status": "error",
			"data":   map[string]any{"alerts": []any{}},
		})
	}))
	t.Cleanup(server.Close)
	provider, err := New(Config{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	_, err = provider.CurrentAlerts(context.Background())
	assertErrorClass(t, err, apperr.MalformedResponse)
}

func TestCurrentAlertsTreatsForbiddenAsUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	t.Cleanup(server.Close)
	provider, err := New(Config{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	_, err = provider.CurrentAlerts(context.Background())
	assertErrorClass(t, err, apperr.Unauthorized)
}

func assertErrorClass(t *testing.T, err error, want apperr.Class) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with class %q, got nil", want)
	}
	var appErr apperr.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("error type = %T, want apperr.Error", err)
	}
	if appErr.Class != want {
		t.Fatalf("error class = %q, want %q (message: %s)", appErr.Class, want, appErr.Message)
	}
}

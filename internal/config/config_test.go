package config

import (
	"strings"
	"testing"
	"time"
)

// removedPullPathEnv are the deleted control-plane pull-path variables
// (ADR-0025). Load rejects them when set; clearing them keeps these tests
// hermetic against a stale developer shell.
var removedPullPathEnv = []string{
	"ATLAS_PROVIDER_MODE",
	"ATLAS_CEPH_DASHBOARD_URL",
	"ATLAS_CEPH_DASHBOARD_USER",
	"ATLAS_CEPH_DASHBOARD_PASSWORD",
	"ATLAS_CEPH_CLUSTER_NAME",
	"ATLAS_CEPH_DASHBOARD_INSECURE_TLS",
}

func clearAtlasEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"ATLAS_HTTP_ADDR",
		"ATLAS_DATABASE_URL",
		"ATLAS_FAKE_SCENARIO",
		"ATLAS_FAKE_ALERT_SCENARIO",
		"ATLAS_FAKE_AGENT_SCENARIO",
		"ATLAS_READ_SOURCE",
		"ATLAS_AGENT_MODE",
		"ATLAS_ALERT_SOURCE",
		"ATLAS_ALERT_EVAL_INTERVAL",
		"ATLAS_OIDC_ISSUER",
		"ATLAS_OIDC_AUDIENCE",
		"ATLAS_OIDC_JWKS_URL",
		"ATLAS_PROMETHEUS_URL",
		"ATLAS_PROMETHEUS_BEARER_TOKEN",
		"ATLAS_PROMETHEUS_INSECURE_TLS",
		"ATLAS_ENROLLMENT_CA_CERT_PATH",
		"ATLAS_ENROLLMENT_CA_KEY_PATH",
	} {
		t.Setenv(key, "")
	}
	for _, key := range removedPullPathEnv {
		t.Setenv(key, "")
	}
}

func TestLoadDefaults(t *testing.T) {
	clearAtlasEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Fatalf("HTTPAddr = %q, want :8080", cfg.HTTPAddr)
	}
	if cfg.DatabaseURL == "" {
		t.Fatal("DatabaseURL is empty")
	}
	if cfg.FakeScenario != "reef-healthy-baremetal" {
		t.Fatalf("FakeScenario = %q, want reef-healthy-baremetal", cfg.FakeScenario)
	}
	if cfg.FakeAlertScenario != "osd-down-alert" {
		t.Fatalf("FakeAlertScenario = %q, want osd-down-alert", cfg.FakeAlertScenario)
	}
	if cfg.FakeAgentScenario != "" {
		t.Fatalf("FakeAgentScenario = %q, want the happy-path default", cfg.FakeAgentScenario)
	}
	if cfg.ReadSource != ReadSourceProvider {
		t.Fatalf("ReadSource = %q, want provider", cfg.ReadSource)
	}
	if cfg.AgentMode != AgentModeDisabled {
		t.Fatalf("AgentMode = %q, want disabled", cfg.AgentMode)
	}
	if cfg.AlertSource != AlertSourceFake {
		t.Fatalf("AlertSource = %q, want fake", cfg.AlertSource)
	}
	if cfg.AlertEvalInterval != 0 {
		t.Fatalf("AlertEvalInterval = %s, want the one-shot zero default", cfg.AlertEvalInterval)
	}
	if cfg.PrometheusURL != "" || cfg.PrometheusBearerToken != "" || cfg.PrometheusInsecureTLS {
		t.Fatalf("prometheus config = %q/%q/%t, want all empty by default", cfg.PrometheusURL, cfg.PrometheusBearerToken, cfg.PrometheusInsecureTLS)
	}
}

func TestLoadCanonicalizesEmptyAliases(t *testing.T) {
	clearAtlasEnv(t)
	t.Setenv("ATLAS_READ_SOURCE", "")
	t.Setenv("ATLAS_AGENT_MODE", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.ReadSource != ReadSourceProvider {
		t.Fatalf("ReadSource = %q, want provider", cfg.ReadSource)
	}
	if cfg.AgentMode != AgentModeDisabled {
		t.Fatalf("AgentMode = %q, want disabled", cfg.AgentMode)
	}
}

func TestLoadRejectsUnknownModes(t *testing.T) {
	cases := []struct {
		name string
		key  string
	}{
		{name: "read source", key: "ATLAS_READ_SOURCE"},
		{name: "agent mode", key: "ATLAS_AGENT_MODE"},
		{name: "alert source", key: "ATLAS_ALERT_SOURCE"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearAtlasEnv(t)
			t.Setenv(tc.key, "bogus")

			_, err := Load()
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.key) {
				t.Fatalf("error %q does not name %s", err, tc.key)
			}
		})
	}
}

func TestLoadRejectsPartialOIDCTrio(t *testing.T) {
	clearAtlasEnv(t)
	t.Setenv("ATLAS_OIDC_ISSUER", "https://atlas-dev-issuer.local")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{"ATLAS_OIDC_ISSUER", "ATLAS_OIDC_AUDIENCE", "ATLAS_OIDC_JWKS_URL"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not name %s", err, want)
		}
	}
}

func TestLoadRejectsPartialEnrollmentCAPair(t *testing.T) {
	clearAtlasEnv(t)
	t.Setenv("ATLAS_ENROLLMENT_CA_CERT_PATH", "/etc/atlas/ca.crt")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{"ATLAS_ENROLLMENT_CA_CERT_PATH", "ATLAS_ENROLLMENT_CA_KEY_PATH"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not name %s", err, want)
		}
	}
}

func TestLoadAcceptsFullEnrollmentCAPair(t *testing.T) {
	clearAtlasEnv(t)
	t.Setenv("ATLAS_ENROLLMENT_CA_CERT_PATH", "/etc/atlas/ca.crt")
	t.Setenv("ATLAS_ENROLLMENT_CA_KEY_PATH", "/etc/atlas/ca.key")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.EnrollmentCACertPath != "/etc/atlas/ca.crt" || cfg.EnrollmentCAKeyPath != "/etc/atlas/ca.key" {
		t.Fatalf("enrollment CA paths = %q/%q", cfg.EnrollmentCACertPath, cfg.EnrollmentCAKeyPath)
	}
}

func TestLoadRejectsPartialAPITLSPair(t *testing.T) {
	clearAtlasEnv(t)
	t.Setenv("ATLAS_API_TLS_CERT_PATH", "/etc/atlas/api.crt")

	_, err := Load()
	if err != nil {
		for _, want := range []string{"ATLAS_API_TLS_CERT_PATH", "ATLAS_API_TLS_KEY_PATH"} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("error %q does not name %s", err, want)
			}
		}
		return
	}
	t.Fatal("expected error")
}

func TestLoadAcceptsFullAPITLSPair(t *testing.T) {
	clearAtlasEnv(t)
	t.Setenv("ATLAS_API_TLS_CERT_PATH", "/etc/atlas/api.crt")
	t.Setenv("ATLAS_API_TLS_KEY_PATH", "/etc/atlas/api.key")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.APITLSCertPath != "/etc/atlas/api.crt" || cfg.APITLSKeyPath != "/etc/atlas/api.key" {
		t.Fatalf("API TLS paths = %q/%q", cfg.APITLSCertPath, cfg.APITLSKeyPath)
	}
	if cfg.HTTPSAddr != "" {
		t.Fatalf("HTTPSAddr = %q, want empty by default (TLS serving replaces the HTTP listener)", cfg.HTTPSAddr)
	}
}

func TestLoadAcceptsHTTPSAddrWithTLSPair(t *testing.T) {
	clearAtlasEnv(t)
	t.Setenv("ATLAS_API_TLS_CERT_PATH", "/etc/atlas/api.crt")
	t.Setenv("ATLAS_API_TLS_KEY_PATH", "/etc/atlas/api.key")
	t.Setenv("ATLAS_HTTPS_ADDR", ":8443")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.HTTPSAddr != ":8443" {
		t.Fatalf("HTTPSAddr = %q, want :8443", cfg.HTTPSAddr)
	}
}

func TestLoadRejectsHTTPSAddrWithoutTLSPair(t *testing.T) {
	clearAtlasEnv(t)
	t.Setenv("ATLAS_HTTPS_ADDR", ":8443")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{"ATLAS_HTTPS_ADDR", "ATLAS_API_TLS_CERT_PATH", "ATLAS_API_TLS_KEY_PATH"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not name %s", err, want)
		}
	}
}

// The control-plane pull path is gone (ADR-0025): every removed variable
// fails fast when set, whatever the value, so a stale environment cannot
// silently fall back to fake seeding.
func TestLoadRejectsRemovedPullPathEnv(t *testing.T) {
	cases := []struct {
		name  string
		key   string
		value string
	}{
		{name: "provider mode ceph", key: "ATLAS_PROVIDER_MODE", value: "ceph"},
		{name: "provider mode fake", key: "ATLAS_PROVIDER_MODE", value: "fake"},
		{name: "dashboard url", key: "ATLAS_CEPH_DASHBOARD_URL", value: "https://mon.example.invalid:8443"},
		{name: "dashboard user", key: "ATLAS_CEPH_DASHBOARD_USER", value: "atlas-reader"},
		{name: "dashboard password", key: "ATLAS_CEPH_DASHBOARD_PASSWORD", value: "secret"},
		{name: "cluster name", key: "ATLAS_CEPH_CLUSTER_NAME", value: "reef-lab"},
		{name: "dashboard insecure tls", key: "ATLAS_CEPH_DASHBOARD_INSECURE_TLS", value: "true"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearAtlasEnv(t)
			t.Setenv(tc.key, tc.value)

			_, err := Load()
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.key) {
				t.Fatalf("error %q does not name %s", err, tc.key)
			}
			if !strings.Contains(err.Error(), "no longer supported") {
				t.Fatalf("error %q does not say the variable is no longer supported", err)
			}
			if !strings.Contains(err.Error(), "enrolled Atlas Agent") {
				t.Fatalf("error %q does not point at the replacement path", err)
			}
		})
	}
}

func TestLoadRejectsInvalidBoolean(t *testing.T) {
	clearAtlasEnv(t)
	t.Setenv("ATLAS_PROMETHEUS_INSECURE_TLS", "tru")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "ATLAS_PROMETHEUS_INSECURE_TLS") {
		t.Fatalf("error %q does not name ATLAS_PROMETHEUS_INSECURE_TLS", err)
	}
}

func TestLoadReadsPrometheusAlertConfiguration(t *testing.T) {
	clearAtlasEnv(t)
	t.Setenv("ATLAS_ALERT_SOURCE", "prometheus")
	t.Setenv("ATLAS_PROMETHEUS_URL", "http://prometheus.example.invalid:9090")
	t.Setenv("ATLAS_PROMETHEUS_BEARER_TOKEN", "secret-token")
	t.Setenv("ATLAS_PROMETHEUS_INSECURE_TLS", "true")
	t.Setenv("ATLAS_ALERT_EVAL_INTERVAL", "45s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.AlertSource != AlertSourcePrometheus {
		t.Fatalf("AlertSource = %q, want prometheus", cfg.AlertSource)
	}
	if cfg.PrometheusURL != "http://prometheus.example.invalid:9090" {
		t.Fatalf("PrometheusURL = %q", cfg.PrometheusURL)
	}
	if cfg.PrometheusBearerToken != "secret-token" {
		t.Fatalf("PrometheusBearerToken = %q", cfg.PrometheusBearerToken)
	}
	if !cfg.PrometheusInsecureTLS {
		t.Fatal("PrometheusInsecureTLS = false, want true")
	}
	if cfg.AlertEvalInterval != 45*time.Second {
		t.Fatalf("AlertEvalInterval = %s, want 45s", cfg.AlertEvalInterval)
	}
}

func TestLoadPrometheusAlertSourceRequiresURL(t *testing.T) {
	clearAtlasEnv(t)
	t.Setenv("ATLAS_ALERT_SOURCE", "prometheus")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "ATLAS_PROMETHEUS_URL") {
		t.Fatalf("error %q does not name ATLAS_PROMETHEUS_URL", err)
	}
}

func TestLoadPrometheusAlertSourceRejectsSchemelessURL(t *testing.T) {
	clearAtlasEnv(t)
	t.Setenv("ATLAS_ALERT_SOURCE", "prometheus")
	t.Setenv("ATLAS_PROMETHEUS_URL", "prometheus.example.invalid:9090")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "ATLAS_PROMETHEUS_URL") {
		t.Fatalf("error %q does not name ATLAS_PROMETHEUS_URL", err)
	}
	if !strings.Contains(err.Error(), "absolute URL") {
		t.Fatalf("error %q does not explain the absolute URL requirement", err)
	}
}

func TestLoadPrometheusAlertSourceAcceptsValidConfiguration(t *testing.T) {
	clearAtlasEnv(t)
	t.Setenv("ATLAS_ALERT_SOURCE", "prometheus")
	t.Setenv("ATLAS_PROMETHEUS_URL", "http://prometheus.example.invalid:9090/")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.AlertSource != AlertSourcePrometheus {
		t.Fatalf("AlertSource = %q, want prometheus", cfg.AlertSource)
	}
}

func TestLoadRejectsInvalidAlertEvalInterval(t *testing.T) {
	cases := []struct {
		name  string
		value string
	}{
		{name: "not a duration", value: "sometimes"},
		{name: "negative", value: "-5m"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearAtlasEnv(t)
			t.Setenv("ATLAS_ALERT_EVAL_INTERVAL", tc.value)

			_, err := Load()
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), "ATLAS_ALERT_EVAL_INTERVAL") {
				t.Fatalf("error %q does not name ATLAS_ALERT_EVAL_INTERVAL", err)
			}
		})
	}
}

func TestLoadJoinsAllFindingsIntoOneError(t *testing.T) {
	clearAtlasEnv(t)
	t.Setenv("ATLAS_PROVIDER_MODE", "ceph")
	t.Setenv("ATLAS_CEPH_DASHBOARD_URL", "https://mon.example.invalid:8443")
	t.Setenv("ATLAS_READ_SOURCE", "bogus")
	t.Setenv("ATLAS_ALERT_SOURCE", "prometheus")
	t.Setenv("ATLAS_ALERT_EVAL_INTERVAL", "never")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{
		"ATLAS_PROVIDER_MODE",
		"ATLAS_CEPH_DASHBOARD_URL",
		"ATLAS_READ_SOURCE",
		"ATLAS_ALERT_SOURCE",
		"ATLAS_PROMETHEUS_URL",
		"ATLAS_ALERT_EVAL_INTERVAL",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("joined error %q does not name %s", err, want)
		}
	}
}

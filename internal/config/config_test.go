package config

import (
	"strings"
	"testing"
	"time"
)

func clearModeEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"ATLAS_HTTP_ADDR",
		"ATLAS_DATABASE_URL",
		"ATLAS_PROVIDER_MODE",
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
		"ATLAS_CEPH_DASHBOARD_URL",
		"ATLAS_CEPH_DASHBOARD_USER",
		"ATLAS_CEPH_DASHBOARD_PASSWORD",
		"ATLAS_CEPH_CLUSTER_NAME",
		"ATLAS_CEPH_DASHBOARD_INSECURE_TLS",
		"ATLAS_PROMETHEUS_URL",
		"ATLAS_PROMETHEUS_BEARER_TOKEN",
		"ATLAS_PROMETHEUS_INSECURE_TLS",
		"ATLAS_ENROLLMENT_CA_CERT_PATH",
		"ATLAS_ENROLLMENT_CA_KEY_PATH",
	} {
		t.Setenv(key, "")
	}
}

func TestLoadDefaultsToFakeProvider(t *testing.T) {
	clearModeEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Fatalf("HTTPAddr = %q, want :8080", cfg.HTTPAddr)
	}
	if cfg.ProviderMode != ProviderModeFake {
		t.Fatalf("ProviderMode = %q, want fake", cfg.ProviderMode)
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
	if cfg.CephDashboardURL != "" || cfg.CephDashboardUser != "" || cfg.CephDashboardPassword != "" || cfg.CephClusterName != "" {
		t.Fatalf("ceph dashboard config = %q/%q/%q/%q, want all empty by default", cfg.CephDashboardURL, cfg.CephDashboardUser, cfg.CephDashboardPassword, cfg.CephClusterName)
	}
	if cfg.CephDashboardInsecureTLS {
		t.Fatal("CephDashboardInsecureTLS = true, want false by default")
	}
}

func TestLoadReadsCephDashboardConfig(t *testing.T) {
	clearModeEnv(t)
	t.Setenv("ATLAS_CEPH_DASHBOARD_URL", "https://mon.example.invalid:8443")
	t.Setenv("ATLAS_CEPH_DASHBOARD_USER", "atlas-reader")
	t.Setenv("ATLAS_CEPH_DASHBOARD_PASSWORD", "secret")
	t.Setenv("ATLAS_CEPH_CLUSTER_NAME", "reef-lab")
	t.Setenv("ATLAS_CEPH_DASHBOARD_INSECURE_TLS", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.CephDashboardURL != "https://mon.example.invalid:8443" {
		t.Fatalf("CephDashboardURL = %q", cfg.CephDashboardURL)
	}
	if cfg.CephDashboardUser != "atlas-reader" {
		t.Fatalf("CephDashboardUser = %q", cfg.CephDashboardUser)
	}
	if cfg.CephDashboardPassword != "secret" {
		t.Fatalf("CephDashboardPassword = %q", cfg.CephDashboardPassword)
	}
	if cfg.CephClusterName != "reef-lab" {
		t.Fatalf("CephClusterName = %q", cfg.CephClusterName)
	}
	if !cfg.CephDashboardInsecureTLS {
		t.Fatal("CephDashboardInsecureTLS = false, want true")
	}
}

func TestLoadCanonicalizesEmptyAliases(t *testing.T) {
	clearModeEnv(t)
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
		{name: "provider mode", key: "ATLAS_PROVIDER_MODE"},
		{name: "read source", key: "ATLAS_READ_SOURCE"},
		{name: "agent mode", key: "ATLAS_AGENT_MODE"},
		{name: "alert source", key: "ATLAS_ALERT_SOURCE"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clearModeEnv(t)
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
	clearModeEnv(t)
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
	clearModeEnv(t)
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
	clearModeEnv(t)
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

func TestLoadCephModeRequiresDashboardConfiguration(t *testing.T) {
	clearModeEnv(t)
	t.Setenv("ATLAS_PROVIDER_MODE", "ceph")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{"ATLAS_CEPH_DASHBOARD_URL", "ATLAS_CEPH_DASHBOARD_USER", "ATLAS_CEPH_DASHBOARD_PASSWORD"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not name %s", err, want)
		}
	}
}

func TestLoadCephModeRejectsSchemelessDashboardURL(t *testing.T) {
	clearModeEnv(t)
	t.Setenv("ATLAS_PROVIDER_MODE", "ceph")
	t.Setenv("ATLAS_CEPH_DASHBOARD_URL", "mon.example.invalid:8443")
	t.Setenv("ATLAS_CEPH_DASHBOARD_USER", "atlas-reader")
	t.Setenv("ATLAS_CEPH_DASHBOARD_PASSWORD", "secret")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "ATLAS_CEPH_DASHBOARD_URL") {
		t.Fatalf("error %q does not name ATLAS_CEPH_DASHBOARD_URL", err)
	}
	if !strings.Contains(err.Error(), "absolute URL") {
		t.Fatalf("error %q does not explain the absolute URL requirement", err)
	}
}

func TestLoadCephModeAcceptsValidDashboardConfiguration(t *testing.T) {
	clearModeEnv(t)
	t.Setenv("ATLAS_PROVIDER_MODE", "ceph")
	t.Setenv("ATLAS_CEPH_DASHBOARD_URL", "https://mon.example.invalid:8443/")
	t.Setenv("ATLAS_CEPH_DASHBOARD_USER", "atlas-reader")
	t.Setenv("ATLAS_CEPH_DASHBOARD_PASSWORD", "secret")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.ProviderMode != ProviderModeCeph {
		t.Fatalf("ProviderMode = %q, want ceph", cfg.ProviderMode)
	}
}

func TestLoadRejectsInvalidBoolean(t *testing.T) {
	clearModeEnv(t)
	t.Setenv("ATLAS_CEPH_DASHBOARD_INSECURE_TLS", "tru")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "ATLAS_CEPH_DASHBOARD_INSECURE_TLS") {
		t.Fatalf("error %q does not name ATLAS_CEPH_DASHBOARD_INSECURE_TLS", err)
	}
}

func TestLoadReadsPrometheusAlertConfiguration(t *testing.T) {
	clearModeEnv(t)
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
	clearModeEnv(t)
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
	clearModeEnv(t)
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
	clearModeEnv(t)
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
			clearModeEnv(t)
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
	clearModeEnv(t)
	t.Setenv("ATLAS_PROVIDER_MODE", "ceph")
	t.Setenv("ATLAS_CEPH_DASHBOARD_URL", "mon.example.invalid:8443")
	t.Setenv("ATLAS_READ_SOURCE", "bogus")
	t.Setenv("ATLAS_CEPH_DASHBOARD_INSECURE_TLS", "tru")
	t.Setenv("ATLAS_ALERT_SOURCE", "prometheus")
	t.Setenv("ATLAS_ALERT_EVAL_INTERVAL", "never")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{
		"ATLAS_READ_SOURCE",
		"ATLAS_CEPH_DASHBOARD_URL",
		"ATLAS_CEPH_DASHBOARD_USER",
		"ATLAS_CEPH_DASHBOARD_PASSWORD",
		"ATLAS_CEPH_DASHBOARD_INSECURE_TLS",
		"ATLAS_ALERT_SOURCE",
		"ATLAS_PROMETHEUS_URL",
		"ATLAS_ALERT_EVAL_INTERVAL",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("joined error %q does not name %s", err, want)
		}
	}
}

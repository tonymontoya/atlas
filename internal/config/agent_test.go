package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func clearAgentEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"ATLAS_AGENT_ATLAS_URL",
		"ATLAS_AGENT_ATLAS_CA_PATH",
		"ATLAS_AGENT_ATLAS_INSECURE_TLS",
		"ATLAS_AGENT_ENROLLMENT_CREDENTIAL",
		"ATLAS_AGENT_ENROLLMENT_CREDENTIAL_FILE",
		"ATLAS_AGENT_STATE_DIR",
		"ATLAS_AGENT_COLLECT_INTERVAL",
		"ATLAS_AGENT_RETRY_INITIAL",
		"ATLAS_AGENT_RETRY_MAX",
		"ATLAS_AGENT_DASHBOARD_URL",
		"ATLAS_AGENT_DASHBOARD_USER",
		"ATLAS_AGENT_DASHBOARD_PASSWORD",
		"ATLAS_AGENT_DASHBOARD_CLUSTER_NAME",
		"ATLAS_AGENT_DASHBOARD_INSECURE_TLS",
	} {
		t.Setenv(key, "")
	}
}

func setAgentEnv(t *testing.T, values map[string]string) {
	t.Helper()
	clearAgentEnv(t)
	for key, value := range values {
		t.Setenv(key, value)
	}
}

func validAgentEnv() map[string]string {
	return map[string]string{
		"ATLAS_AGENT_ATLAS_URL":          "https://atlas.example.invalid",
		"ATLAS_AGENT_DASHBOARD_URL":      "https://mon.example.invalid:8443",
		"ATLAS_AGENT_DASHBOARD_USER":     "atlas-reader",
		"ATLAS_AGENT_DASHBOARD_PASSWORD": "reader-password",
	}
}

func TestLoadAgentRejectsMissingRequiredConfiguration(t *testing.T) {
	clearAgentEnv(t)

	_, err := LoadAgent()
	if err == nil {
		t.Fatal("LoadAgent without any configuration returned no error")
	}
	for _, want := range []string{
		"ATLAS_AGENT_ATLAS_URL is required",
		"ATLAS_AGENT_DASHBOARD_URL is required",
		"ATLAS_AGENT_DASHBOARD_USER is required",
		"ATLAS_AGENT_DASHBOARD_PASSWORD is required",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not mention %q", err, want)
		}
	}
}

func TestLoadAgentDefaults(t *testing.T) {
	setAgentEnv(t, validAgentEnv())

	cfg, err := LoadAgent()
	if err != nil {
		t.Fatalf("LoadAgent returned error: %v", err)
	}
	if cfg.AtlasURL != "https://atlas.example.invalid" {
		t.Fatalf("AtlasURL = %q", cfg.AtlasURL)
	}
	if cfg.StateDir != "atlas-agent-state" {
		t.Fatalf("StateDir default = %q, want atlas-agent-state", cfg.StateDir)
	}
	if cfg.CollectInterval != 60*time.Second {
		t.Fatalf("CollectInterval default = %s, want 60s", cfg.CollectInterval)
	}
	if cfg.RetryInitial != time.Second {
		t.Fatalf("RetryInitial default = %s, want 1s", cfg.RetryInitial)
	}
	if cfg.RetryMax != 30*time.Second {
		t.Fatalf("RetryMax default = %s, want 30s", cfg.RetryMax)
	}
	if cfg.EnrollmentCredential != "" {
		t.Fatalf("EnrollmentCredential default = %q, want empty (only needed to enroll)", cfg.EnrollmentCredential)
	}
	if cfg.DashboardClusterName != "" {
		t.Fatalf("DashboardClusterName default = %q, want empty so the provider applies its own default", cfg.DashboardClusterName)
	}
}

func TestLoadAgentRejectsPlainHTTPAtlasURL(t *testing.T) {
	env := validAgentEnv()
	env["ATLAS_AGENT_ATLAS_URL"] = "http://atlas.example.invalid"
	setAgentEnv(t, env)

	_, err := LoadAgent()
	if err == nil || !strings.Contains(err.Error(), "must use https") {
		t.Fatalf("error = %v, want https requirement for ATLAS_AGENT_ATLAS_URL", err)
	}
}

func TestLoadAgentRejectsSchemelessAtlasURL(t *testing.T) {
	env := validAgentEnv()
	env["ATLAS_AGENT_ATLAS_URL"] = "atlas.example.invalid"
	setAgentEnv(t, env)

	_, err := LoadAgent()
	if err == nil || !strings.Contains(err.Error(), "absolute URL") {
		t.Fatalf("error = %v, want absolute URL requirement", err)
	}
}

func TestLoadAgentRejectsSchemelessDashboardURL(t *testing.T) {
	env := validAgentEnv()
	env["ATLAS_AGENT_DASHBOARD_URL"] = "mon.example.invalid:8443"
	setAgentEnv(t, env)

	_, err := LoadAgent()
	if err == nil || !strings.Contains(err.Error(), "absolute URL") {
		t.Fatalf("error = %v, want absolute URL requirement", err)
	}
}

func TestLoadAgentRejectsInvalidValues(t *testing.T) {
	cases := []struct {
		name  string
		key   string
		value string
		want  string
	}{
		{name: "bad interval", key: "ATLAS_AGENT_COLLECT_INTERVAL", value: "nope", want: "invalid duration for ATLAS_AGENT_COLLECT_INTERVAL"},
		{name: "zero interval", key: "ATLAS_AGENT_COLLECT_INTERVAL", value: "0s", want: "ATLAS_AGENT_COLLECT_INTERVAL must be positive"},
		{name: "zero retry initial", key: "ATLAS_AGENT_RETRY_INITIAL", value: "0s", want: "ATLAS_AGENT_RETRY_INITIAL must be positive"},
		{name: "zero retry max", key: "ATLAS_AGENT_RETRY_MAX", value: "0s", want: "ATLAS_AGENT_RETRY_MAX must be positive"},
		{name: "bad bool", key: "ATLAS_AGENT_ATLAS_INSECURE_TLS", value: "yes", want: "invalid boolean for ATLAS_AGENT_ATLAS_INSECURE_TLS"},
		{name: "bad dashboard bool", key: "ATLAS_AGENT_DASHBOARD_INSECURE_TLS", value: "maybe", want: "invalid boolean for ATLAS_AGENT_DASHBOARD_INSECURE_TLS"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := validAgentEnv()
			env[tc.key] = tc.value
			setAgentEnv(t, env)

			_, err := LoadAgent()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestLoadAgentRejectsRetryMaxBelowInitial(t *testing.T) {
	env := validAgentEnv()
	env["ATLAS_AGENT_RETRY_INITIAL"] = "10s"
	env["ATLAS_AGENT_RETRY_MAX"] = "5s"
	setAgentEnv(t, env)

	_, err := LoadAgent()
	if err == nil || !strings.Contains(err.Error(), "ATLAS_AGENT_RETRY_MAX must be at least ATLAS_AGENT_RETRY_INITIAL") {
		t.Fatalf("error = %v, want retry max floor", err)
	}
}

func TestLoadAgentAcceptsExplicitValues(t *testing.T) {
	env := validAgentEnv()
	env["ATLAS_AGENT_STATE_DIR"] = "/var/lib/atlas-agent"
	env["ATLAS_AGENT_COLLECT_INTERVAL"] = "5m"
	env["ATLAS_AGENT_RETRY_INITIAL"] = "2s"
	env["ATLAS_AGENT_RETRY_MAX"] = "1m"
	env["ATLAS_AGENT_ENROLLMENT_CREDENTIAL"] = "atl_enroll_token"
	env["ATLAS_AGENT_ATLAS_CA_PATH"] = "/etc/atlas-agent/atlas-ca.pem"
	env["ATLAS_AGENT_DASHBOARD_CLUSTER_NAME"] = "lab-ceph"
	env["ATLAS_AGENT_DASHBOARD_INSECURE_TLS"] = "true"
	setAgentEnv(t, env)

	cfg, err := LoadAgent()
	if err != nil {
		t.Fatalf("LoadAgent returned error: %v", err)
	}
	if cfg.StateDir != "/var/lib/atlas-agent" {
		t.Fatalf("StateDir = %q", cfg.StateDir)
	}
	if cfg.CollectInterval != 5*time.Minute {
		t.Fatalf("CollectInterval = %s", cfg.CollectInterval)
	}
	if cfg.RetryInitial != 2*time.Second || cfg.RetryMax != time.Minute {
		t.Fatalf("retry = %s/%s, want 2s/1m", cfg.RetryInitial, cfg.RetryMax)
	}
	if cfg.EnrollmentCredential != "atl_enroll_token" {
		t.Fatalf("EnrollmentCredential = %q", cfg.EnrollmentCredential)
	}
	if cfg.AtlasRootCAPath != "/etc/atlas-agent/atlas-ca.pem" {
		t.Fatalf("AtlasRootCAPath = %q", cfg.AtlasRootCAPath)
	}
	if cfg.DashboardClusterName != "lab-ceph" {
		t.Fatalf("DashboardClusterName = %q", cfg.DashboardClusterName)
	}
	if !cfg.DashboardInsecureTLS {
		t.Fatal("DashboardInsecureTLS = false, want true")
	}
}

func TestLoadAgentReadsCredentialFile(t *testing.T) {
	credentialPath := filepath.Join(t.TempDir(), "enrollment-credential")
	if err := os.WriteFile(credentialPath, []byte("atl_enroll_from_file\n"), 0o600); err != nil {
		t.Fatalf("write credential file: %v", err)
	}
	env := validAgentEnv()
	env["ATLAS_AGENT_ENROLLMENT_CREDENTIAL_FILE"] = credentialPath
	setAgentEnv(t, env)

	cfg, err := LoadAgent()
	if err != nil {
		t.Fatalf("LoadAgent returned error: %v", err)
	}
	if cfg.EnrollmentCredential != "atl_enroll_from_file" {
		t.Fatalf("EnrollmentCredential = %q, want the trimmed file contents", cfg.EnrollmentCredential)
	}
}

func TestLoadAgentRejectsCredentialFileWithInlineCredential(t *testing.T) {
	credentialPath := filepath.Join(t.TempDir(), "enrollment-credential")
	if err := os.WriteFile(credentialPath, []byte("atl_enroll_from_file"), 0o600); err != nil {
		t.Fatalf("write credential file: %v", err)
	}
	env := validAgentEnv()
	env["ATLAS_AGENT_ENROLLMENT_CREDENTIAL"] = "atl_enroll_inline"
	env["ATLAS_AGENT_ENROLLMENT_CREDENTIAL_FILE"] = credentialPath
	setAgentEnv(t, env)

	_, err := LoadAgent()
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{"ATLAS_AGENT_ENROLLMENT_CREDENTIAL", "ATLAS_AGENT_ENROLLMENT_CREDENTIAL_FILE"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not name %s", err, want)
		}
	}
}

func TestLoadAgentRejectsMissingCredentialFile(t *testing.T) {
	env := validAgentEnv()
	env["ATLAS_AGENT_ENROLLMENT_CREDENTIAL_FILE"] = filepath.Join(t.TempDir(), "absent")
	setAgentEnv(t, env)

	_, err := LoadAgent()
	if err == nil || !strings.Contains(err.Error(), "ATLAS_AGENT_ENROLLMENT_CREDENTIAL_FILE") {
		t.Fatalf("error = %v, want a missing-credential-file error naming the variable", err)
	}
}

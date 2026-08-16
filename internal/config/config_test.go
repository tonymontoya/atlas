package config

import "testing"

func TestLoadDefaultsToFakeProvider(t *testing.T) {
	t.Setenv("ATLAS_HTTP_ADDR", "")
	t.Setenv("ATLAS_DATABASE_URL", "")
	t.Setenv("ATLAS_PROVIDER_MODE", "")
	t.Setenv("ATLAS_FAKE_SCENARIO", "")
	t.Setenv("ATLAS_FAKE_AGENT_SCENARIO", "")
	t.Setenv("ATLAS_READ_SOURCE", "")
	t.Setenv("ATLAS_AGENT_MODE", "")

	cfg := Load()
	if cfg.HTTPAddr != ":8080" {
		t.Fatalf("HTTPAddr = %q, want :8080", cfg.HTTPAddr)
	}
	if cfg.ProviderMode != "fake" {
		t.Fatalf("ProviderMode = %q, want fake", cfg.ProviderMode)
	}
	if cfg.DatabaseURL == "" {
		t.Fatal("DatabaseURL is empty")
	}
	if cfg.FakeScenario != "reef-healthy-baremetal" {
		t.Fatalf("FakeScenario = %q, want reef-healthy-baremetal", cfg.FakeScenario)
	}
	if cfg.FakeAgentScenario != "" {
		t.Fatalf("FakeAgentScenario = %q, want the happy-path default", cfg.FakeAgentScenario)
	}
	if cfg.ReadSource != "provider" {
		t.Fatalf("ReadSource = %q, want provider", cfg.ReadSource)
	}
	if cfg.AgentMode != "disabled" {
		t.Fatalf("AgentMode = %q, want disabled", cfg.AgentMode)
	}
	if cfg.CephDashboardURL != "" || cfg.CephDashboardUser != "" || cfg.CephDashboardPassword != "" || cfg.CephClusterName != "" {
		t.Fatalf("ceph dashboard config = %q/%q/%q/%q, want all empty by default", cfg.CephDashboardURL, cfg.CephDashboardUser, cfg.CephDashboardPassword, cfg.CephClusterName)
	}
	if cfg.CephDashboardInsecureTLS {
		t.Fatal("CephDashboardInsecureTLS = true, want false by default")
	}
}

func TestLoadReadsCephDashboardConfig(t *testing.T) {
	t.Setenv("ATLAS_CEPH_DASHBOARD_URL", "https://mon.example.invalid:8443")
	t.Setenv("ATLAS_CEPH_DASHBOARD_USER", "atlas-reader")
	t.Setenv("ATLAS_CEPH_DASHBOARD_PASSWORD", "secret")
	t.Setenv("ATLAS_CEPH_CLUSTER_NAME", "reef-lab")
	t.Setenv("ATLAS_CEPH_DASHBOARD_INSECURE_TLS", "true")

	cfg := Load()
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

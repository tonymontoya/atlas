package config

import "testing"

func TestLoadDefaultsToFakeProvider(t *testing.T) {
	t.Setenv("ATLAS_HTTP_ADDR", "")
	t.Setenv("ATLAS_DATABASE_URL", "")
	t.Setenv("ATLAS_PROVIDER_MODE", "")
	t.Setenv("ATLAS_FAKE_SCENARIO", "")
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
	if cfg.ReadSource != "provider" {
		t.Fatalf("ReadSource = %q, want provider", cfg.ReadSource)
	}
	if cfg.AgentMode != "disabled" {
		t.Fatalf("AgentMode = %q, want disabled", cfg.AgentMode)
	}
}

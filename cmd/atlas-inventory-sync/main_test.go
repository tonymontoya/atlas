package main

import (
	"context"
	"strings"
	"testing"

	"github.com/tonymontoya/ceph-atlas/internal/config"
)

func TestBuildInventoryProviderFakeMode(t *testing.T) {
	provider, name, err := buildInventoryProvider(config.Config{
		ProviderMode: config.ProviderModeFake,
		FakeScenario: "reef-healthy-baremetal",
	})
	if err != nil {
		t.Fatalf("buildInventoryProvider returned error: %v", err)
	}
	if name != "fake" {
		t.Fatalf("provider name = %q, want fake", name)
	}
	identity, err := provider.ClusterIdentity(context.Background())
	if err != nil {
		t.Fatalf("ClusterIdentity returned error: %v", err)
	}
	if identity.FSID == "" {
		t.Fatal("expected fake provider cluster identity")
	}
}

func TestBuildInventoryProviderCephMode(t *testing.T) {
	provider, name, err := buildInventoryProvider(config.Config{
		ProviderMode:             config.ProviderModeCeph,
		CephDashboardURL:         "https://mon.example.invalid:8443",
		CephDashboardUser:        "atlas-reader",
		CephDashboardPassword:    "secret",
		CephClusterName:          "reef-lab",
		CephDashboardInsecureTLS: true,
	})
	if err != nil {
		t.Fatalf("buildInventoryProvider returned error: %v", err)
	}
	if name != "ceph" {
		t.Fatalf("provider name = %q, want ceph", name)
	}
	if provider == nil {
		t.Fatal("expected constructed ceph provider")
	}
}

func TestBuildInventoryProviderCephModeRejectsMissingCredentials(t *testing.T) {
	_, _, err := buildInventoryProvider(config.Config{
		ProviderMode:      config.ProviderModeCeph,
		CephDashboardURL:  "https://mon.example.invalid:8443",
		CephDashboardUser: "atlas-reader",
	})
	if err == nil {
		t.Fatal("expected error for missing password")
	}
	if !strings.Contains(err.Error(), "Password") {
		t.Fatalf("error %q does not name the missing field", err)
	}
}

func TestBuildInventoryProviderRejectsUnknownMode(t *testing.T) {
	_, _, err := buildInventoryProvider(config.Config{
		ProviderMode: config.ProviderMode("bogus"),
	})
	if err == nil {
		t.Fatal("expected error for unknown provider mode")
	}
	if !strings.Contains(err.Error(), "bogus") {
		t.Fatalf("error %q does not name the rejected mode", err)
	}
}

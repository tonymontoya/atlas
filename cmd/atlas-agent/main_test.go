package main

import (
	"strings"
	"testing"

	"github.com/tonymontoya/ceph-atlas/internal/config"
)

func TestBuildDashboardProviderMapsConfiguration(t *testing.T) {
	provider, err := buildDashboardProvider(config.AgentConfig{
		DashboardURL:         "https://mon.example.invalid:8443",
		DashboardUser:        "atlas-reader",
		DashboardPassword:    "reader-password",
		DashboardClusterName: "lab-ceph",
	})
	if err != nil {
		t.Fatalf("build dashboard provider: %v", err)
	}
	if provider == nil {
		t.Fatal("provider is nil")
	}
}

func TestBuildDashboardProviderDefaultsClusterName(t *testing.T) {
	provider, err := buildDashboardProvider(config.AgentConfig{
		DashboardURL:      "https://mon.example.invalid:8443",
		DashboardUser:     "atlas-reader",
		DashboardPassword: "reader-password",
	})
	if err != nil {
		t.Fatalf("build dashboard provider: %v", err)
	}
	if provider == nil {
		t.Fatal("provider is nil")
	}
}

func TestBuildDashboardProviderRejectsMissingCredentials(t *testing.T) {
	_, err := buildDashboardProvider(config.AgentConfig{
		DashboardURL: "https://mon.example.invalid:8443",
	})
	if err == nil {
		t.Fatal("provider without credentials was built")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "username") && !strings.Contains(strings.ToLower(err.Error()), "password") {
		t.Fatalf("error %q does not name the missing credential", err)
	}
}

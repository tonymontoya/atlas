package fake

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/tonymontoya/ceph-atlas/internal/inventory"
	"github.com/tonymontoya/ceph-atlas/internal/providers"
)

func TestFixturesLoadHealthyBareMetal(t *testing.T) {
	provider := New(DefaultFixtureRoot(), "reef-healthy-baremetal")

	identity, err := provider.ClusterIdentity(context.Background())
	if err != nil {
		t.Fatalf("ClusterIdentity returned error: %v", err)
	}
	if identity.FSID == "" {
		t.Fatal("ClusterIdentity FSID is empty")
	}

	health, err := provider.Health(context.Background())
	if err != nil {
		t.Fatalf("Health returned error: %v", err)
	}
	if health.Status != inventory.HealthOK {
		t.Fatalf("Health status = %q, want %q", health.Status, inventory.HealthOK)
	}
}

func TestFixturesLoadOSDDownScenario(t *testing.T) {
	provider := New(DefaultFixtureRoot(), "reef-osd-down-baremetal")

	health, err := provider.Health(context.Background())
	if err != nil {
		t.Fatalf("Health returned error: %v", err)
	}
	if health.Status != inventory.HealthWarn {
		t.Fatalf("Health status = %q, want %q", health.Status, inventory.HealthWarn)
	}

	osds, err := provider.OSDs(context.Background())
	if err != nil {
		t.Fatalf("OSDs returned error: %v", err)
	}
	foundDown := false
	for _, osd := range osds {
		if !osd.Up {
			foundDown = true
		}
	}
	if !foundDown {
		t.Fatal("expected at least one down OSD")
	}
}

func TestFixturesReturnProviderErrorForMissingScenario(t *testing.T) {
	provider := New(DefaultFixtureRoot(), "missing")

	_, err := provider.Health(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	var providerErr providers.ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("error type = %T, want ProviderError", err)
	}
	if providerErr.Class != providers.ErrorUnavailable {
		t.Fatalf("error class = %q, want %q", providerErr.Class, providers.ErrorUnavailable)
	}
}

func TestFixturesRejectUnknownErrorClass(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "ceph", "synthetic")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	fixture := []byte(`{"error": {"class": "Bogus", "message": "not a real class"}}`)
	if err := os.WriteFile(filepath.Join(dir, "health.json"), fixture, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	_, err := New(root, "synthetic").Health(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	var providerErr providers.ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("error type = %T, want ProviderError", err)
	}
	if providerErr.Class != providers.ErrorMalformedResponse {
		t.Fatalf("error class = %q, want %q", providerErr.Class, providers.ErrorMalformedResponse)
	}
}

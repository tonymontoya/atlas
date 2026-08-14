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

func TestFixturesLoadExtendedInventory(t *testing.T) {
	scenarios := []string{
		"reef-healthy-baremetal",
		"reef-osd-down-baremetal",
		"reef-healthy-rook",
		"reef-osd-down-rook",
		"pacific-readonly",
	}
	ctx := context.Background()
	for _, scenario := range scenarios {
		t.Run(scenario, func(t *testing.T) {
			provider := New(DefaultFixtureRoot(), scenario)

			hosts, err := provider.Hosts(ctx)
			if err != nil {
				t.Fatalf("Hosts returned error: %v", err)
			}
			if len(hosts) == 0 {
				t.Fatal("Hosts is empty")
			}

			devices, err := provider.HostDevices(ctx, hosts[0].Name)
			if err != nil {
				t.Fatalf("HostDevices returned error: %v", err)
			}
			if len(devices) == 0 {
				t.Fatalf("HostDevices for first host %q is empty", hosts[0].Name)
			}

			daemons, err := provider.Daemons(ctx)
			if err != nil {
				t.Fatalf("Daemons returned error: %v", err)
			}
			if len(daemons) == 0 {
				t.Fatal("Daemons is empty")
			}

			pools, err := provider.Pools(ctx)
			if err != nil {
				t.Fatalf("Pools returned error: %v", err)
			}
			if len(pools) == 0 {
				t.Fatal("Pools is empty")
			}
		})
	}
}

func TestFixturesReturnNotFoundForUnknownHost(t *testing.T) {
	provider := New(DefaultFixtureRoot(), "reef-healthy-baremetal")

	_, err := provider.HostDevices(context.Background(), "missing-host.example.invalid")
	if err == nil {
		t.Fatal("expected error")
	}
	var providerErr providers.ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("error type = %T, want ProviderError", err)
	}
	if providerErr.Class != providers.ErrorNotFound {
		t.Fatalf("error class = %q, want %q", providerErr.Class, providers.ErrorNotFound)
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

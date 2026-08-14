package contracttest

import (
	"context"
	"errors"
	"testing"

	"github.com/tonymontoya/ceph-atlas/internal/inventory"
	"github.com/tonymontoya/ceph-atlas/internal/observability"
	"github.com/tonymontoya/ceph-atlas/internal/providers"
)

type Scenario string

const (
	ScenarioSuccess      Scenario = "success"
	ScenarioUnavailable  Scenario = "unavailable"
	ScenarioUnauthorized Scenario = "unauthorized"
	ScenarioMalformed    Scenario = "malformed"
	ScenarioPartial      Scenario = "partial"
)

// ReadProvider is the full read surface the suite validates.
type ReadProvider interface {
	providers.CephReadProvider
}

type ReadProviderFactory func(t *testing.T, scenario Scenario) ReadProvider

func RunReadProviderSuite(t *testing.T, factory ReadProviderFactory) {
	errorScenarios := []struct {
		scenario Scenario
		class    providers.ErrorClass
	}{
		{ScenarioUnavailable, providers.ErrorUnavailable},
		{ScenarioUnauthorized, providers.ErrorUnauthorized},
		{ScenarioMalformed, providers.ErrorMalformedResponse},
		{ScenarioPartial, providers.ErrorPartial},
	}
	methods := []struct {
		name          string
		call          func(ctx context.Context, p ReadProvider) error
		verifySuccess func(t *testing.T, p ReadProvider)
	}{
		{
			name: "ClusterIdentity",
			call: func(ctx context.Context, p ReadProvider) error {
				_, err := p.ClusterIdentity(ctx)
				return err
			},
			verifySuccess: func(t *testing.T, p ReadProvider) {
				identity, err := p.ClusterIdentity(context.Background())
				if err != nil {
					t.Fatalf("ClusterIdentity returned error: %v", err)
				}
				if identity.FSID == "" {
					t.Fatal("ClusterIdentity FSID is empty")
				}
			},
		},
		{
			name: "Health",
			call: func(ctx context.Context, p ReadProvider) error {
				_, err := p.Health(ctx)
				return err
			},
			verifySuccess: func(t *testing.T, p ReadProvider) {
				health, err := p.Health(context.Background())
				if err != nil {
					t.Fatalf("Health returned error: %v", err)
				}
				switch health.Status {
				case inventory.HealthOK, inventory.HealthWarn, inventory.HealthErr:
				default:
					t.Fatalf("Health status = %q, want a known HealthStatus", health.Status)
				}
			},
		},
		{
			name: "OSDs",
			call: func(ctx context.Context, p ReadProvider) error {
				_, err := p.OSDs(ctx)
				return err
			},
			verifySuccess: func(t *testing.T, p ReadProvider) {
				osds, err := p.OSDs(context.Background())
				if err != nil {
					t.Fatalf("OSDs returned error: %v", err)
				}
				if len(osds) == 0 {
					t.Fatal("OSDs returned empty inventory for a success scenario")
				}
			},
		},
		{
			name: "Hosts",
			call: func(ctx context.Context, p ReadProvider) error {
				_, err := p.Hosts(ctx)
				return err
			},
			verifySuccess: func(t *testing.T, p ReadProvider) {
				hosts, err := p.Hosts(context.Background())
				if err != nil {
					t.Fatalf("Hosts returned error: %v", err)
				}
				if len(hosts) == 0 {
					t.Fatal("Hosts returned empty inventory for a success scenario")
				}
				for _, host := range hosts {
					if host.Name == "" {
						t.Fatal("Hosts returned a host with an empty name")
					}
				}
			},
		},
		{
			name: "HostDevices",
			call: func(ctx context.Context, p ReadProvider) error {
				_, err := p.HostDevices(ctx, "host-device-probe.example.invalid")
				return err
			},
			verifySuccess: func(t *testing.T, p ReadProvider) {
				hosts, err := p.Hosts(context.Background())
				if err != nil {
					t.Fatalf("Hosts returned error: %v", err)
				}
				if len(hosts) == 0 {
					t.Fatal("Hosts returned empty inventory; cannot probe HostDevices")
				}
				devices, err := p.HostDevices(context.Background(), hosts[0].Name)
				if err != nil {
					t.Fatalf("HostDevices returned error: %v", err)
				}
				for _, device := range devices {
					if device.Serial == "" {
						t.Fatal("HostDevices returned a Storage Device with an empty serial")
					}
					if device.Host != hosts[0].Name {
						t.Fatalf("HostDevices returned device for host %q, want %q", device.Host, hosts[0].Name)
					}
				}
			},
		},
		{
			name: "Daemons",
			call: func(ctx context.Context, p ReadProvider) error {
				_, err := p.Daemons(ctx)
				return err
			},
			verifySuccess: func(t *testing.T, p ReadProvider) {
				daemons, err := p.Daemons(context.Background())
				if err != nil {
					t.Fatalf("Daemons returned error: %v", err)
				}
				if len(daemons) == 0 {
					t.Fatal("Daemons returned empty inventory for a success scenario")
				}
				for _, daemon := range daemons {
					if daemon.Type == "" || daemon.Name == "" {
						t.Fatalf("Daemons returned a Ceph Daemon with an empty type or name: %+v", daemon)
					}
				}
			},
		},
		{
			name: "Pools",
			call: func(ctx context.Context, p ReadProvider) error {
				_, err := p.Pools(ctx)
				return err
			},
			verifySuccess: func(t *testing.T, p ReadProvider) {
				pools, err := p.Pools(context.Background())
				if err != nil {
					t.Fatalf("Pools returned error: %v", err)
				}
				if len(pools) == 0 {
					t.Fatal("Pools returned empty inventory for a success scenario")
				}
				for _, pool := range pools {
					if pool.Name == "" {
						t.Fatal("Pools returned a Pool with an empty name")
					}
				}
			},
		},
	}
	for _, method := range methods {
		t.Run(method.name, func(t *testing.T) {
			t.Run("Success", func(t *testing.T) {
				method.verifySuccess(t, factory(t, ScenarioSuccess))
			})
			for _, s := range errorScenarios {
				t.Run(string(s.scenario), func(t *testing.T) {
					err := method.call(context.Background(), factory(t, s.scenario))
					assertErrorClass(t, err, s.class)
				})
			}
		})
	}
	t.Run("HostDevicesUnknownHost", func(t *testing.T) {
		provider := factory(t, ScenarioSuccess)
		_, err := provider.HostDevices(context.Background(), "host-device-probe.example.invalid")
		assertErrorClass(t, err, providers.ErrorNotFound)
	})
	t.Run("ContextCancellation", func(t *testing.T) {
		provider := factory(t, ScenarioSuccess)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		for _, method := range methods {
			t.Run(method.name, func(t *testing.T) {
				assertErrorClass(t, method.call(ctx, provider), providers.ErrorTimeout)
			})
		}
	})
}

func assertErrorClass(t *testing.T, err error, want providers.ErrorClass) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with class %q, got nil", want)
	}
	var providerErr providers.ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("error type = %T, want ProviderError", err)
	}
	if providerErr.Class != want {
		t.Fatalf("error class = %q, want %q (message: %s)", providerErr.Class, want, providerErr.Message)
	}
}

// ObservabilityProvider is the alert surface the suite validates.
type ObservabilityProvider interface {
	providers.ObservabilityProvider
}

type ObservabilityProviderFactory func(t *testing.T, scenario Scenario) ObservabilityProvider

func RunObservabilityProviderSuite(t *testing.T, factory ObservabilityProviderFactory) {
	errorScenarios := []struct {
		scenario Scenario
		class    providers.ErrorClass
	}{
		{ScenarioUnavailable, providers.ErrorUnavailable},
		{ScenarioUnauthorized, providers.ErrorUnauthorized},
		{ScenarioMalformed, providers.ErrorMalformedResponse},
	}
	call := func(ctx context.Context, p ObservabilityProvider) error {
		_, err := p.CurrentAlerts(ctx)
		return err
	}
	t.Run("CurrentAlerts", func(t *testing.T) {
		t.Run("Success", func(t *testing.T) {
			p := factory(t, ScenarioSuccess)
			alerts, err := p.CurrentAlerts(context.Background())
			if err != nil {
				t.Fatalf("CurrentAlerts returned error: %v", err)
			}
			if len(alerts) == 0 {
				t.Fatal("CurrentAlerts returned empty alerts for a success scenario")
			}
			for _, alert := range alerts {
				if alert.Name == "" {
					t.Fatalf("CurrentAlerts returned an alert with an empty name: %+v", alert)
				}
				if alert.Severity == "" {
					t.Fatalf("CurrentAlerts returned an alert with an empty severity: %+v", alert)
				}
				switch alert.State {
				case observability.AlertStateFiring, observability.AlertStatePending, observability.AlertStateResolved:
				default:
					t.Fatalf("CurrentAlerts alert state = %q, want a known AlertState", alert.State)
				}
				if alert.StartedAt.IsZero() {
					t.Fatalf("CurrentAlerts returned an alert with a zero StartedAt: %+v", alert)
				}
				if alert.Source == "" {
					t.Fatalf("CurrentAlerts returned an alert with an empty source: %+v", alert)
				}
			}
		})
		for _, s := range errorScenarios {
			t.Run(string(s.scenario), func(t *testing.T) {
				assertErrorClass(t, call(context.Background(), factory(t, s.scenario)), s.class)
			})
		}
	})
	t.Run("ContextCancellation", func(t *testing.T) {
		provider := factory(t, ScenarioSuccess)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		t.Run("CurrentAlerts", func(t *testing.T) {
			assertErrorClass(t, call(ctx, provider), providers.ErrorTimeout)
		})
	})
}

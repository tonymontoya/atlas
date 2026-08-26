package contracttest

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/tonymontoya/ceph-atlas/internal/apperr"
	"github.com/tonymontoya/ceph-atlas/internal/inventory"
	"github.com/tonymontoya/ceph-atlas/internal/inventory/entities"
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

// Return nil for a scenario the implementation cannot produce (for example
// Partial over an HTTP transport that never yields partial payloads); the
// suite skips that scenario.
type ReadProviderFactory func(t *testing.T, scenario Scenario) ReadProvider

// providerRead is one read the suite exercises: the error-path call
// every scenario runs, the success verification, and optional
// entity-specific semantics run as extra subtests.
type providerRead struct {
	name          string
	call          func(ctx context.Context, p ReadProvider) error
	verifySuccess func(t *testing.T, p ReadProvider)
	// extraSubtests runs read-specific semantics as subtests of this
	// read's scope; nil when the shared checks suffice.
	extraSubtests func(t *testing.T, p ReadProvider)
}

// entityReads names the suite's per-entity coverage: how each declared
// entity is exercised on a read provider. Storage Devices keep their
// host-scoped provider shape: their exercise probes HostDevices per
// host and asserts the unknown-host semantic.
var entityReads = map[entities.Entity]providerRead{
	entities.OSDs: {
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
	entities.Hosts: {
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
	entities.StorageDevices: {
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
		extraSubtests: func(t *testing.T, p ReadProvider) {
			t.Run("UnknownHost", func(t *testing.T) {
				_, err := p.HostDevices(context.Background(), "host-device-probe.example.invalid")
				assertErrorClass(t, err, apperr.NotFound)
			})
		},
	},
	entities.Daemons: {
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
				switch daemon.Type {
				case inventory.DaemonMon, inventory.DaemonMgr, inventory.DaemonOsd, inventory.DaemonMds, inventory.DaemonRgw:
				default:
					t.Fatalf("Ceph Daemon type = %q, want a known DaemonType", daemon.Type)
				}
				switch daemon.Status {
				case inventory.DaemonRunning, inventory.DaemonStopped, inventory.DaemonStarting, inventory.DaemonError, inventory.DaemonUnknown:
				default:
					t.Fatalf("Ceph Daemon status = %q, want a known DaemonStatus", daemon.Status)
				}
			}
		},
	},
	entities.Pools: {
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

// declaredEntityReads builds the suite's per-entity coverage in
// declaration order, reporting a declared entity that lacks coverage —
// a newly declared entity cannot silently skip contract testing.
func declaredEntityReads() ([]providerRead, error) {
	reads := make([]providerRead, 0, len(entities.All))
	for _, entity := range entities.All {
		read, ok := entityReads[entity]
		if !ok {
			return nil, fmt.Errorf("declared entity %q lacks provider contract coverage", entity.Noun)
		}
		reads = append(reads, read)
	}
	return reads, nil
}

func RunReadProviderSuite(t *testing.T, factory ReadProviderFactory) {
	declared, err := declaredEntityReads()
	if err != nil {
		t.Fatal(err)
	}
	errorScenarios := []struct {
		scenario Scenario
		class    apperr.Class
	}{
		{ScenarioUnavailable, apperr.Unavailable},
		{ScenarioUnauthorized, apperr.Unauthorized},
		{ScenarioMalformed, apperr.MalformedResponse},
		{ScenarioPartial, apperr.Partial},
	}
	// ClusterIdentity and Health stay bespoke: singleton-shaped, not
	// declared list entities.
	bespokeReads := []providerRead{
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
	}
	reads := append(bespokeReads, declared...)
	for _, read := range reads {
		t.Run(read.name, func(t *testing.T) {
			t.Run("Success", func(t *testing.T) {
				read.verifySuccess(t, factory(t, ScenarioSuccess))
			})
			for _, s := range errorScenarios {
				t.Run(string(s.scenario), func(t *testing.T) {
					p := factory(t, s.scenario)
					if p == nil {
						t.Logf("provider does not produce scenario %q; skipping", s.scenario)
						return
					}
					err := read.call(context.Background(), p)
					assertErrorClass(t, err, s.class)
				})
			}
			if read.extraSubtests != nil {
				read.extraSubtests(t, factory(t, ScenarioSuccess))
			}
		})
	}
	t.Run("ContextCancellation", func(t *testing.T) {
		provider := factory(t, ScenarioSuccess)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		for _, read := range reads {
			t.Run(read.name, func(t *testing.T) {
				assertErrorClass(t, read.call(ctx, provider), apperr.Timeout)
			})
		}
	})
}

func assertErrorClass(t *testing.T, err error, want apperr.Class) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with class %q, got nil", want)
	}
	var appErr apperr.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("error type = %T, want apperr.Error", err)
	}
	if appErr.Class != want {
		t.Fatalf("error class = %q, want %q (message: %s)", appErr.Class, want, appErr.Message)
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
		class    apperr.Class
	}{
		{ScenarioUnavailable, apperr.Unavailable},
		{ScenarioUnauthorized, apperr.Unauthorized},
		{ScenarioMalformed, apperr.MalformedResponse},
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
			assertErrorClass(t, call(ctx, provider), apperr.Timeout)
		})
	})
}

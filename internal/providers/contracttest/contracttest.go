package contracttest

import (
	"context"
	"errors"
	"testing"

	"github.com/tonymontoya/ceph-atlas/internal/inventory"
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

type CephReadProviderFactory func(t *testing.T, scenario Scenario) providers.CephReadProvider

func RunCephReadProviderSuite(t *testing.T, factory CephReadProviderFactory) {
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
		call          func(ctx context.Context, p providers.CephReadProvider) error
		verifySuccess func(t *testing.T, p providers.CephReadProvider)
	}{
		{
			name: "ClusterIdentity",
			call: func(ctx context.Context, p providers.CephReadProvider) error {
				_, err := p.ClusterIdentity(ctx)
				return err
			},
			verifySuccess: func(t *testing.T, p providers.CephReadProvider) {
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
			call: func(ctx context.Context, p providers.CephReadProvider) error {
				_, err := p.Health(ctx)
				return err
			},
			verifySuccess: func(t *testing.T, p providers.CephReadProvider) {
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
			call: func(ctx context.Context, p providers.CephReadProvider) error {
				_, err := p.OSDs(ctx)
				return err
			},
			verifySuccess: func(t *testing.T, p providers.CephReadProvider) {
				osds, err := p.OSDs(context.Background())
				if err != nil {
					t.Fatalf("OSDs returned error: %v", err)
				}
				if len(osds) == 0 {
					t.Fatal("OSDs returned empty inventory for a success scenario")
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

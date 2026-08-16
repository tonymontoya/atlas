package app

import (
	"context"
	"testing"

	"github.com/tonymontoya/ceph-atlas/internal/config"
)

func TestNewFromConfigDefaultsToProviderReadSource(t *testing.T) {
	application, err := NewFromConfig(context.Background(), config.Config{
		FakeScenario: "reef-healthy-baremetal",
		ReadSource:   config.ReadSourceProvider,
		AgentMode:    config.AgentModeDisabled,
	})
	if err != nil {
		t.Fatalf("NewFromConfig returned error: %v", err)
	}
	t.Cleanup(func() { _ = application.Close() })

	identity, err := application.CephProvider.ClusterIdentity(context.Background())
	if err != nil {
		t.Fatalf("ClusterIdentity returned error: %v", err)
	}
	if identity.FSID == "" {
		t.Fatal("expected fake provider cluster identity")
	}
}

func TestNewFromConfigRejectsUnsupportedReadSource(t *testing.T) {
	_, err := NewFromConfig(context.Background(), config.Config{
		ReadSource: config.ReadSource("unsupported"),
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNewFromConfigRejectsUnsupportedAgentMode(t *testing.T) {
	_, err := NewFromConfig(context.Background(), config.Config{
		AgentMode: config.AgentMode("eager"),
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNewFromConfigRejectsUnknownFakeAgentScenario(t *testing.T) {
	_, err := NewFromConfig(context.Background(), config.Config{
		ReadSource:        config.ReadSourcePostgres,
		AgentMode:         config.AgentModeFake,
		FakeAgentScenario: "nonsense",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

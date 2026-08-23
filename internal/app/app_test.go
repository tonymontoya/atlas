package app

import (
	"context"
	"testing"

	"github.com/tonymontoya/ceph-atlas/internal/config"
	"github.com/tonymontoya/ceph-atlas/internal/store"
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

	index, err := application.ClusterInventory.ListClusterSummaries(context.Background(), store.ListClustersQuery{})
	if err != nil {
		t.Fatalf("ListClusterSummaries returned error: %v", err)
	}
	if index.Total != 1 || len(index.Clusters) != 1 || index.Clusters[0].FSID == nil {
		t.Fatalf("index = %+v, want the fake provider's cluster", index)
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

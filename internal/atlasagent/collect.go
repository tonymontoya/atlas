package atlasagent

import (
	"context"
	"fmt"
	"time"

	"github.com/tonymontoya/ceph-atlas/internal/fleet"
	"github.com/tonymontoya/ceph-atlas/internal/inventory"
	"github.com/tonymontoya/ceph-atlas/internal/providers"
)

// ObservationBatch is one typed Observation Batch for one collection
// cycle (ADR-0025): the request body of POST /api/v1/agent/observations.
// It mirrors the server's handler struct and the OpenAPI
// AgentObservationBatch schema. Provider and scenario are deliberately
// absent — Atlas records provider "agent" server-side, and Dashboard
// credentials never leave the Agent.
type ObservationBatch struct {
	ObservedAt time.Time                 `json:"observedAt"`
	Cluster    fleet.ClusterIdentity     `json:"cluster"`
	Health     inventory.Health          `json:"health"`
	OSDs       []inventory.OSD           `json:"osds"`
	Hosts      []inventory.Host          `json:"hosts"`
	Devices    []inventory.StorageDevice `json:"devices"`
	Daemons    []inventory.Daemon        `json:"daemons"`
	Pools      []inventory.Pool          `json:"pools"`
}

// Collect reads one complete inventory batch from the read provider
// running inside the Agent, in the same order the control-plane sync
// uses: identity, health, OSDs, hosts, per-host devices, daemons,
// pools. Collection is atomic from the push's perspective — any read
// failure aborts the batch, so a partial batch is never pushed.
func Collect(ctx context.Context, provider providers.CephReadProvider, observedAt time.Time) (ObservationBatch, error) {
	identity, err := provider.ClusterIdentity(ctx)
	if err != nil {
		return ObservationBatch{}, fmt.Errorf("read cluster identity: %w", err)
	}
	health, err := provider.Health(ctx)
	if err != nil {
		return ObservationBatch{}, fmt.Errorf("read health: %w", err)
	}
	osds, err := provider.OSDs(ctx)
	if err != nil {
		return ObservationBatch{}, fmt.Errorf("read osds: %w", err)
	}
	hosts, err := provider.Hosts(ctx)
	if err != nil {
		return ObservationBatch{}, fmt.Errorf("read hosts: %w", err)
	}
	devices, err := providers.AllHostDevices(ctx, provider, hosts)
	if err != nil {
		return ObservationBatch{}, fmt.Errorf("read storage devices: %w", err)
	}
	daemons, err := provider.Daemons(ctx)
	if err != nil {
		return ObservationBatch{}, fmt.Errorf("read daemons: %w", err)
	}
	pools, err := provider.Pools(ctx)
	if err != nil {
		return ObservationBatch{}, fmt.Errorf("read pools: %w", err)
	}

	return ObservationBatch{
		ObservedAt: observedAt,
		Cluster:    identity,
		Health:     health,
		OSDs:       osds,
		Hosts:      hosts,
		Devices:    devices,
		Daemons:    daemons,
		Pools:      pools,
	}, nil
}

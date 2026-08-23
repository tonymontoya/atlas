package singlecluster

import (
	"context"
	"errors"
	"testing"

	"github.com/tonymontoya/ceph-atlas/internal/apperr"
	"github.com/tonymontoya/ceph-atlas/internal/inventory"
	"github.com/tonymontoya/ceph-atlas/internal/providers/fake"
	"github.com/tonymontoya/ceph-atlas/internal/store"
)

const (
	fakeHealthyFSID = "00000000-0000-4000-8000-000000000101"
	fakeOSDDownFSID = "00000000-0000-4000-8000-000000000102"
)

func TestIndexListsTheProvidersCluster(t *testing.T) {
	reader := New(fake.New(fake.DefaultFixtureRoot(), "reef-healthy-baremetal"))

	index, err := reader.ListClusterSummaries(context.Background(), store.ListClustersQuery{})
	if err != nil {
		t.Fatalf("ListClusterSummaries: %v", err)
	}
	if index.Total != 1 || len(index.Clusters) != 1 {
		t.Fatalf("index = %+v, want exactly the provider's cluster", index)
	}
	summary := index.Clusters[0]
	if summary.FSID == nil || *summary.FSID != fakeHealthyFSID || summary.Name != "reef-baremetal-healthy" {
		t.Fatalf("summary = %+v, want the fixture identity", summary)
	}
	if summary.HealthStatus == nil || *summary.HealthStatus != "HEALTH_OK" {
		t.Fatalf("health status = %v, want HEALTH_OK", summary.HealthStatus)
	}
	if summary.AgentLastSeen != nil {
		t.Fatalf("agent last-seen = %v, want none in provider mode", summary.AgentLastSeen)
	}
}

func TestIndexHonorsSearchAndOffset(t *testing.T) {
	reader := New(fake.New(fake.DefaultFixtureRoot(), "reef-healthy-baremetal"))
	ctx := context.Background()

	bySearch, err := reader.ListClusterSummaries(ctx, store.ListClustersQuery{Search: "REEF"})
	if err != nil || bySearch.Total != 1 {
		t.Fatalf("search by name = %+v, err %v", bySearch, err)
	}
	miss, err := reader.ListClusterSummaries(ctx, store.ListClustersQuery{Search: "rook"})
	if err != nil || miss.Total != 0 || len(miss.Clusters) != 0 {
		t.Fatalf("search miss = %+v, err %v", miss, err)
	}
	pageTwo, err := reader.ListClusterSummaries(ctx, store.ListClustersQuery{Offset: 1})
	if err != nil || pageTwo.Total != 1 || len(pageTwo.Clusters) != 0 {
		t.Fatalf("offset page = %+v, err %v", pageTwo, err)
	}
}

func TestScopedReadsServeOnlyTheProvidersCluster(t *testing.T) {
	reader := New(fake.New(fake.DefaultFixtureRoot(), "reef-osd-down-baremetal"))
	ctx := context.Background()

	health, err := reader.ClusterHealth(ctx, fakeOSDDownFSID)
	if err != nil {
		t.Fatalf("ClusterHealth: %v", err)
	}
	if health.Status != inventory.HealthWarn {
		t.Fatalf("health status = %q, want HEALTH_WARN", health.Status)
	}

	devices, err := reader.ClusterStorageDevices(ctx, fakeOSDDownFSID)
	if err != nil {
		t.Fatalf("ClusterStorageDevices: %v", err)
	}
	if len(devices) != 3 {
		t.Fatalf("device count = %d, want 3 across all hosts", len(devices))
	}
}

func TestScopedReadsRejectOtherFSIDs(t *testing.T) {
	reader := New(fake.New(fake.DefaultFixtureRoot(), "reef-healthy-baremetal"))
	ctx := context.Background()

	reads := []struct {
		name string
		call func() error
	}{
		{"health", func() error { _, err := reader.ClusterHealth(ctx, "00000000-0000-4000-8000-0000000009ff"); return err }},
		{"osds", func() error { _, err := reader.ClusterOSDs(ctx, "00000000-0000-4000-8000-0000000009ff"); return err }},
		{"devices", func() error { _, err := reader.ClusterStorageDevices(ctx, "not-a-uuid"); return err }},
		{"pools", func() error { _, err := reader.ClusterPools(ctx, ""); return err }},
	}
	for _, read := range reads {
		t.Run(read.name, func(t *testing.T) {
			err := read.call()
			var appErr apperr.Error
			if !errors.As(err, &appErr) || appErr.Class != apperr.NotFound {
				t.Fatalf("error = %v, want NotFound", err)
			}
		})
	}
}

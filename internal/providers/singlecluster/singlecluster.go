// Package singlecluster adapts one CephReadProvider — the provider
// read source's single implicit cluster — to the cluster-scoped
// inventory reads the API serves. The provider's own identity is the
// only addressable cluster; any other FSID is a not-found, so provider
// mode keeps the same per-cluster semantics as the postgres read
// source over exactly one cluster.
package singlecluster

import (
	"context"
	"strings"

	"github.com/tonymontoya/ceph-atlas/internal/apperr"
	"github.com/tonymontoya/ceph-atlas/internal/inventory"
	"github.com/tonymontoya/ceph-atlas/internal/providers"
	"github.com/tonymontoya/ceph-atlas/internal/store"
)

type Reader struct {
	provider providers.CephReadProvider
}

func New(provider providers.CephReadProvider) *Reader {
	return &Reader{provider: provider}
}

func clusterNotFound() apperr.Error {
	return apperr.Error{Class: apperr.NotFound, Message: "cluster not found"}
}

// resolve answers whether fsid addresses the provider's cluster.
func (r *Reader) resolve(ctx context.Context, fsid string) error {
	identity, err := r.provider.ClusterIdentity(ctx)
	if err != nil {
		return err
	}
	if !store.IsUUIDShape(fsid) || !strings.EqualFold(identity.FSID, fsid) {
		return clusterNotFound()
	}
	return nil
}

// ListClusterSummaries indexes the one cluster the provider serves:
// identity and health from the provider, and no Agent last-seen —
// provider-mode observations are pulled, not pushed. A provider health
// failure lists the cluster without health rather than failing the
// whole index; the scoped health endpoint reports the failure.
func (r *Reader) ListClusterSummaries(ctx context.Context, query store.ListClustersQuery) (store.ClusterIndex, error) {
	query.Limit, query.Offset = store.ClampPage(query.Limit, query.Offset)
	index := store.ClusterIndex{Clusters: make([]store.ClusterSummary, 0), Limit: query.Limit, Offset: query.Offset}

	identity, err := r.provider.ClusterIdentity(ctx)
	if err != nil {
		return store.ClusterIndex{}, err
	}
	health, healthErr := r.provider.Health(ctx)
	if healthErr != nil {
		health = inventory.Health{}
	}

	if search := strings.ToLower(query.Search); search != "" &&
		!strings.Contains(strings.ToLower(identity.Name), search) &&
		!strings.Contains(strings.ToLower(identity.FSID), search) {
		return index, nil
	}
	index.Total = 1
	if query.Offset > 0 {
		return index, nil
	}

	fsid, name, version := identity.FSID, identity.Name, identity.CephVersion
	summary := store.ClusterSummary{
		FSID:        &fsid,
		Name:        name,
		ClusterType: identity.Type,
		CephVersion: &version,
	}
	if health.Status != "" {
		status, healthSummary := string(health.Status), health.Summary
		summary.HealthStatus = &status
		summary.HealthSummary = &healthSummary
	}
	index.Clusters = append(index.Clusters, summary)
	return index, nil
}

func (r *Reader) ClusterHealth(ctx context.Context, fsid string) (inventory.Health, error) {
	if err := r.resolve(ctx, fsid); err != nil {
		return inventory.Health{}, err
	}
	return r.provider.Health(ctx)
}

func (r *Reader) ClusterOSDs(ctx context.Context, fsid string) ([]inventory.OSD, error) {
	if err := r.resolve(ctx, fsid); err != nil {
		return nil, err
	}
	return r.provider.OSDs(ctx)
}

func (r *Reader) ClusterHosts(ctx context.Context, fsid string) ([]inventory.Host, error) {
	if err := r.resolve(ctx, fsid); err != nil {
		return nil, err
	}
	return r.provider.Hosts(ctx)
}

func (r *Reader) ClusterStorageDevices(ctx context.Context, fsid string) ([]inventory.StorageDevice, error) {
	if err := r.resolve(ctx, fsid); err != nil {
		return nil, err
	}
	hosts, err := r.provider.Hosts(ctx)
	if err != nil {
		return nil, err
	}
	return providers.AllHostDevices(ctx, r.provider, hosts)
}

func (r *Reader) ClusterDaemons(ctx context.Context, fsid string) ([]inventory.Daemon, error) {
	if err := r.resolve(ctx, fsid); err != nil {
		return nil, err
	}
	return r.provider.Daemons(ctx)
}

func (r *Reader) ClusterPools(ctx context.Context, fsid string) ([]inventory.Pool, error) {
	if err := r.resolve(ctx, fsid); err != nil {
		return nil, err
	}
	return r.provider.Pools(ctx)
}

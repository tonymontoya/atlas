// Package singlecluster adapts one CephReadProvider — the provider
// read source's single implicit cluster — to the cluster-scoped
// inventory reads the API serves. The provider's own identity is the
// only addressable cluster; any other FSID is a not-found, so provider
// mode keeps the same per-cluster semantics as the postgres read
// source over exactly one cluster.
//
// The list-shaped reads derive their wiring from the entity
// declaration: a package-level binding table names how the provider
// serves each declared entity, and construction fails loudly when a
// declared entity lacks a binding.
package singlecluster

import (
	"context"
	"fmt"
	"strings"

	"github.com/tonymontoya/ceph-atlas/internal/apperr"
	"github.com/tonymontoya/ceph-atlas/internal/inventory"
	"github.com/tonymontoya/ceph-atlas/internal/inventory/entities"
	"github.com/tonymontoya/ceph-atlas/internal/providers"
	"github.com/tonymontoya/ceph-atlas/internal/store"
)

// providerFetch serves one declared entity's rows from a read
// provider. The concrete slice type crosses only inside this package:
// each named reader method asserts its own type back out, so the
// adapter's seams stay typed.
type providerFetch func(ctx context.Context, p providers.CephReadProvider) (any, error)

// providerBindings names how the read provider serves each declared
// entity. Storage Devices keep their special shape: the provider
// serves them per host, so that binding lists the cluster's hosts and
// fans out across them.
var providerBindings = map[entities.Entity]providerFetch{
	entities.OSDs: func(ctx context.Context, p providers.CephReadProvider) (any, error) {
		return p.OSDs(ctx)
	},
	entities.Hosts: func(ctx context.Context, p providers.CephReadProvider) (any, error) {
		return p.Hosts(ctx)
	},
	entities.StorageDevices: func(ctx context.Context, p providers.CephReadProvider) (any, error) {
		hosts, err := p.Hosts(ctx)
		if err != nil {
			return nil, err
		}
		return providers.AllHostDevices(ctx, p, hosts)
	},
	entities.Daemons: func(ctx context.Context, p providers.CephReadProvider) (any, error) {
		return p.Daemons(ctx)
	},
	entities.Pools: func(ctx context.Context, p providers.CephReadProvider) (any, error) {
		return p.Pools(ctx)
	},
}

type Reader struct {
	provider providers.CephReadProvider
	bindings map[entities.Entity]providerFetch
}

// New builds the adapter with every declared entity bound to its
// provider fetch. A declared entity without a binding panics: the
// wiring bug is a programmer error that construction — in tests and at
// process start — must surface, never a request-time surprise.
func New(provider providers.CephReadProvider) *Reader {
	return &Reader{provider: provider, bindings: bindEntities(providerBindings)}
}

// bindEntities copies the source table for one reader, failing loudly
// when a declared entity lacks a binding.
func bindEntities(source map[entities.Entity]providerFetch) map[entities.Entity]providerFetch {
	bindings := make(map[entities.Entity]providerFetch, len(entities.All))
	for _, entity := range entities.All {
		fetch, ok := source[entity]
		if !ok {
			panic(fmt.Sprintf("singlecluster: declared entity %q has no provider binding", entity.Noun))
		}
		bindings[entity] = fetch
	}
	return bindings
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

// listEntity serves one declared entity's rows for the cluster the
// FSID addresses: resolve first, then the entity's binding, then the
// declared slice type asserted back. A binding serving the wrong type
// is a wiring bug that fails loudly here.
func listEntity[T any](ctx context.Context, r *Reader, fsid string, entity entities.Entity) ([]T, error) {
	if err := r.resolve(ctx, fsid); err != nil {
		return nil, err
	}
	rows, err := r.bindings[entity](ctx, r.provider)
	if err != nil {
		return nil, err
	}
	typed, ok := rows.([]T)
	if !ok {
		panic(fmt.Sprintf("singlecluster: binding for entity %q serves %T, want []%T", entity.Noun, rows, []T(nil)))
	}
	return typed, nil
}

// The list-shaped cluster reads delegate through the
// declaration-driven binding table; signatures stay on the seam
// (completeness is enforced by TestEveryDeclaredEntityHasReaderMethod
// and by New's binding check).

// ClusterOSDs serves the cluster's current OSD inventory.
func (r *Reader) ClusterOSDs(ctx context.Context, fsid string) ([]inventory.OSD, error) {
	return listEntity[inventory.OSD](ctx, r, fsid, entities.OSDs)
}

func (r *Reader) ClusterHosts(ctx context.Context, fsid string) ([]inventory.Host, error) {
	return listEntity[inventory.Host](ctx, r, fsid, entities.Hosts)
}

func (r *Reader) ClusterStorageDevices(ctx context.Context, fsid string) ([]inventory.StorageDevice, error) {
	return listEntity[inventory.StorageDevice](ctx, r, fsid, entities.StorageDevices)
}

func (r *Reader) ClusterDaemons(ctx context.Context, fsid string) ([]inventory.Daemon, error) {
	return listEntity[inventory.Daemon](ctx, r, fsid, entities.Daemons)
}

func (r *Reader) ClusterPools(ctx context.Context, fsid string) ([]inventory.Pool, error) {
	return listEntity[inventory.Pool](ctx, r, fsid, entities.Pools)
}

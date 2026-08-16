package inventorysync

import (
	"context"
	"errors"
	"time"

	"github.com/tonymontoya/ceph-atlas/internal/providers"
	"github.com/tonymontoya/ceph-atlas/internal/providers/fake"
	"github.com/tonymontoya/ceph-atlas/internal/store"
)

type Writer interface {
	BeginInventorySyncRun(ctx context.Context, run store.BeginSyncRun) (int64, error)
	SaveInventoryObservation(ctx context.Context, obs store.InventoryObservation) (store.SaveInventoryResult, error)
	SucceedInventorySyncRun(ctx context.Context, result store.SyncRunResult) error
	FailInventorySyncRun(ctx context.Context, failure store.SyncRunFailure) error
}

type Options struct {
	// ProviderName is recorded on the sync run and the observation batch
	// (for example "fake" or "ceph"). RunFakeOnce sets it to "fake".
	ProviderName string
	Scenario     string
	ObservedAt   time.Time
}

func RunFakeOnce(ctx context.Context, writer Writer, opts Options) (store.SaveInventoryResult, error) {
	opts.ProviderName = "fake"
	return RunOnce(ctx, writer, fake.New(fake.DefaultFixtureRoot(), opts.Scenario), opts)
}

func RunOnce(ctx context.Context, writer Writer, provider providers.CephReadProvider, opts Options) (store.SaveInventoryResult, error) {
	if opts.ProviderName == "" {
		return store.SaveInventoryResult{}, errors.New("inventorysync.Options.ProviderName is required")
	}
	runID, err := writer.BeginInventorySyncRun(ctx, store.BeginSyncRun{
		Provider: opts.ProviderName,
		Scenario: opts.Scenario,
	})
	if err != nil {
		return store.SaveInventoryResult{}, err
	}

	result, err := runOnce(ctx, writer, provider, opts)
	if err != nil {
		failErr := writer.FailInventorySyncRun(ctx, failureFromError(runID, err))
		if failErr != nil {
			return store.SaveInventoryResult{}, errors.Join(err, failErr)
		}
		return store.SaveInventoryResult{}, err
	}
	if err := writer.SucceedInventorySyncRun(ctx, store.SyncRunResult{RunID: runID, SnapshotID: result.SnapshotID}); err != nil {
		return store.SaveInventoryResult{}, err
	}
	return result, nil
}

func runOnce(ctx context.Context, writer Writer, provider providers.CephReadProvider, opts Options) (store.SaveInventoryResult, error) {
	identity, err := provider.ClusterIdentity(ctx)
	if err != nil {
		return store.SaveInventoryResult{}, err
	}
	health, err := provider.Health(ctx)
	if err != nil {
		return store.SaveInventoryResult{}, err
	}
	osds, err := provider.OSDs(ctx)
	if err != nil {
		return store.SaveInventoryResult{}, err
	}
	hosts, err := provider.Hosts(ctx)
	if err != nil {
		return store.SaveInventoryResult{}, err
	}
	devices, err := providers.AllHostDevices(ctx, provider, hosts)
	if err != nil {
		return store.SaveInventoryResult{}, err
	}
	daemons, err := provider.Daemons(ctx)
	if err != nil {
		return store.SaveInventoryResult{}, err
	}
	pools, err := provider.Pools(ctx)
	if err != nil {
		return store.SaveInventoryResult{}, err
	}

	observedAt := opts.ObservedAt
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}

	return writer.SaveInventoryObservation(ctx, store.InventoryObservation{
		Provider:   opts.ProviderName,
		Scenario:   opts.Scenario,
		ObservedAt: observedAt,
		Cluster:    identity,
		Health:     health,
		OSDs:       osds,
		Hosts:      hosts,
		Devices:    devices,
		Daemons:    daemons,
		Pools:      pools,
	})
}

func failureFromError(runID int64, err error) store.SyncRunFailure {
	failure := store.SyncRunFailure{
		RunID:        runID,
		ErrorMessage: err.Error(),
	}
	var providerErr providers.ProviderError
	if errors.As(err, &providerErr) {
		failure.ErrorClass = string(providerErr.Class)
	} else {
		failure.ErrorClass = "Internal"
	}
	return failure
}

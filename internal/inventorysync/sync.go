package inventorysync

import (
	"context"
	"errors"
	"time"

	"github.com/tonymontoya/ceph-atlas/internal/apperr"
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

// AgentProviderName is the provider recorded on runs and snapshots an
// enrolled Atlas Agent pushed. RunPush pins it server-side so pushes
// are never attributed by payload claims (ADR-0025).
const AgentProviderName = "agent"

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
	return recordRun(ctx, writer, store.BeginSyncRun{
		Provider: opts.ProviderName,
		Scenario: opts.Scenario,
	}, func(ctx context.Context) (store.SaveInventoryResult, error) {
		return runOnce(ctx, writer, provider, opts)
	})
}

// RunPush persists one agent-pushed Observation Batch (ADR-0025). The
// batch was already collected by the Agent, so there is no provider to
// pull from: the run lifecycle wraps the existing single-transaction
// save path, pinning provider to "agent" and leaving the scenario
// empty. clusterID is the cluster the client certificate resolved to,
// stamped on the run at begin so failed pushes stay attributed. A save
// failure still records a failed run; the batch itself stays
// all-or-nothing.
func RunPush(ctx context.Context, writer Writer, clusterID int64, obs store.InventoryObservation) (store.SaveInventoryResult, error) {
	obs.Provider = AgentProviderName
	begin := store.BeginSyncRun{Provider: AgentProviderName}
	if clusterID > 0 {
		begin.ClusterID = &clusterID
	}
	return recordRun(ctx, writer, begin, func(ctx context.Context) (store.SaveInventoryResult, error) {
		return writer.SaveInventoryObservation(ctx, obs)
	})
}

// recordRun wraps one save in the sync-run lifecycle: begin, save,
// succeed or fail with the error's class.
func recordRun(ctx context.Context, writer Writer, run store.BeginSyncRun, save func(context.Context) (store.SaveInventoryResult, error)) (store.SaveInventoryResult, error) {
	runID, err := writer.BeginInventorySyncRun(ctx, run)
	if err != nil {
		return store.SaveInventoryResult{}, err
	}

	result, err := save(ctx)
	if err != nil {
		failErr := writer.FailInventorySyncRun(ctx, failureFromError(runID, err))
		if failErr != nil {
			return store.SaveInventoryResult{}, errors.Join(err, failErr)
		}
		return store.SaveInventoryResult{}, err
	}
	if err := writer.SucceedInventorySyncRun(ctx, store.SyncRunResult{RunID: runID, SnapshotID: result.SnapshotID, ClusterID: result.ClusterID}); err != nil {
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
	var appErr apperr.Error
	if errors.As(err, &appErr) {
		failure.ErrorClass = string(appErr.Class)
	} else {
		failure.ErrorClass = string(apperr.Internal)
	}
	return failure
}

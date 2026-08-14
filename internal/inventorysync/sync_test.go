package inventorysync

import (
	"context"
	"testing"
	"time"

	"github.com/tonymontoya/ceph-atlas/internal/store"
)

type recordingWriter struct {
	observation store.InventoryObservation
	succeeded   store.SyncRunResult
	failed      store.SyncRunFailure
}

func (w *recordingWriter) BeginInventorySyncRun(_ context.Context, run store.BeginSyncRun) (int64, error) {
	return 30, nil
}

func (w *recordingWriter) SaveInventoryObservation(_ context.Context, obs store.InventoryObservation) (store.SaveInventoryResult, error) {
	w.observation = obs
	return store.SaveInventoryResult{ClusterID: 10, SnapshotID: 20}, nil
}

func (w *recordingWriter) SucceedInventorySyncRun(_ context.Context, result store.SyncRunResult) error {
	w.succeeded = result
	return nil
}

func (w *recordingWriter) FailInventorySyncRun(_ context.Context, failure store.SyncRunFailure) error {
	w.failed = failure
	return nil
}

func TestRunOnceCollectsProviderDataAndWritesObservation(t *testing.T) {
	writer := &recordingWriter{}
	observedAt := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)

	result, err := RunFakeOnce(context.Background(), writer, Options{
		Scenario:   "reef-osd-down-baremetal",
		ObservedAt: observedAt,
	})
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if result.SnapshotID != 20 {
		t.Fatalf("SnapshotID = %d, want 20", result.SnapshotID)
	}
	if writer.observation.Provider != "fake" {
		t.Fatalf("provider = %q, want fake", writer.observation.Provider)
	}
	if writer.observation.Cluster.FSID != "00000000-0000-4000-8000-000000000102" {
		t.Fatalf("cluster fsid = %q", writer.observation.Cluster.FSID)
	}
	if writer.observation.Health.Status != "HEALTH_WARN" {
		t.Fatalf("health status = %q, want HEALTH_WARN", writer.observation.Health.Status)
	}
	if len(writer.observation.OSDs) != 2 {
		t.Fatalf("OSD count = %d, want 2", len(writer.observation.OSDs))
	}
	if !writer.observation.ObservedAt.Equal(observedAt) {
		t.Fatalf("ObservedAt = %s, want %s", writer.observation.ObservedAt, observedAt)
	}
	if writer.succeeded.RunID != 30 || writer.succeeded.SnapshotID != 20 {
		t.Fatalf("sync success = %+v, want run 30 snapshot 20", writer.succeeded)
	}
	if writer.failed.RunID != 0 {
		t.Fatalf("unexpected sync failure = %+v", writer.failed)
	}
}

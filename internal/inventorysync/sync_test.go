package inventorysync

import (
	"context"
	"testing"
	"time"

	"github.com/tonymontoya/ceph-atlas/internal/providers/ceph"
	"github.com/tonymontoya/ceph-atlas/internal/providers/ceph/dashtest"
	"github.com/tonymontoya/ceph-atlas/internal/store"
)

type recordingWriter struct {
	begin       store.BeginSyncRun
	observation store.InventoryObservation
	succeeded   store.SyncRunResult
	failed      store.SyncRunFailure
}

func (w *recordingWriter) BeginInventorySyncRun(_ context.Context, run store.BeginSyncRun) (int64, error) {
	w.begin = run
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
	if len(writer.observation.Hosts) != 3 {
		t.Fatalf("Host count = %d, want 3", len(writer.observation.Hosts))
	}
	if len(writer.observation.Devices) != 3 {
		t.Fatalf("Storage Device count = %d, want 3", len(writer.observation.Devices))
	}
	spareDevices := 0
	for _, device := range writer.observation.Devices {
		if device.OSDID == nil {
			spareDevices++
		}
	}
	if spareDevices != 1 {
		t.Fatalf("Storage Devices without an OSD link = %d, want 1", spareDevices)
	}
	if len(writer.observation.Daemons) != 7 {
		t.Fatalf("Ceph Daemon count = %d, want 7", len(writer.observation.Daemons))
	}
	if len(writer.observation.Pools) != 2 {
		t.Fatalf("Pool count = %d, want 2", len(writer.observation.Pools))
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

func TestRunOnceRecordsErrorClassOnFailure(t *testing.T) {
	writer := &recordingWriter{}

	_, err := RunFakeOnce(context.Background(), writer, Options{Scenario: "provider-unauthorized"})
	if err == nil {
		t.Fatal("expected error")
	}
	if writer.failed.RunID != 30 {
		t.Fatalf("failure run ID = %d, want 30", writer.failed.RunID)
	}
	if writer.failed.ErrorClass != "Unauthorized" {
		t.Fatalf("failure error class = %q, want %q", writer.failed.ErrorClass, "Unauthorized")
	}
	if writer.succeeded.RunID != 0 {
		t.Fatalf("unexpected sync success = %+v", writer.succeeded)
	}
}

func TestRunOnceCollectsCephProviderDataAndWritesObservation(t *testing.T) {
	dashboard := dashtest.New(t, dashtest.ModeSuccess)
	provider, err := ceph.New(ceph.Config{
		BaseURL:  dashboard.URL(),
		Username: dashtest.Username,
		Password: dashtest.Password,
	})
	if err != nil {
		t.Fatalf("ceph.New returned error: %v", err)
	}
	writer := &recordingWriter{}
	observedAt := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)

	result, err := RunOnce(context.Background(), writer, provider, Options{
		ProviderName: "ceph",
		ObservedAt:   observedAt,
	})
	if err != nil {
		t.Fatalf("RunOnce returned error: %v", err)
	}
	if result.SnapshotID != 20 {
		t.Fatalf("SnapshotID = %d, want 20", result.SnapshotID)
	}
	if writer.begin.Provider != "ceph" || writer.begin.Scenario != "" {
		t.Fatalf("begin = %+v, want provider ceph with no scenario", writer.begin)
	}
	if writer.observation.Provider != "ceph" {
		t.Fatalf("provider = %q, want ceph", writer.observation.Provider)
	}
	if writer.observation.Scenario != "" {
		t.Fatalf("scenario = %q, want empty", writer.observation.Scenario)
	}
	if writer.observation.Cluster.FSID != dashtest.FSID {
		t.Fatalf("cluster fsid = %q, want %q", writer.observation.Cluster.FSID, dashtest.FSID)
	}
	if writer.observation.Health.Status != "HEALTH_OK" {
		t.Fatalf("health status = %q, want HEALTH_OK", writer.observation.Health.Status)
	}
	if len(writer.observation.OSDs) != 3 {
		t.Fatalf("OSD count = %d, want 3", len(writer.observation.OSDs))
	}
	if len(writer.observation.Hosts) != 2 {
		t.Fatalf("Host count = %d, want 2", len(writer.observation.Hosts))
	}
	if len(writer.observation.Devices) != 3 {
		t.Fatalf("Storage Device count = %d, want 3", len(writer.observation.Devices))
	}
	if len(writer.observation.Daemons) != 5 {
		t.Fatalf("Ceph Daemon count = %d, want 5", len(writer.observation.Daemons))
	}
	if len(writer.observation.Pools) != 2 {
		t.Fatalf("Pool count = %d, want 2", len(writer.observation.Pools))
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

func TestRunOnceRecordsCephErrorClassOnFailure(t *testing.T) {
	dashboard := dashtest.New(t, dashtest.ModeUnauthorized)
	provider, err := ceph.New(ceph.Config{
		BaseURL:  dashboard.URL(),
		Username: dashtest.Username,
		Password: dashtest.Password,
	})
	if err != nil {
		t.Fatalf("ceph.New returned error: %v", err)
	}
	writer := &recordingWriter{}

	_, err = RunOnce(context.Background(), writer, provider, Options{ProviderName: "ceph"})
	if err == nil {
		t.Fatal("expected error")
	}
	if writer.begin.Provider != "ceph" {
		t.Fatalf("begin provider = %q, want ceph", writer.begin.Provider)
	}
	if writer.failed.RunID != 30 {
		t.Fatalf("failure run ID = %d, want 30", writer.failed.RunID)
	}
	if writer.failed.ErrorClass != "Unauthorized" {
		t.Fatalf("failure error class = %q, want %q", writer.failed.ErrorClass, "Unauthorized")
	}
	if writer.succeeded.RunID != 0 {
		t.Fatalf("unexpected sync success = %+v", writer.succeeded)
	}
}

func TestRunOnceRequiresProviderName(t *testing.T) {
	dashboard := dashtest.New(t, dashtest.ModeSuccess)
	provider, err := ceph.New(ceph.Config{
		BaseURL:  dashboard.URL(),
		Username: dashtest.Username,
		Password: dashtest.Password,
	})
	if err != nil {
		t.Fatalf("ceph.New returned error: %v", err)
	}
	if _, err := RunOnce(context.Background(), &recordingWriter{}, provider, Options{}); err == nil {
		t.Fatal("expected error for missing ProviderName")
	}
}

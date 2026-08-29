package atlasagent

import (
	"context"
	"testing"
	"time"

	"github.com/tonymontoya/ceph-atlas/internal/inventory"
	"github.com/tonymontoya/ceph-atlas/internal/providers/ceph"
	"github.com/tonymontoya/ceph-atlas/internal/providers/ceph/dashtest"
)

func TestCollectReadsFullBatchFromDashboard(t *testing.T) {
	dashboard := dashtest.New(t, dashtest.ModeSuccess)
	provider, err := ceph.New(ceph.Config{
		BaseURL:  dashboard.URL(),
		Username: dashtest.Username,
		Password: dashtest.Password,
	})
	if err != nil {
		t.Fatalf("new dashboard provider: %v", err)
	}

	observedAt := time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC)
	batch, err := Collect(context.Background(), provider, observedAt)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}

	if !batch.ObservedAt.Equal(observedAt) {
		t.Fatalf("observedAt = %v, want %v", batch.ObservedAt, observedAt)
	}
	if batch.Cluster != clusterIdentityFixture() {
		t.Fatalf("cluster identity = %+v, want the dashtest fixture identity", batch.Cluster)
	}
	if batch.Health.Status != inventory.HealthOK {
		t.Fatalf("health status = %q, want HEALTH_OK", batch.Health.Status)
	}
	if len(batch.Health.Checks) != 1 || batch.Health.Checks[0].Name != "OSD_DOWN" {
		t.Fatalf("health checks = %+v, want one OSD_DOWN", batch.Health.Checks)
	}
	if len(batch.OSDs) != 3 {
		t.Fatalf("osds = %d, want 3", len(batch.OSDs))
	}
	downAndIn := 0
	for _, osd := range batch.OSDs {
		if !osd.Up && osd.In {
			downAndIn++
		}
	}
	if downAndIn != 1 {
		t.Fatalf("down+in osds = %d, want 1", downAndIn)
	}
	if len(batch.Hosts) != 2 {
		t.Fatalf("hosts = %d, want 2", len(batch.Hosts))
	}
	if len(batch.Devices) != 3 {
		t.Fatalf("devices = %d, want 3 (per-host fan-out)", len(batch.Devices))
	}
	if len(batch.Daemons) != 5 {
		t.Fatalf("daemons = %d, want 5", len(batch.Daemons))
	}
	if len(batch.Pools) != 2 {
		t.Fatalf("pools = %d, want 2", len(batch.Pools))
	}
}

func TestCollectNeverReturnsPartialBatch(t *testing.T) {
	// Any read failure aborts with a zero batch: the dashtest
	// unavailable mode fails every endpoint, and a dashboard that dies
	// mid-collection fails some later read after earlier ones succeeded.
	cases := []struct {
		name string
		mode dashtest.Mode
	}{
		{name: "unavailable from the start", mode: dashtest.ModeUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dashboard := dashtest.New(t, tc.mode)
			provider, err := ceph.New(ceph.Config{
				BaseURL:  dashboard.URL(),
				Username: dashtest.Username,
				Password: dashtest.Password,
			})
			if err != nil {
				t.Fatalf("new dashboard provider: %v", err)
			}

			batch, err := Collect(context.Background(), provider, time.Now().UTC())
			if err == nil {
				t.Fatal("collect against an unavailable dashboard returned no error")
			}
			if batch.ObservedAt != (time.Time{}) || batch.OSDs != nil || batch.Devices != nil {
				t.Fatalf("failed collect returned a partial batch: %+v", batch)
			}
		})
	}

	t.Run("dashboard dies mid collection", func(t *testing.T) {
		dashboard := dashtest.New(t, dashtest.ModeSuccess)
		provider, err := ceph.New(ceph.Config{
			BaseURL:  dashboard.URL(),
			Username: dashtest.Username,
			Password: dashtest.Password,
		})
		if err != nil {
			t.Fatalf("new dashboard provider: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		first, err := provider.ClusterIdentity(ctx)
		if err != nil {
			t.Fatalf("first identity read: %v", err)
		}
		_ = first
		cancel() // subsequent reads fail with the context cancelled

		batch, err := Collect(ctx, provider, time.Now().UTC())
		if err == nil {
			t.Fatal("collect with a cancelled context returned no error")
		}
		if batch.OSDs != nil || batch.Devices != nil {
			t.Fatalf("failed collect returned a partial batch: %+v", batch)
		}
	})
}

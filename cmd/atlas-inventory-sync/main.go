package main

import (
	"context"
	"log"

	"github.com/tonymontoya/ceph-atlas/internal/config"
	"github.com/tonymontoya/ceph-atlas/internal/inventorysync"
	"github.com/tonymontoya/ceph-atlas/internal/store"
)

func main() {
	ctx := context.Background()
	cfg := config.Load()
	if cfg.ProviderMode != "fake" {
		log.Fatalf("atlas-inventory-sync only supports fake provider mode in this scaffold, got %q", cfg.ProviderMode)
	}

	db, err := store.OpenPostgres(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	result, err := inventorysync.RunFakeOnce(ctx, store.NewPostgres(db), inventorysync.Options{
		Scenario: cfg.FakeScenario,
	})
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("persisted fake inventory snapshot cluster_id=%d snapshot_id=%d scenario=%s", result.ClusterID, result.SnapshotID, cfg.FakeScenario)
}

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
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	db, err := store.OpenPostgres(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	writer := store.NewPostgres(db)

	result, err := inventorysync.RunFakeOnce(ctx, writer, inventorysync.Options{Scenario: cfg.FakeScenario})
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("persisted fake inventory snapshot (scenario %q) cluster_id=%d snapshot_id=%d", cfg.FakeScenario, result.ClusterID, result.SnapshotID)
}

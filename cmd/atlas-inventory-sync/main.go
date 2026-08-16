package main

import (
	"context"
	"log"

	"github.com/tonymontoya/ceph-atlas/internal/config"
	"github.com/tonymontoya/ceph-atlas/internal/inventorysync"
	"github.com/tonymontoya/ceph-atlas/internal/providers/ceph"
	"github.com/tonymontoya/ceph-atlas/internal/store"
)

func main() {
	ctx := context.Background()
	cfg := config.Load()

	db, err := store.OpenPostgres(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	writer := store.NewPostgres(db)

	var result store.SaveInventoryResult
	switch cfg.ProviderMode {
	case "fake":
		result, err = inventorysync.RunFakeOnce(ctx, writer, inventorysync.Options{
			Scenario: cfg.FakeScenario,
		})
	case "ceph":
		var provider *ceph.Provider
		provider, err = ceph.New(ceph.Config{
			BaseURL:            cfg.CephDashboardURL,
			Username:           cfg.CephDashboardUser,
			Password:           cfg.CephDashboardPassword,
			ClusterName:        cfg.CephClusterName,
			InsecureSkipVerify: cfg.CephDashboardInsecureTLS,
		})
		if err == nil {
			result, err = inventorysync.RunOnce(ctx, writer, provider, inventorysync.Options{
				ProviderName: "ceph",
			})
		}
	default:
		log.Fatalf("unsupported ATLAS_PROVIDER_MODE %q (supported: fake, ceph)", cfg.ProviderMode)
	}
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("persisted %s inventory snapshot cluster_id=%d snapshot_id=%d", cfg.ProviderMode, result.ClusterID, result.SnapshotID)
}

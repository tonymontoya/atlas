package main

import (
	"context"
	"fmt"
	"log"

	"github.com/tonymontoya/ceph-atlas/internal/config"
	"github.com/tonymontoya/ceph-atlas/internal/inventorysync"
	"github.com/tonymontoya/ceph-atlas/internal/providers"
	"github.com/tonymontoya/ceph-atlas/internal/providers/ceph"
	"github.com/tonymontoya/ceph-atlas/internal/providers/fake"
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

	provider, providerName, err := buildInventoryProvider(cfg)
	if err != nil {
		log.Fatal(err)
	}
	opts := inventorysync.Options{ProviderName: providerName}
	if cfg.ProviderMode == config.ProviderModeFake {
		opts.Scenario = cfg.FakeScenario
	}
	result, err := inventorysync.RunOnce(ctx, writer, provider, opts)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("persisted %s inventory snapshot cluster_id=%d snapshot_id=%d", providerName, result.ClusterID, result.SnapshotID)
}

// buildInventoryProvider maps the typed provider mode onto a constructed
// read provider. config.Load performs the environment-facing validation;
// this switch only selects the adapter, so both arms start from
// pre-validated configuration and the default arm guards direct
// construction in tests.
func buildInventoryProvider(cfg config.Config) (providers.CephReadProvider, string, error) {
	switch cfg.ProviderMode {
	case config.ProviderModeFake:
		return fake.New(fake.DefaultFixtureRoot(), cfg.FakeScenario), string(config.ProviderModeFake), nil
	case config.ProviderModeCeph:
		provider, err := ceph.New(ceph.Config{
			BaseURL:            cfg.CephDashboardURL,
			Username:           cfg.CephDashboardUser,
			Password:           cfg.CephDashboardPassword,
			ClusterName:        cfg.CephClusterName,
			InsecureSkipVerify: cfg.CephDashboardInsecureTLS,
		})
		if err != nil {
			return nil, "", fmt.Errorf("invalid ceph provider configuration: %w", err)
		}
		return provider, string(config.ProviderModeCeph), nil
	default:
		return nil, "", fmt.Errorf("unsupported provider mode %q (supported: fake, ceph)", cfg.ProviderMode)
	}
}

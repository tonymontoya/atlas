package main

import (
	"context"
	"log"

	"github.com/tonymontoya/ceph-atlas/internal/casedetection"
	"github.com/tonymontoya/ceph-atlas/internal/config"
	"github.com/tonymontoya/ceph-atlas/internal/store"
)

func main() {
	ctx := context.Background()
	cfg := config.Load()
	switch cfg.ProviderMode {
	case "fake", "ceph":
	default:
		log.Fatalf("unsupported ATLAS_PROVIDER_MODE %q (supported: fake, ceph)", cfg.ProviderMode)
	}
	if cfg.ProviderMode == "ceph" {
		log.Printf("ATLAS_PROVIDER_MODE=ceph: alert evaluation still reads the fake Prometheus fixtures; a real alert source is not implemented yet")
	}

	db, err := store.OpenPostgres(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	result, err := casedetection.RunFakeOnce(ctx, store.NewPostgres(db), casedetection.Options{
		Scenario: cfg.FakeAlertScenario,
	})
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("evaluated fake alerts scenario=%s alerts_evaluated=%d cases_created=%d", cfg.FakeAlertScenario, result.AlertsEvaluated, result.CasesCreated)
}

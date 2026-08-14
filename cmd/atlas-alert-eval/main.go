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
	if cfg.ProviderMode != "fake" {
		log.Fatalf("atlas-alert-eval only supports fake provider mode in this scaffold, got %q", cfg.ProviderMode)
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

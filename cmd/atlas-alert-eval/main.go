package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tonymontoya/ceph-atlas/internal/casedetection"
	"github.com/tonymontoya/ceph-atlas/internal/config"
	"github.com/tonymontoya/ceph-atlas/internal/providers"
	"github.com/tonymontoya/ceph-atlas/internal/providers/fake"
	"github.com/tonymontoya/ceph-atlas/internal/providers/prometheus"
	"github.com/tonymontoya/ceph-atlas/internal/store"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	provider, providerName, err := buildAlertSource(cfg)
	if err != nil {
		log.Fatal(err)
	}

	db, err := store.OpenPostgres(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	writer := store.NewPostgres(db)
	opts := casedetection.RunOptions{Provider: providerName}
	if cfg.AlertSource == config.AlertSourceFake {
		opts.Scenario = cfg.FakeAlertScenario
	}

	if cfg.AlertEvalInterval <= 0 {
		result, err := casedetection.RunOnce(ctx, writer, provider, opts)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("evaluated %s alerts scenario=%q alerts_evaluated=%d cases_created=%d", providerName, opts.Scenario, result.AlertsEvaluated, result.CasesCreated)
		return
	}

	log.Printf("atlas-alert-eval evaluating %s alerts every %s until shutdown", providerName, cfg.AlertEvalInterval)
	runLoop(ctx, cfg.AlertEvalInterval, func() {
		result, err := casedetection.RunOnce(ctx, writer, provider, opts)
		if err != nil {
			// The failed pass is already recorded on its run row;
			// a transient outage must not stop the loop.
			log.Printf("alert evaluation failed: %v", err)
			return
		}
		log.Printf("evaluated %s alerts scenario=%q alerts_evaluated=%d cases_created=%d", providerName, opts.Scenario, result.AlertsEvaluated, result.CasesCreated)
	})
}

// buildAlertSource maps the typed alert source onto a constructed
// observability provider. config.Load performs the environment-facing
// validation; this switch only selects the adapter, so both arms start
// from pre-validated configuration and the default arm guards direct
// construction in tests.
func buildAlertSource(cfg config.Config) (providers.ObservabilityProvider, string, error) {
	switch cfg.AlertSource {
	case config.AlertSourceFake:
		return fake.NewObservability(fake.DefaultFixtureRoot(), cfg.FakeAlertScenario), string(config.AlertSourceFake), nil
	case config.AlertSourcePrometheus:
		provider, err := prometheus.New(prometheus.Config{
			BaseURL:            cfg.PrometheusURL,
			BearerToken:        cfg.PrometheusBearerToken,
			InsecureSkipVerify: cfg.PrometheusInsecureTLS,
		})
		if err != nil {
			return nil, "", fmt.Errorf("invalid prometheus alert source configuration: %w", err)
		}
		return provider, string(config.AlertSourcePrometheus), nil
	default:
		return nil, "", fmt.Errorf("unsupported alert source %q (supported: fake, prometheus)", cfg.AlertSource)
	}
}

// runLoop evaluates immediately, then on every tick, until ctx ends.
func runLoop(ctx context.Context, interval time.Duration, evaluate func()) {
	evaluate()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			evaluate()
		}
	}
}

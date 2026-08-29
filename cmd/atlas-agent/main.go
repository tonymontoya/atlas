// Command atlas-agent runs the Atlas Agent (ADR-0025, ADR-0026) inside
// a Cluster's trust domain: it enrolls once against Atlas with a
// locally generated key and a one-time Enrollment Credential, persists
// the issued client certificate, then collects full inventory batches
// from the local Ceph Dashboard and pushes them to Atlas over mutual
// TLS. It is read-only by construction: there is no dispatch or
// command surface, and Dashboard credentials never leave the Agent.
//
// One-shot mode (-once) runs a single collect-and-push cycle and exits,
// for deterministic CI runs.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/tonymontoya/ceph-atlas/internal/atlasagent"
	"github.com/tonymontoya/ceph-atlas/internal/config"
	"github.com/tonymontoya/ceph-atlas/internal/providers/ceph"
)

func main() {
	once := flag.Bool("once", false, "run one collect-and-push cycle and exit")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.LoadAgent()
	if err != nil {
		log.Fatal(err)
	}

	provider, err := buildDashboardProvider(cfg)
	if err != nil {
		log.Fatal(err)
	}

	runner := atlasagent.NewRunner(atlasagent.Options{
		Config:   cfg,
		Provider: provider,
		Log:      log.Default(),
	})

	if *once {
		if err := runner.RunOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Fatal(err)
		}
		return
	}

	if err := runner.RunDaemon(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}

// buildDashboardProvider maps the validated agent configuration onto
// the Dashboard read provider running inside the Agent. config.LoadAgent
// performs the environment-facing validation; this only constructs the
// adapter.
func buildDashboardProvider(cfg config.AgentConfig) (*ceph.Provider, error) {
	return ceph.New(ceph.Config{
		BaseURL:            cfg.DashboardURL,
		Username:           cfg.DashboardUser,
		Password:           cfg.DashboardPassword,
		ClusterName:        cfg.DashboardClusterName,
		InsecureSkipVerify: cfg.DashboardInsecureTLS,
	})
}

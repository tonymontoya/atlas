package main

import (
	"context"
	"log"
	"net/http"

	atlasapi "github.com/tonymontoya/ceph-atlas/internal/api"
	"github.com/tonymontoya/ceph-atlas/internal/app"
	"github.com/tonymontoya/ceph-atlas/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	application, err := app.NewFromConfig(context.Background(), cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := application.Close(); err != nil {
			log.Printf("close application: %v", err)
		}
	}()
	server := atlasapi.NewServer(application)

	log.Printf("atlas-api listening on %s with provider mode %s and read source %s", cfg.HTTPAddr, cfg.ProviderMode, cfg.ReadSource)
	if err := http.ListenAndServe(cfg.HTTPAddr, server.Routes()); err != nil {
		log.Fatal(err)
	}
}

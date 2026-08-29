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

	// TLS serving is control-plane configuration: when the serving
	// certificate is configured, the enrollment CA verifies Agent client
	// certificates on the same listener (ADR-0026). Ordinary local
	// development paths stay plain HTTP. With ATLAS_HTTPS_ADDR set the
	// TLS listener runs alongside the plain HTTP one, so Operators keep
	// the published HTTP port while Agents enroll and push over mutual
	// TLS.
	if cfg.APITLSCertPath != "" && cfg.APITLSKeyPath != "" && cfg.HTTPSAddr != "" {
		go serveTLS(cfg.HTTPSAddr, cfg, server)
		serveHTTP(cfg.HTTPAddr, cfg, server)
		return
	}

	if cfg.APITLSCertPath != "" && cfg.APITLSKeyPath != "" {
		serveTLS(cfg.HTTPAddr, cfg, server)
		return
	}

	serveHTTP(cfg.HTTPAddr, cfg, server)
}

func serveHTTP(addr string, cfg config.Config, server *atlasapi.Server) {
	log.Printf("atlas-api listening on %s with provider mode %s and read source %s", addr, cfg.ProviderMode, cfg.ReadSource)
	if err := http.ListenAndServe(addr, server.Routes()); err != nil {
		log.Fatal(err)
	}
}

func serveTLS(addr string, cfg config.Config, server *atlasapi.Server) {
	log.Printf("atlas-api listening on https://%s with provider mode %s and read source %s (client certificates verified by the enrollment CA)", addr, cfg.ProviderMode, cfg.ReadSource)
	err := (&http.Server{
		Addr:      addr,
		Handler:   server.Routes(),
		TLSConfig: server.ClientCertTLSConfig(),
	}).ListenAndServeTLS(cfg.APITLSCertPath, cfg.APITLSKeyPath)
	if err != nil {
		log.Fatal(err)
	}
}

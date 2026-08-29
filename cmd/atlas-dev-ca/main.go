// Command atlas-dev-ca generates the ephemeral dev certificate
// authority for the local dev stack (#43): a fresh CA plus an API
// serving certificate on every bring-up, so the enrolled Agent loop
// runs over real mutual TLS with no real CA anywhere. Dev-only —
// production control planes bring their own CA material (ADR-0026).
package main

import (
	"flag"
	"log"
	"strings"

	"github.com/tonymontoya/ceph-atlas/internal/ca/devca"
)

func main() {
	outDir := flag.String("out-dir", "/atlas-dev-ca", "directory the CA and serving pair are written to")
	serverDNS := flag.String("server-dns", "api", "comma-separated DNS names the serving certificate answers to (in addition to localhost and loopback)")
	flag.Parse()

	material, err := devca.Generate(*outDir, strings.Split(*serverDNS, ","))
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("atlas-dev-ca wrote a fresh dev CA to %s", *outDir)
	log.Printf("enrollment CA:      %s + %s", material.CACertPath, material.CAKeyPath)
	log.Printf("api serving pair:   %s + %s", material.ServerCertPath, material.ServerKeyPath)
}

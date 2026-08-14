package main

import (
	"flag"
	"log"
	"net/http"
	"time"

	"github.com/tonymontoya/ceph-atlas/internal/identity/devissuer"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8090", "listen address for the dev issuer")
	issuerURL := flag.String("issuer", "", "issuer URL placed in token iss claims; must match the API's ATLAS_OIDC_ISSUER. Defaults to http://<addr>")
	audience := flag.String("audience", "atlas-api", "audience placed in token aud claims; must match the API's ATLAS_OIDC_AUDIENCE")
	subject := flag.String("subject", "dev-operator", "subject for the startup token")
	displayName := flag.String("display-name", "Dev Operator", "display name for the startup token")
	ttl := flag.Duration("ttl", 15*time.Minute, "lifetime of issued tokens")
	flag.Parse()

	resolvedIssuer := *issuerURL
	if resolvedIssuer == "" {
		resolvedIssuer = "http://" + *addr
	}

	issuer, err := devissuer.New(resolvedIssuer, *audience)
	if err != nil {
		log.Fatal(err)
	}
	token, err := issuer.IssueToken(*subject, *displayName, *ttl)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("atlas-dev-issuer listening on %s (issuer %s, audience %s)", *addr, resolvedIssuer, *audience)
	log.Printf("jwks:  %s/.well-known/jwks.json", resolvedIssuer)
	log.Printf("token endpoint: POST %s/token?subject=...&displayName=...&ttl=...", resolvedIssuer)
	log.Printf("startup token (subject %s, expires in %s):", *subject, *ttl)
	log.Printf("%s", token)

	if err := http.ListenAndServe(*addr, issuer.Handler()); err != nil {
		log.Fatal(err)
	}
}

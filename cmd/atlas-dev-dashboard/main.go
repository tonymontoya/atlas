// Command atlas-dev-dashboard serves the fake Ceph Dashboard
// (internal/providers/ceph/dashfake) as a standalone HTTP service, so
// the local dev stack can run a real atlas-agent against a
// fixture-backed Dashboard instead of a real cluster. It is dev-only:
// production Agents point at a real Ceph Dashboard.
package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/tonymontoya/ceph-atlas/internal/providers/ceph/dashfake"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address for the fake Dashboard")
	flag.Parse()

	dashboard := dashfake.NewDashboard(dashfake.ModeSuccess)
	log.Printf("atlas-dev-dashboard listening on %s (fsid %s, user %s)", *addr, dashfake.FSID, dashfake.Username)
	if err := http.ListenAndServe(*addr, dashboard.Handler()); err != nil {
		log.Fatal(err)
	}
}

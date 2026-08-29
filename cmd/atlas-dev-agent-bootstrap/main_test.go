package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/tonymontoya/ceph-atlas/internal/providers/ceph/dashfake"
)

// stubAtlas implements just the API surface the bootstrap drives:
// health, the cluster index, deregistration, and registration.
type stubAtlas struct {
	mu sync.Mutex

	healthFailures int
	clusters       []clusterSummary
	deleted        []int64
	bearerSeen     []string
	nextID         int64
}

func (s *stubAtlas) handler(t *testing.T) http.Handler {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.healthFailures > 0 {
			s.healthFailures--
			http.Error(w, "starting", http.StatusServiceUnavailable)
			return
		}
		writeStubJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /api/v1/clusters", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		search := r.URL.Query().Get("q")
		matches := []clusterSummary{}
		for _, cluster := range s.clusters {
			if search == "" || cluster.Name == search || (cluster.FSID != nil && *cluster.FSID == search) {
				matches = append(matches, cluster)
			}
		}
		writeStubJSON(w, http.StatusOK, map[string]any{"clusters": matches, "total": len(matches)})
	})
	mux.HandleFunc("DELETE /api/v1/clusters/{id}", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if bearer := r.Header.Get("Authorization"); bearer != "" {
			s.bearerSeen = append(s.bearerSeen, bearer)
		}
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "bad id", http.StatusBadRequest)
			return
		}
		s.deleted = append(s.deleted, id)
		s.clusters = filterClusters(s.clusters, id)
		writeStubJSON(w, http.StatusOK, map[string]any{"id": id, "deregisteredAt": "2026-08-29T00:00:00Z"})
	})
	mux.HandleFunc("POST /api/v1/clusters", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if bearer := r.Header.Get("Authorization"); bearer != "" {
			s.bearerSeen = append(s.bearerSeen, bearer)
		}
		s.nextID++
		s.clusters = append(s.clusters, clusterSummary{ID: s.nextID, Name: "dev-agent-cluster"})
		writeStubJSON(w, http.StatusCreated, map[string]any{
			"cluster":              clusterSummary{ID: s.nextID, Name: "dev-agent-cluster"},
			"enrollmentCredential": map[string]any{"token": "atl_enroll_stub_token"},
		})
	})
	return mux
}

func newBootstrapHarness(t *testing.T, atlas *stubAtlas) options {
	t.Helper()
	apiServer := httptest.NewServer(atlas.handler(t))
	t.Cleanup(apiServer.Close)

	issuerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeStubJSON(w, http.StatusOK, map[string]string{"token": "dev-stub-token", "tokenType": "Bearer"})
	}))
	t.Cleanup(issuerServer.Close)

	return options{
		apiURL:         apiServer.URL,
		issuerURL:      issuerServer.URL,
		clusterName:    "dev-agent-cluster",
		fsid:           dashfake.FSID,
		credentialPath: filepath.Join(t.TempDir(), "enrollment-credential"),
		waitTimeout:    5 * time.Second,
		interval:       10 * time.Millisecond,
		log:            testBootstrapLogger(t),
	}
}

// TestBootstrapDeregistersStaleRowsAndRegistersFresh drives the
// rerun-stability contract: a previous bring-up's live FSID holder and
// a dormant same-name row are deregistered through the API with the
// dev issuer's bearer token, then a fresh registration's one-time
// credential lands in the output file.
func TestBootstrapDeregistersStaleRowsAndRegistersFresh(t *testing.T) {
	staleFSID := dashfake.FSID
	dormantFSID := ""
	atlas := &stubAtlas{
		clusters: []clusterSummary{
			{ID: 11, FSID: &staleFSID, Name: "previous-bringup"},
			{ID: 12, FSID: &dormantFSID, Name: "dev-agent-cluster"},
		},
	}
	opts := newBootstrapHarness(t, atlas)

	if err := run(context.Background(), opts); err != nil {
		t.Fatalf("run bootstrap: %v", err)
	}

	if len(atlas.deleted) != 2 || atlas.deleted[0] != 11 || atlas.deleted[1] != 12 {
		t.Fatalf("deleted = %v, want the stale FSID holder and the dormant row [11 12]", atlas.deleted)
	}
	for _, bearer := range atlas.bearerSeen {
		if bearer != "Bearer dev-stub-token" {
			t.Fatalf("bearer = %q, want the dev issuer token", bearer)
		}
	}
	credential, err := os.ReadFile(opts.credentialPath)
	if err != nil {
		t.Fatalf("read credential file: %v", err)
	}
	if string(credential) != "atl_enroll_stub_token" {
		t.Fatalf("credential = %q, want the freshly minted one-time token", credential)
	}
}

// TestBootstrapSurvivesSlowStartup keeps bring-ups green when the API
// is still coming up: health and issuer polls retry instead of
// failing the bootstrap.
func TestBootstrapSurvivesSlowStartup(t *testing.T) {
	atlas := &stubAtlas{healthFailures: 2}
	opts := newBootstrapHarness(t, atlas)

	if err := run(context.Background(), opts); err != nil {
		t.Fatalf("run bootstrap against a slow API: %v", err)
	}
	if len(atlas.deleted) != 0 {
		t.Fatalf("deleted = %v, want none for an empty index", atlas.deleted)
	}
	if _, err := os.Stat(opts.credentialPath); err != nil {
		t.Fatalf("credential file after slow startup: %v", err)
	}
}

// TestBootstrapFailsWithoutACredential proves the bootstrap refuses to
// hand the Agent an empty credential file when the API response is
// malformed.
func TestBootstrapFailsWithoutACredential(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			writeStubJSON(w, http.StatusOK, map[string]string{"status": "ok"})
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/clusters" {
			writeStubJSON(w, http.StatusCreated, map[string]any{"cluster": clusterSummary{ID: 1, Name: "dev-agent-cluster"}})
			return
		}
		writeStubJSON(w, http.StatusOK, map[string]any{"clusters": []any{}, "total": 0})
	}))
	t.Cleanup(apiServer.Close)
	issuerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeStubJSON(w, http.StatusOK, map[string]string{"token": "dev-stub-token"})
	}))
	t.Cleanup(issuerServer.Close)

	opts := newBootstrapHarness(t, &stubAtlas{})
	opts.apiURL = apiServer.URL
	opts.issuerURL = issuerServer.URL

	if err := run(context.Background(), opts); err == nil {
		t.Fatal("expected an error when the registration response carries no credential")
	}
	if _, err := os.Stat(opts.credentialPath); !os.IsNotExist(err) {
		t.Fatalf("credential file exists despite failure: %v", err)
	}
}

func writeStubJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func filterClusters(clusters []clusterSummary, id int64) []clusterSummary {
	kept := clusters[:0]
	for _, cluster := range clusters {
		if cluster.ID != id {
			kept = append(kept, cluster)
		}
	}
	return kept
}

func testBootstrapLogger(t *testing.T) *log.Logger {
	t.Helper()
	return log.New(testWriter{t}, "bootstrap: ", 0)
}

type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Logf("%s", p)
	return len(p), nil
}

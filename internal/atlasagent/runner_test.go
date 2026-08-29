package atlasagent

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tonymontoya/ceph-atlas/internal/ca/catest"
	"github.com/tonymontoya/ceph-atlas/internal/config"
	"github.com/tonymontoya/ceph-atlas/internal/providers/ceph"
	"github.com/tonymontoya/ceph-atlas/internal/providers/ceph/dashtest"
)

// stubAtlas is a scripted in-process Atlas for runner tests: a real
// TLS listener whose enroll endpoint mints certificates through the
// test CA and whose observations endpoint serves a configurable status
// sequence. The full-loop test replaces it with the real API server.
type stubAtlas struct {
	t          *testing.T
	authority  *catest.TestCA
	caCertPath string

	mu           sync.Mutex
	enrollCalls  int
	pushCalls    int
	pushStatuses []int // consumed one per push; the last repeats
}

func newStubAtlas(t *testing.T, pushStatuses ...int) *stubAtlas {
	t.Helper()
	authority := catest.New(t)
	certPath, _ := authority.WriteFiles(t)
	if len(pushStatuses) == 0 {
		pushStatuses = []int{http.StatusCreated}
	}
	return &stubAtlas{
		t:            t,
		authority:    authority,
		caCertPath:   certPath,
		pushStatuses: pushStatuses,
	}
}

// start serves the stub over TLS with a CA-signed serving certificate.
func (s *stubAtlas) start() string {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/agent/enroll", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.enrollCalls++
		s.mu.Unlock()
		var request struct {
			CredentialToken string `json:"credentialToken"`
			FSID            string `json:"fsid"`
			CSR             string `json:"csr"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		if request.CredentialToken != "atl_enroll_valid" {
			errorEnvelope(w, http.StatusUnauthorized, "Unauthorized", "credential is invalid or used")
			return
		}
		issued, err := s.authority.Issue([]byte(request.CSR))
		if err != nil {
			http.Error(w, "bad csr", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"cluster": map[string]any{"id": 11, "fsid": request.FSID, "name": "stub-cluster"},
			"certificate": map[string]any{
				"pem":          string(issued.PEMChain),
				"serialNumber": issued.SerialNumber,
				"notAfter":     issued.NotAfter.Format(time.RFC3339),
			},
		})
	})
	mux.HandleFunc("POST /api/v1/agent/observations", func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
			errorEnvelope(w, http.StatusUnauthorized, "Unauthorized", "no client certificate")
			return
		}
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if !strings.Contains(string(body), `"osds"`) {
			errorEnvelope(w, http.StatusBadRequest, "InvalidRequest", "not a batch")
			return
		}
		s.mu.Lock()
		index := s.pushCalls
		if index >= len(s.pushStatuses) {
			index = len(s.pushStatuses) - 1
		}
		status := s.pushStatuses[index]
		s.pushCalls++
		s.mu.Unlock()
		if status != http.StatusCreated {
			errorEnvelope(w, status, "Some", fmt.Sprintf("scripted status %d", status))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"clusterId": 11, "snapshotId": 1000 + index})
	})

	server := httptest.NewUnstartedServer(mux)
	server.TLS = serverTLSConfig(s.t, s.authority)
	server.StartTLS()
	s.t.Cleanup(server.Close)
	return server.URL
}

// newDashboardProvider wires the real Dashboard provider against the
// in-process fake Dashboard (dashtest), never a real Ceph.
func newDashboardProvider(t *testing.T) *ceph.Provider {
	t.Helper()
	dashboard := dashtest.New(t, dashtest.ModeSuccess)
	provider, err := ceph.New(ceph.Config{
		BaseURL:  dashboard.URL(),
		Username: dashtest.Username,
		Password: dashtest.Password,
	})
	if err != nil {
		t.Fatalf("new dashboard provider: %v", err)
	}
	return provider
}

// newTestRunner builds a Runner against the stub Atlas: fast retries
// and a tight collect interval for the daemon loop.
func newTestRunner(t *testing.T, atlas *stubAtlas, atlasURL, stateDir, credential string) *Runner {
	t.Helper()
	return NewRunner(Options{
		Config: config.AgentConfig{
			AtlasURL:             atlasURL,
			AtlasRootCAPath:      atlas.caCertPath,
			EnrollmentCredential: credential,
			StateDir:             stateDir,
			CollectInterval:      10 * time.Millisecond,
			RetryInitial:         time.Millisecond,
			RetryMax:             4 * time.Millisecond,
		},
		Provider: newDashboardProvider(t),
		Log:      testLogger(t),
	})
}

func testLogger(t *testing.T) logger {
	return logWriterFunc(func(format string, args ...any) {
		t.Logf(format, args...)
	})
}

type logWriterFunc func(format string, args ...any)

func (f logWriterFunc) Printf(format string, args ...any) { f(format, args...) }

func TestRunnerRunOnceEnrollsCollectsAndPushes(t *testing.T) {
	atlas := newStubAtlas(t)
	atlasURL := atlas.start()

	runner := newTestRunner(t, atlas, atlasURL, t.TempDir(), "atl_enroll_valid")
	if err := runner.RunOnce(context.Background()); err != nil {
		t.Fatalf("run once: %v", err)
	}

	atlas.mu.Lock()
	defer atlas.mu.Unlock()
	if atlas.enrollCalls != 1 {
		t.Fatalf("enroll calls = %d, want 1", atlas.enrollCalls)
	}
	if atlas.pushCalls != 1 {
		t.Fatalf("push calls = %d, want 1", atlas.pushCalls)
	}
}

func TestRunnerRunOnceReusesStoredEnrollment(t *testing.T) {
	atlas := newStubAtlas(t)
	atlasURL := atlas.start()
	stateDir := t.TempDir()

	first := newTestRunner(t, atlas, atlasURL, stateDir, "atl_enroll_valid")
	if err := first.RunOnce(context.Background()); err != nil {
		t.Fatalf("first run: %v", err)
	}

	// The credential was burned by the first enrollment — a second
	// enrollment attempt would 401. Success with no credential proves
	// the stored certificate was reused.
	second := newTestRunner(t, atlas, atlasURL, stateDir, "")
	if err := second.RunOnce(context.Background()); err != nil {
		t.Fatalf("second run: %v", err)
	}

	atlas.mu.Lock()
	defer atlas.mu.Unlock()
	if atlas.enrollCalls != 1 {
		t.Fatalf("enroll calls = %d, want 1 (stored enrollment reused)", atlas.enrollCalls)
	}
	if atlas.pushCalls != 2 {
		t.Fatalf("push calls = %d, want 2", atlas.pushCalls)
	}
}

func TestRunnerRunOnceRetriesTransientPushFailure(t *testing.T) {
	atlas := newStubAtlas(t, http.StatusInternalServerError, http.StatusCreated)
	atlasURL := atlas.start()

	runner := newTestRunner(t, atlas, atlasURL, t.TempDir(), "atl_enroll_valid")
	if err := runner.RunOnce(context.Background()); err != nil {
		t.Fatalf("run once: %v", err)
	}

	atlas.mu.Lock()
	defer atlas.mu.Unlock()
	if atlas.pushCalls != 2 {
		t.Fatalf("push calls = %d, want 2 (one retried failure)", atlas.pushCalls)
	}
}

func TestRunnerRunOnceFailsFastOnPermanentPushRejection(t *testing.T) {
	atlas := newStubAtlas(t, http.StatusUnauthorized)
	atlasURL := atlas.start()

	runner := newTestRunner(t, atlas, atlasURL, t.TempDir(), "atl_enroll_valid")
	err := runner.RunOnce(context.Background())
	if err == nil || !IsPermanent(err) {
		t.Fatalf("err = %v, want a permanent rejection", err)
	}

	atlas.mu.Lock()
	defer atlas.mu.Unlock()
	if atlas.pushCalls != 1 {
		t.Fatalf("push calls = %d, want 1 (no retry on 401)", atlas.pushCalls)
	}
}

func TestRunnerRequiresCredentialToEnroll(t *testing.T) {
	atlas := newStubAtlas(t)
	atlasURL := atlas.start()

	runner := newTestRunner(t, atlas, atlasURL, t.TempDir(), "")
	err := runner.RunOnce(context.Background())
	if err == nil || !IsPermanent(err) {
		t.Fatalf("err = %v, want a permanent configuration error", err)
	}
	if !strings.Contains(err.Error(), "ATLAS_AGENT_ENROLLMENT_CREDENTIAL") {
		t.Fatalf("error %q does not tell the operator to set the credential", err)
	}

	atlas.mu.Lock()
	defer atlas.mu.Unlock()
	if atlas.enrollCalls != 0 {
		t.Fatalf("enroll calls = %d, want 0", atlas.enrollCalls)
	}
}

// mintExpiredEnrollment stores an enrollment whose certificate already
// expired: hand-signed with the test CA key, since the authority's
// Issue pins a one-year leaf TTL.
func mintExpiredEnrollment(t *testing.T, authority *catest.TestCA) Enrollment {
	t.Helper()
	_, key := catest.NewCSRKeyPair(t)
	caBlock, _ := pem.Decode(authority.CertificatePEM())
	caCertificate, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		t.Fatalf("parse CA certificate: %v", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		t.Fatalf("generate serial: %v", err)
	}
	template := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "atlas-agent", Organization: []string{"ceph-atlas"}},
		NotBefore:             time.Now().Add(-2 * time.Hour),
		NotAfter:              time.Now().Add(-time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, caCertificate, key.Public(), authority.Key)
	if err != nil {
		t.Fatalf("create expired certificate: %v", err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse expired certificate: %v", err)
	}
	chainPEM := append(
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCertificate.Raw})...,
	)
	return Enrollment{ChainPEM: chainPEM, Leaf: leaf, Key: key}
}

func TestRunnerExpiredCertificateWithoutCredentialIsFatal(t *testing.T) {
	atlas := newStubAtlas(t)
	atlasURL := atlas.start()
	stateDir := t.TempDir()

	if err := (StateStore{Dir: stateDir}).Save(mintExpiredEnrollment(t, atlas.authority)); err != nil {
		t.Fatalf("store expired enrollment: %v", err)
	}

	runner := newTestRunner(t, atlas, atlasURL, stateDir, "")
	err := runner.RunOnce(context.Background())
	if err == nil {
		t.Fatal("run with an expired certificate and no credential returned no error")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Fatalf("error %q does not explain the expiry", err)
	}

	atlas.mu.Lock()
	defer atlas.mu.Unlock()
	if atlas.enrollCalls != 0 || atlas.pushCalls != 0 {
		t.Fatalf("calls = enroll %d / push %d, want none", atlas.enrollCalls, atlas.pushCalls)
	}
}

func TestRunnerExpiredCertificateReenrollsWithFreshCredential(t *testing.T) {
	atlas := newStubAtlas(t)
	atlasURL := atlas.start()
	stateDir := t.TempDir()

	if err := (StateStore{Dir: stateDir}).Save(mintExpiredEnrollment(t, atlas.authority)); err != nil {
		t.Fatalf("store expired enrollment: %v", err)
	}

	runner := newTestRunner(t, atlas, atlasURL, stateDir, "atl_enroll_valid")
	if err := runner.RunOnce(context.Background()); err != nil {
		t.Fatalf("run once with fresh credential: %v", err)
	}

	loaded, err := (StateStore{Dir: stateDir}).Load()
	if err != nil {
		t.Fatalf("load replaced enrollment: %v", err)
	}
	if !loaded.Leaf.NotAfter.After(time.Now()) {
		t.Fatal("stored certificate is still the expired one")
	}
}

func TestRunnerDaemonCollectsOnIntervalAndStopsCleanly(t *testing.T) {
	atlas := newStubAtlas(t)
	atlasURL := atlas.start()

	runner := newTestRunner(t, atlas, atlasURL, t.TempDir(), "atl_enroll_valid")

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()
	if err := runner.RunDaemon(ctx); err != nil {
		t.Fatalf("run daemon: %v", err)
	}

	atlas.mu.Lock()
	defer atlas.mu.Unlock()
	if atlas.enrollCalls != 1 {
		t.Fatalf("enroll calls = %d, want 1", atlas.enrollCalls)
	}
	if atlas.pushCalls < 2 {
		t.Fatalf("push calls = %d, want at least 2 over the interval", atlas.pushCalls)
	}
}

func TestRunnerDaemonStopsOnPermanentError(t *testing.T) {
	atlas := newStubAtlas(t, http.StatusConflict)
	atlasURL := atlas.start()

	runner := newTestRunner(t, atlas, atlasURL, t.TempDir(), "atl_enroll_valid")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := runner.RunDaemon(ctx)
	if err == nil || !IsPermanent(err) {
		t.Fatalf("err = %v, want the permanent conflict to stop the daemon", err)
	}
}

func TestRunnerPersistsStateUnderStateDir(t *testing.T) {
	atlas := newStubAtlas(t)
	atlasURL := atlas.start()
	stateDir := t.TempDir()

	runner := newTestRunner(t, atlas, atlasURL, stateDir, "atl_enroll_valid")
	if err := runner.RunOnce(context.Background()); err != nil {
		t.Fatalf("run once: %v", err)
	}

	for _, name := range []string{StateCertificateFile, StateKeyFile} {
		if _, err := os.Stat(filepath.Join(stateDir, name)); err != nil {
			t.Fatalf("state file %s: %v", name, err)
		}
	}
}

// serverTLSConfig builds the stub Atlas TLS shape: a serving
// certificate the enrollment CA signed, and the same client-certificate
// verification posture as the real API server.
func serverTLSConfig(t *testing.T, authority *catest.TestCA) *tls.Config {
	return &tls.Config{
		Certificates: []tls.Certificate{newServerCertificate(t, authority)},
		ClientCAs:    certPoolWith(t, authority),
		ClientAuth:   tls.VerifyClientCertIfGiven,
	}
}

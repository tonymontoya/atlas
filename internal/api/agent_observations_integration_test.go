package api

import (
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tonymontoya/ceph-atlas/internal/ca/catest"
	"github.com/tonymontoya/ceph-atlas/internal/testdb"
)

type observationsHarness struct {
	enrollmentHarness
	apiServer *httptest.Server
}

// newObservationsHarness extends the enrollment harness with a real
// TLS listener: the enrollment CA verifies Agent client certificates
// (Server.ClientCertTLSConfig) while httptest contributes the server
// certificate its own client trusts. Tests exercise the full mutual
// TLS path — r.TLS.PeerCertificates is only populated through a real
// handshake.
func newObservationsHarness(t *testing.T) *observationsHarness {
	t.Helper()
	base := newEnrollmentHarness(t)

	apiServer := httptest.NewUnstartedServer(base.server.Routes())
	apiServer.TLS = base.server.ClientCertTLSConfig()
	apiServer.StartTLS()
	t.Cleanup(apiServer.Close)

	return &observationsHarness{enrollmentHarness: *base, apiServer: apiServer}
}

// cleanupObservationRows removes the clusters this file's FSIDs bind.
// FSID, not registration name: an accepted push renames the cluster row
// to the observed name (upsert semantics shared with the sync path), so
// name-based cleanup would miss renamed rows.
func cleanupObservationRows(t *testing.T, db *sql.DB) {
	t.Helper()
	testdb.DeleteClusters(t, db, "fsid::text LIKE $1", "00000000-0000-4000-8000-0000000008%")
}

// enrollAgent is the full happy-path enrollment through the real API,
// keeping the private key so the issued certificate can drive a mutual
// TLS client. It returns the client certificate and the registered
// cluster's id.
func (h *observationsHarness) enrollAgent(t *testing.T, name, fsid string) (tls.Certificate, int64) {
	t.Helper()
	credentialToken := h.registerCluster(t, name)
	csrPEM, key := catest.NewCSRKeyPair(t)

	response := h.enroll(t, credentialToken, fsid, string(csrPEM))
	if response.Code != http.StatusCreated {
		t.Fatalf("enroll = %d: %s", response.Code, response.Body.String())
	}
	var body struct {
		Cluster struct {
			ID int64 `json:"id"`
		} `json:"cluster"`
		Certificate struct {
			PEM string `json:"pem"`
		} `json:"certificate"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode enrollment response: %v", err)
	}

	chain := catest.ParseChain(t, []byte(body.Certificate.PEM))
	return tls.Certificate{
		Certificate: [][]byte{chain[0].Raw},
		PrivateKey:  key,
	}, body.Cluster.ID
}

// clientFor returns an HTTP client trusting the test server's
// certificate, optionally presenting an Agent client certificate.
func (h *observationsHarness) clientFor(t *testing.T, clientCert *tls.Certificate) *http.Client {
	t.Helper()
	client := h.apiServer.Client()
	transport := client.Transport.(*http.Transport).Clone()
	if clientCert != nil {
		transport.TLSClientConfig.Certificates = []tls.Certificate{*clientCert}
	}
	client.Transport = transport
	return client
}

// push posts one Observation Batch to the agent endpoint. A nil
// clientCert sends no certificate at all.
func (h *observationsHarness) push(t *testing.T, clientCert *tls.Certificate, body map[string]any) (int, string) {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal batch: %v", err)
	}
	response, err := h.clientFor(t, clientCert).Post(h.apiServer.URL+"/api/v1/agent/observations", "application/json", strings.NewReader(string(payload)))
	if err != nil {
		t.Fatalf("push observations: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return response.StatusCode, string(raw)
}

// observationBatch builds a fixture-shaped Observation Batch (the
// normalized shape dev/fixtures/ceph documents) for one collection
// cycle.
func observationBatch(fsid string) map[string]any {
	return map[string]any{
		"observedAt": "2026-08-21T12:00:00Z",
		"cluster": map[string]any{
			"fsid":        fsid,
			"name":        "obs-test-cluster",
			"cephVersion": "18.2.x",
			"type":        "bare-metal",
		},
		"health": map[string]any{
			"status":  "HEALTH_WARN",
			"summary": "1 OSD down",
			"checks": []map[string]any{
				{"name": "OSD_DOWN", "severity": "warning", "summary": "1 OSD down"},
			},
		},
		"osds": []map[string]any{
			{"id": 0, "host": "host-a.example.invalid", "up": true, "in": true, "device": "nvme-obs-a"},
			{"id": 1, "host": "host-b.example.invalid", "up": false, "in": true, "device": "nvme-obs-b"},
		},
		"hosts": []map[string]any{
			{"name": "host-a.example.invalid", "address": "192.0.2.10"},
			{"name": "host-b.example.invalid", "address": "192.0.2.11"},
		},
		"devices": []map[string]any{
			{"host": "host-a.example.invalid", "serial": "nvme-obs-a", "type": "ssd", "path": "/dev/nvme0n1", "health": "ok", "osdId": 0},
			{"host": "host-b.example.invalid", "serial": "nvme-obs-b", "type": "ssd", "path": "/dev/nvme1n1", "health": "error", "osdId": 1},
		},
		"daemons": []map[string]any{
			{"type": "mon", "name": "mon.a", "host": "host-a.example.invalid", "status": "running", "version": "18.2.x"},
			{"type": "osd", "name": "osd.1", "host": "host-b.example.invalid", "status": "stopped"},
		},
		"pools": []map[string]any{
			{"id": 1, "name": ".mgr", "type": "replicated", "size": 3, "minSize": 2},
		},
	}
}

func TestAgentObservationsPushPersistsBatchThroughCertificateCluster(t *testing.T) {
	h := newObservationsHarness(t)
	db := h.db
	cleanupObservationRows(t, db)
	t.Cleanup(func() { cleanupObservationRows(t, db) })

	const fsid = "00000000-0000-4000-8000-000000000801"
	clientCert, clusterID := h.enrollAgent(t, "api-obs-test-a", fsid)

	code, body := h.push(t, &clientCert, observationBatch(fsid))
	if code != http.StatusCreated {
		t.Fatalf("push observations = %d: %s", code, body)
	}
	var receipt struct {
		ClusterID  int64 `json:"clusterId"`
		SnapshotID int64 `json:"snapshotId"`
	}
	if err := json.NewDecoder(strings.NewReader(body)).Decode(&receipt); err != nil {
		t.Fatalf("decode receipt: %v", err)
	}
	if receipt.ClusterID != clusterID {
		t.Fatalf("receipt cluster id = %d, want %d (certificate attribution)", receipt.ClusterID, clusterID)
	}
	if receipt.SnapshotID == 0 {
		t.Fatal("receipt snapshot id = 0, want the persisted snapshot")
	}

	ctx := context.Background()
	var snapshotProvider string
	var snapshotClusterID int64
	var runProvider, runStatus string
	var runScenario sql.NullString
	if err := db.QueryRowContext(ctx, `
		SELECT snapshots.provider, snapshots.cluster_id, runs.provider, runs.status, runs.scenario
		FROM inventory_snapshots AS snapshots
		JOIN inventory_sync_runs AS runs ON runs.snapshot_id = snapshots.id
		WHERE snapshots.id = $1
	`, receipt.SnapshotID).Scan(&snapshotProvider, &snapshotClusterID, &runProvider, &runStatus, &runScenario); err != nil {
		t.Fatalf("query snapshot and run: %v", err)
	}
	if snapshotProvider != "agent" || runProvider != "agent" {
		t.Fatalf("providers = snapshot %q / run %q, want agent/agent", snapshotProvider, runProvider)
	}
	if snapshotClusterID != clusterID {
		t.Fatalf("snapshot cluster id = %d, want %d", snapshotClusterID, clusterID)
	}
	if runStatus != "succeeded" {
		t.Fatalf("run status = %q, want succeeded", runStatus)
	}
	if runScenario.Valid {
		t.Fatalf("run scenario = %q, want none", runScenario.String)
	}

	var osdCount int
	var hasDownOSD bool
	if err := db.QueryRowContext(ctx, `
		SELECT count(*), bool_or(NOT osd_up)
		FROM osd_observations
		WHERE snapshot_id = $1
	`, receipt.SnapshotID).Scan(&osdCount, &hasDownOSD); err != nil {
		t.Fatalf("query osd observations: %v", err)
	}
	if osdCount != 2 || !hasDownOSD {
		t.Fatalf("osd observations = (%d, down=%t), want (2, true)", osdCount, hasDownOSD)
	}
}

func TestAgentObservationsRequireClientCertificate(t *testing.T) {
	h := newObservationsHarness(t)
	db := h.db
	cleanupObservationRows(t, db)
	t.Cleanup(func() { cleanupObservationRows(t, db) })

	code, body := h.push(t, nil, observationBatch("00000000-0000-4000-8000-000000000802"))
	if code != http.StatusUnauthorized {
		t.Fatalf("push without certificate = %d: %s", code, body)
	}
	if class := decodeErrorClassFromString(t, body); class != "Unauthorized" {
		t.Fatalf("error class = %q, want Unauthorized", class)
	}
}

func TestAgentObservationsRejectUnenrolledCertificate(t *testing.T) {
	h := newObservationsHarness(t)
	db := h.db
	cleanupObservationRows(t, db)
	t.Cleanup(func() { cleanupObservationRows(t, db) })

	// Signed by the enrollment CA but never enrolled: the chain
	// verifies at the TLS handshake while the serial maps to no
	// registered Cluster.
	csrPEM, key := catest.NewCSRKeyPair(t)
	issued, err := h.authority.Issue(csrPEM)
	if err != nil {
		t.Fatalf("issue unenrolled certificate: %v", err)
	}
	leaf := catest.ParseChain(t, issued.PEMChain)[0]
	unenrolled := tls.Certificate{Certificate: [][]byte{leaf.Raw}, PrivateKey: key}

	const fsid = "00000000-0000-4000-8000-000000000803"
	code, body := h.push(t, &unenrolled, observationBatch(fsid))
	if code != http.StatusUnauthorized {
		t.Fatalf("push with unenrolled certificate = %d: %s", code, body)
	}
	if class := decodeErrorClassFromString(t, body); class != "Unauthorized" {
		t.Fatalf("error class = %q, want Unauthorized", class)
	}
	if strings.Contains(body, fsid) {
		t.Fatalf("error body %q leaks the requested fsid", body)
	}
}

func TestAgentObservationsRejectFSIDMismatch(t *testing.T) {
	h := newObservationsHarness(t)
	db := h.db
	cleanupObservationRows(t, db)
	t.Cleanup(func() { cleanupObservationRows(t, db) })

	clientCert, _ := h.enrollAgent(t, "api-obs-test-mismatch", "00000000-0000-4000-8000-000000000804")

	// The batch claims a different cluster than the certificate is
	// enrolled for: attribution is by certificate, so the push is a
	// conflict, never a silent re-attribution.
	code, body := h.push(t, &clientCert, observationBatch("00000000-0000-4000-8000-000000000899"))
	if code != http.StatusConflict {
		t.Fatalf("push with mismatched fsid = %d: %s", code, body)
	}
	if class := decodeErrorClassFromString(t, body); class != "Conflict" {
		t.Fatalf("error class = %q, want Conflict", class)
	}
}

func TestAgentObservationsRejectDeregisteredCluster(t *testing.T) {
	h := newObservationsHarness(t)
	db := h.db
	cleanupObservationRows(t, db)
	t.Cleanup(func() { cleanupObservationRows(t, db) })

	const fsid = "00000000-0000-4000-8000-000000000805"
	clientCert, clusterID := h.enrollAgent(t, "api-obs-test-deregistered", fsid)

	response := h.do(t, http.MethodDelete, fmt.Sprintf("/api/v1/clusters/%d", clusterID), nil, true)
	if response.Code != http.StatusOK {
		t.Fatalf("deregister = %d: %s", response.Code, response.Body.String())
	}

	code, body := h.push(t, &clientCert, observationBatch(fsid))
	if code != http.StatusUnauthorized {
		t.Fatalf("push after deregistration = %d: %s", code, body)
	}
	if class := decodeErrorClassFromString(t, body); class != "Unauthorized" {
		t.Fatalf("error class = %q, want Unauthorized", class)
	}
}

func TestAgentObservationsValidateBatchFields(t *testing.T) {
	h := newObservationsHarness(t)
	db := h.db
	cleanupObservationRows(t, db)
	t.Cleanup(func() { cleanupObservationRows(t, db) })

	const fsid = "00000000-0000-4000-8000-000000000806"
	clientCert, _ := h.enrollAgent(t, "api-obs-test-invalid", fsid)

	cases := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "missing observedAt", mutate: func(batch map[string]any) { delete(batch, "observedAt") }},
		{name: "zero observedAt", mutate: func(batch map[string]any) { batch["observedAt"] = time.Time{} }},
		{name: "missing cluster", mutate: func(batch map[string]any) { delete(batch, "cluster") }},
		{name: "missing cluster fsid", mutate: func(batch map[string]any) {
			cluster := batch["cluster"].(map[string]any)
			delete(cluster, "fsid")
		}},
		{name: "missing cluster name", mutate: func(batch map[string]any) {
			cluster := batch["cluster"].(map[string]any)
			delete(cluster, "name")
		}},
		{name: "missing ceph version", mutate: func(batch map[string]any) {
			cluster := batch["cluster"].(map[string]any)
			delete(cluster, "cephVersion")
		}},
		{name: "invalid json", mutate: func(batch map[string]any) { batch["osds"] = "not-an-array" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			batch := observationBatch(fsid)
			tc.mutate(batch)
			code, body := h.push(t, &clientCert, batch)
			if code != http.StatusBadRequest {
				t.Fatalf("push = %d: %s", code, body)
			}
			if class := decodeErrorClassFromString(t, body); class != "InvalidRequest" {
				t.Fatalf("error class = %q, want InvalidRequest", class)
			}
		})
	}
}

func TestAgentObservationsWithoutCAConfiguredIsUnsupported(t *testing.T) {
	// The ordinary write harness configures no CA and no TLS — the
	// default local development shape (ADR-0026: no key material in
	// ordinary local dev paths).
	h := newWriteHarness(t)

	response := h.do(t, http.MethodPost, "/api/v1/agent/observations", observationBatch("00000000-0000-4000-8000-000000000807"), false)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("push without CA = %d, want 422", response.Code)
	}
	if class := decodeErrorClass(t, response); class != "Unsupported" {
		t.Fatalf("error class = %q, want Unsupported", class)
	}
}

func TestClientCertTLSConfigRejectsUntrustedCertificate(t *testing.T) {
	h := newObservationsHarness(t)
	db := h.db
	cleanupObservationRows(t, db)
	t.Cleanup(func() { cleanupObservationRows(t, db) })

	// A certificate from a foreign CA never completes the handshake:
	// the TLS layer, not the handler, rejects it.
	foreign := catest.New(t)
	csrPEM, key := catest.NewCSRKeyPair(t)
	issued, err := foreign.Issue(csrPEM)
	if err != nil {
		t.Fatalf("issue foreign certificate: %v", err)
	}
	leaf := catest.ParseChain(t, issued.PEMChain)[0]
	foreignCert := tls.Certificate{Certificate: [][]byte{leaf.Raw}, PrivateKey: key}

	client := h.clientFor(t, &foreignCert)
	response, err := client.Post(h.apiServer.URL+"/api/v1/agent/observations", "application/json", strings.NewReader(`{}`))
	if err == nil {
		defer func() { _ = response.Body.Close() }()
		t.Fatalf("foreign certificate request = %d, want TLS handshake failure", response.StatusCode)
	}
}

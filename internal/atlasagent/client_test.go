package atlasagent

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tonymontoya/ceph-atlas/internal/ca/catest"
)

// recordingAtlas is a stub Atlas API recording requests and serving
// scripted statuses. It exists for client wire-level tests; the full
// loop test runs the real API server instead.
type recordingAtlas struct {
	t        *testing.T
	requests []recordedRequest
	handler  func(w http.ResponseWriter, r *http.Request, body []byte)
}

type recordedRequest struct {
	method string
	path   string
	body   []byte
}

func (a *recordingAtlas) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		a.t.Fatalf("read request body: %v", err)
	}
	a.requests = append(a.requests, recordedRequest{method: r.Method, path: r.URL.Path, body: body})
	if a.handler == nil {
		w.WriteHeader(http.StatusOK)
		return
	}
	a.handler(w, r, body)
}

func errorEnvelope(w http.ResponseWriter, status int, class, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{"class": class, "message": message},
	})
}

func TestEnrollClientEnrollsAndReturnsEnrollment(t *testing.T) {
	authority := catest.New(t)
	enrollment := mintEnrollment(t, authority)
	csrPEM, key := catest.NewCSRKeyPair(t)

	atlas := &recordingAtlas{t: t}
	atlas.handler = func(w http.ResponseWriter, r *http.Request, body []byte) {
		var request map[string]any
		if err := json.Unmarshal(body, &request); err != nil {
			t.Errorf("enroll body is not json: %v", err)
		}
		if request["credentialToken"] != "atl_enroll_token" || request["fsid"] != "00000000-0000-4000-8000-000000000301" {
			t.Errorf("enroll body = %v, want credential and fsid", request)
		}
		if _, ok := request["csr"].(string); !ok {
			t.Errorf("enroll body has no csr string: %v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"cluster": map[string]any{"id": 7, "fsid": "00000000-0000-4000-8000-000000000301", "name": "test-cluster"},
			"certificate": map[string]any{
				"pem":          string(enrollment.ChainPEM),
				"serialNumber": "abc123",
				"notAfter":     "2027-08-01T00:00:00Z",
			},
		})
	}
	server := httptest.NewServer(atlas)
	t.Cleanup(server.Close)

	client, err := NewEnrollClient(server.URL, TLSOptions{})
	if err != nil {
		t.Fatalf("new enroll client: %v", err)
	}
	stored, receipt, err := client.Enroll(context.Background(), EnrollRequest{
		CredentialToken: "atl_enroll_token",
		FSID:            "00000000-0000-4000-8000-000000000301",
		CSRPEM:          csrPEM,
	}, key)
	if err != nil {
		t.Fatalf("enroll: %v", err)
	}

	if len(atlas.requests) != 1 {
		t.Fatalf("atlas saw %d requests, want 1", len(atlas.requests))
	}
	request := atlas.requests[0]
	if request.method != http.MethodPost || request.path != "/api/v1/agent/enroll" {
		t.Fatalf("request = %s %s, want POST /api/v1/agent/enroll", request.method, request.path)
	}
	if !stored.Leaf.Equal(enrollment.Leaf) {
		t.Fatal("stored leaf differs from the issued leaf")
	}
	if !reflect.DeepEqual(stored.Key.Public(), key.Public()) {
		t.Fatal("stored key is not the locally generated key")
	}
	if receipt.ClusterID != 7 || receipt.ClusterName != "test-cluster" {
		t.Fatalf("receipt = %+v, want cluster 7/test-cluster", receipt)
	}
	if receipt.FSID != "00000000-0000-4000-8000-000000000301" {
		t.Fatalf("receipt fsid = %q", receipt.FSID)
	}
}

func TestEnrollClientClassifiesStatuses(t *testing.T) {
	csrPEM, key := catest.NewCSRKeyPair(t)

	cases := []struct {
		name      string
		status    int
		permanent bool
	}{
		{name: "unauthorized credential is permanent", status: http.StatusUnauthorized, permanent: true},
		{name: "fsid conflict is permanent", status: http.StatusConflict, permanent: true},
		{name: "ca not configured is permanent", status: http.StatusUnprocessableEntity, permanent: true},
		{name: "bad request is permanent", status: http.StatusBadRequest, permanent: true},
		{name: "server error is transient", status: http.StatusInternalServerError, permanent: false},
		{name: "rate limited is transient", status: http.StatusTooManyRequests, permanent: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			atlas := &recordingAtlas{t: t}
			atlas.handler = func(w http.ResponseWriter, r *http.Request, body []byte) {
				errorEnvelope(w, tc.status, "Some", "classifiable message")
			}
			server := httptest.NewServer(atlas)
			t.Cleanup(server.Close)

			client, err := NewEnrollClient(server.URL, TLSOptions{})
			if err != nil {
				t.Fatalf("new enroll client: %v", err)
			}
			_, _, err = client.Enroll(context.Background(), EnrollRequest{
				CredentialToken: "atl_enroll_token",
				FSID:            "00000000-0000-4000-8000-000000000301",
				CSRPEM:          csrPEM,
			}, key)
			if err == nil {
				t.Fatal("enroll returned no error")
			}
			if !strings.Contains(err.Error(), "classifiable message") {
				t.Fatalf("error %q does not carry the server message", err)
			}
			if IsPermanent(err) != tc.permanent {
				t.Fatalf("IsPermanent(%v) = %t, want %t", err, IsPermanent(err), tc.permanent)
			}
		})
	}
}

func TestEnrollClientNetworkErrorIsTransient(t *testing.T) {
	server := httptest.NewServer(&recordingAtlas{t: t})
	client, err := NewEnrollClient(server.URL, TLSOptions{})
	if err != nil {
		t.Fatalf("new enroll client: %v", err)
	}
	server.Close() // every request now fails at the transport layer

	csrPEM, key := catest.NewCSRKeyPair(t)
	_, _, err = client.Enroll(context.Background(), EnrollRequest{
		CredentialToken: "atl_enroll_token",
		FSID:            "00000000-0000-4000-8000-000000000301",
		CSRPEM:          csrPEM,
	}, key)
	if err == nil {
		t.Fatal("enroll against a dead server returned no error")
	}
	if IsPermanent(err) {
		t.Fatalf("network error %v classified permanent", err)
	}
}

// newPushTestServer stands up a TLS Atlas stub whose serving
// certificate the enrollment CA signed, requiring a client certificate
// and asserting it is the returned enrollment's leaf.
func newPushTestServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *catest.TestCA, *Enrollment) {
	t.Helper()
	authority := catest.New(t)
	enrollment := mintEnrollment(t, authority)

	manual := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
			t.Errorf("request carried no client certificate")
			errorEnvelope(w, http.StatusUnauthorized, "Unauthorized", "no client certificate")
			return
		}
		if !r.TLS.PeerCertificates[0].Equal(enrollment.Leaf) {
			t.Errorf("client certificate is not the enrollment leaf")
			errorEnvelope(w, http.StatusUnauthorized, "Unauthorized", "wrong client certificate")
			return
		}
		handler(w, r)
	})

	server := httptest.NewUnstartedServer(manual)
	server.TLS = &tls.Config{
		Certificates: []tls.Certificate{newServerCertificate(t, authority)},
		ClientCAs:    certPoolWith(t, authority),
		ClientAuth:   tls.RequireAnyClientCert,
	}
	server.StartTLS()
	t.Cleanup(server.Close)
	return server, authority, enrollment
}

func certPoolWith(t *testing.T, authority *catest.TestCA) *x509.CertPool {
	t.Helper()
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(authority.CertificatePEM()) {
		t.Fatal("append CA certificate to pool")
	}
	return pool
}

func pushTestBatch() ObservationBatch {
	return ObservationBatch{
		ObservedAt: time.Date(2026, 8, 29, 12, 0, 0, 0, time.UTC),
		Cluster:    clusterIdentityFixture(),
	}
}

func TestPushClientPushesBatchOverMutualTLS(t *testing.T) {
	var seenBody map[string]any
	server, authority, enrollment := newPushTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agent/observations" || r.Method != http.MethodPost {
			t.Errorf("request = %s %s, want POST /api/v1/agent/observations", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&seenBody); err != nil {
			t.Errorf("decode batch: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"clusterId": 7, "snapshotId": 42})
	})

	caPath, _ := authority.WriteFiles(t)
	client, err := NewPushClient(server.URL, enrollment, TLSOptions{RootCAPath: caPath})
	if err != nil {
		t.Fatalf("new push client: %v", err)
	}

	receipt, err := client.Push(context.Background(), pushTestBatch())
	if err != nil {
		t.Fatalf("push: %v", err)
	}
	if receipt.ClusterID != 7 || receipt.SnapshotID != 42 {
		t.Fatalf("receipt = %+v, want cluster 7 snapshot 42", receipt)
	}
	if seenBody == nil || seenBody["observedAt"] == nil {
		t.Fatalf("push body = %v, want observed batch", seenBody)
	}
	cluster, _ := seenBody["cluster"].(map[string]any)
	if cluster == nil || cluster["fsid"] != clusterIdentityFixture().FSID {
		t.Fatalf("push cluster = %v, want identity fsid", cluster)
	}
	// The batch is the whole request: no dashboard credentials, no
	// provider field (ADR-0025 records provider server-side).
	for _, banned := range []string{"provider", "scenario", "password", "username"} {
		if _, present := seenBody[banned]; present {
			t.Fatalf("push body carries forbidden field %q", banned)
		}
	}
}

func TestPushClientRejectsUntrustedAtlas(t *testing.T) {
	// A server certificate from a foreign CA must fail verification,
	// proving the push client actually verifies the Atlas TLS chain.
	foreign := catest.New(t)
	enrollment := mintEnrollment(t, foreign)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{newServerCertificate(t, foreign)}}
	server.StartTLS()
	t.Cleanup(server.Close)

	caPath, _ := catest.New(t).WriteFiles(t)
	client, err := NewPushClient(server.URL, enrollment, TLSOptions{RootCAPath: caPath})
	if err != nil {
		t.Fatalf("new push client: %v", err)
	}
	_, err = client.Push(context.Background(), pushTestBatch())
	if err == nil {
		t.Fatal("push against a foreign-CA Atlas succeeded")
	}
	if IsPermanent(err) {
		t.Fatalf("TLS trust failure %v classified permanent", err)
	}
}

func TestPushClientClassifiesStatuses(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		permanent bool
	}{
		{name: "rejected certificate is permanent", status: http.StatusUnauthorized, permanent: true},
		{name: "fsid mismatch is permanent", status: http.StatusConflict, permanent: true},
		{name: "bad batch is permanent", status: http.StatusBadRequest, permanent: true},
		{name: "server error is transient", status: http.StatusInternalServerError, permanent: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server, authority, enrollment := newPushTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				errorEnvelope(w, tc.status, "Some", "push rejected")
			})
			caPath, _ := authority.WriteFiles(t)
			client, err := NewPushClient(server.URL, enrollment, TLSOptions{RootCAPath: caPath})
			if err != nil {
				t.Fatalf("new push client: %v", err)
			}
			_, err = client.Push(context.Background(), pushTestBatch())
			if err == nil {
				t.Fatal("push returned no error")
			}
			if !strings.Contains(err.Error(), "push rejected") {
				t.Fatalf("error %q does not carry the server message", err)
			}
			if IsPermanent(err) != tc.permanent {
				t.Fatalf("IsPermanent(%v) = %t, want %t", err, IsPermanent(err), tc.permanent)
			}
		})
	}
}

package api

import (
	"context"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tonymontoya/ceph-atlas/internal/app"
	"github.com/tonymontoya/ceph-atlas/internal/ca/catest"
	"github.com/tonymontoya/ceph-atlas/internal/config"
	"github.com/tonymontoya/ceph-atlas/internal/identity/devissuer/devissuertest"
	"github.com/tonymontoya/ceph-atlas/internal/testdb"
)

type enrollmentHarness struct {
	writeHarness
	authority *catest.TestCA
}

// newEnrollmentHarness builds the PostgreSQL write harness plus an
// enrollment CA provisioned through the real control-plane file path
// (ATLAS_ENROLLMENT_CA_CERT_PATH / ATLAS_ENROLLMENT_CA_KEY_PATH). The
// key material is an in-process test CA written to t.TempDir
// (ADR-0026): never real certificates.
func newEnrollmentHarness(t *testing.T) *enrollmentHarness {
	t.Helper()
	ctx := context.Background()

	issuer := devissuertest.Start(t)
	token := issuer.Token(t, "enrollment-operator", "Enrollment Operator")

	db, databaseURL := testdb.Open(t)
	cleanupEnrollmentRows(t, db)
	t.Cleanup(func() { cleanupEnrollmentRows(t, db) })

	authority := catest.New(t)
	certPath, keyPath := authority.WriteFiles(t)

	application, err := app.NewFromConfig(ctx, config.Config{
		DatabaseURL:          databaseURL,
		ReadSource:           "postgres",
		AgentMode:            config.AgentModeDisabled,
		OIDCIssuer:           devissuertest.IssuerURL,
		OIDCAudience:         devissuertest.Audience,
		OIDCJWKSURL:          issuer.JWKSURL(),
		EnrollmentCACertPath: certPath,
		EnrollmentCAKeyPath:  keyPath,
	})
	if err != nil {
		t.Fatalf("new app with enrollment CA: %v", err)
	}
	t.Cleanup(func() { _ = application.Close() })
	return &enrollmentHarness{
		writeHarness: writeHarness{server: NewServer(application), token: token, db: db},
		authority:    authority,
	}
}

func cleanupEnrollmentRows(t *testing.T, db *sql.DB) {
	t.Helper()
	testdb.DeleteClusters(t, db, "name LIKE 'api-enroll-test%'")
}

// registerCluster creates a registration through the Operator API and
// returns the one-time credential token.
func (h *enrollmentHarness) registerCluster(t *testing.T, name string) string {
	t.Helper()
	response := h.do(t, http.MethodPost, "/api/v1/clusters", map[string]string{
		"name":        name,
		"clusterType": "bare-metal",
	}, true)
	if response.Code != http.StatusCreated {
		t.Fatalf("create registration = %d: %s", response.Code, response.Body.String())
	}
	var body struct {
		EnrollmentCredential struct {
			Token string `json:"token"`
		} `json:"enrollmentCredential"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode registration response: %v", err)
	}
	if body.EnrollmentCredential.Token == "" {
		t.Fatal("registration returned no credential token")
	}
	return body.EnrollmentCredential.Token
}

// enroll posts the enrollment handshake without a bearer token: the
// credential in the body is the authentication (ADR-0026). Empty fields
// are omitted so missing-field validation is exercisable.
func (h *enrollmentHarness) enroll(t *testing.T, credentialToken, fsid, csr string) *httptest.ResponseRecorder {
	t.Helper()
	body := map[string]string{}
	if credentialToken != "" {
		body["credentialToken"] = credentialToken
	}
	if fsid != "" {
		body["fsid"] = fsid
	}
	if csr != "" {
		body["csr"] = csr
	}
	return h.do(t, http.MethodPost, "/api/v1/agent/enroll", body, false)
}

func decodeErrorClassFromString(t *testing.T, body string) string {
	t.Helper()
	var parsed struct {
		Error struct {
			Class string `json:"class"`
		} `json:"error"`
	}
	if err := json.NewDecoder(strings.NewReader(body)).Decode(&parsed); err != nil {
		t.Fatalf("decode error body %q: %v", body, err)
	}
	return parsed.Error.Class
}

func TestEnrollAgentExchangesCredentialForCertificateChain(t *testing.T) {
	h := newEnrollmentHarness(t)
	credentialToken := h.registerCluster(t, "api-enroll-test-a")
	fsid := "00000000-0000-4000-8000-000000000601"

	response := h.enroll(t, credentialToken, fsid, string(catest.NewCSR(t)))
	if response.Code != http.StatusCreated {
		t.Fatalf("enroll = %d: %s", response.Code, response.Body.String())
	}

	var body struct {
		Cluster struct {
			FSID *string `json:"fsid"`
		} `json:"cluster"`
		Certificate struct {
			PEM          string `json:"pem"`
			SerialNumber string `json:"serialNumber"`
			Fingerprint  string `json:"fingerprint"`
			CommonName   string `json:"commonName"`
		} `json:"certificate"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode enrollment response: %v", err)
	}
	if body.Cluster.FSID == nil || *body.Cluster.FSID != fsid {
		t.Fatalf("enrolled cluster fsid = %v, want %s", body.Cluster.FSID, fsid)
	}
	if body.Certificate.SerialNumber == "" || body.Certificate.Fingerprint == "" || body.Certificate.CommonName != "atlas-agent" {
		t.Fatalf("certificate handles incomplete: %+v", body.Certificate)
	}

	chain := catest.ParseChain(t, []byte(body.Certificate.PEM))
	if len(chain) != 2 {
		t.Fatalf("chain holds %d certificates, want leaf + CA", len(chain))
	}
	roots := x509.NewCertPool()
	roots.AddCert(catest.ParseChain(t, h.authority.CertificatePEM())[0])
	if _, err := chain[0].Verify(x509.VerifyOptions{
		Roots:     roots,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}); err != nil {
		t.Fatalf("issued certificate does not verify against the enrollment CA: %v", err)
	}

	// The credential burned: replay answers 401.
	replay := h.enroll(t, credentialToken, "00000000-0000-4000-8000-000000000602", string(catest.NewCSR(t)))
	if replay.Code != http.StatusUnauthorized {
		t.Fatalf("replay enroll = %d, want 401", replay.Code)
	}
	if class := decodeErrorClassFromString(t, replay.Body.String()); class != "Unauthorized" {
		t.Fatalf("replay error class = %q, want Unauthorized", class)
	}
}

func TestEnrollAgentWithoutCAConfiguredIsUnsupported(t *testing.T) {
	// The ordinary write harness configures no CA paths — the default
	// local development shape (ADR-0026: no key material in ordinary
	// local dev paths).
	h := newWriteHarness(t)

	response := h.do(t, http.MethodPost, "/api/v1/agent/enroll", map[string]string{
		"credentialToken": "atl_enroll_some_token",
		"fsid":            "00000000-0000-4000-8000-000000000603",
		"csr":             string(catest.NewCSR(t)),
	}, false)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("enroll without CA = %d, want 422", response.Code)
	}
	if class := decodeErrorClass(t, response); class != "Unsupported" {
		t.Fatalf("error class = %q, want Unsupported", class)
	}
}

func TestEnrollAgentRejectsBadInput(t *testing.T) {
	h := newEnrollmentHarness(t)
	credentialToken := h.registerCluster(t, "api-enroll-test-invalid")
	fsid := "00000000-0000-4000-8000-000000000604"

	cases := []struct {
		name      string
		token     string
		fsid      string
		csr       string
		wantCode  int
		wantClass string
	}{
		{name: "unknown credential", token: "atl_enroll_unknown", fsid: fsid, csr: string(catest.NewCSR(t)), wantCode: http.StatusUnauthorized, wantClass: "Unauthorized"},
		{name: "malformed csr", token: credentialToken, fsid: fsid, csr: "not a csr", wantCode: http.StatusBadRequest, wantClass: "InvalidRequest"},
		{name: "missing fsid", token: credentialToken, fsid: "", csr: string(catest.NewCSR(t)), wantCode: http.StatusBadRequest, wantClass: "InvalidRequest"},
		{name: "non-uuid fsid", token: credentialToken, fsid: "not-a-uuid", csr: string(catest.NewCSR(t)), wantCode: http.StatusBadRequest, wantClass: "InvalidRequest"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			response := h.enroll(t, tc.token, tc.fsid, tc.csr)
			if response.Code != tc.wantCode {
				t.Fatalf("enroll = %d (%s), want %d", response.Code, response.Body.String(), tc.wantCode)
			}
			if class := decodeErrorClassFromString(t, response.Body.String()); class != tc.wantClass {
				t.Fatalf("error class = %q, want %q", class, tc.wantClass)
			}
		})
	}
}

func TestEnrollAgentRejectsDuplicateFSID(t *testing.T) {
	h := newEnrollmentHarness(t)
	first := h.registerCluster(t, "api-enroll-test-dup-a")
	second := h.registerCluster(t, "api-enroll-test-dup-b")
	fsid := "00000000-0000-4000-8000-000000000605"

	if response := h.enroll(t, first, fsid, string(catest.NewCSR(t))); response.Code != http.StatusCreated {
		t.Fatalf("first enroll = %d: %s", response.Code, response.Body.String())
	}
	response := h.enroll(t, second, fsid, string(catest.NewCSR(t)))
	if response.Code != http.StatusConflict {
		t.Fatalf("duplicate-fsid enroll = %d, want 409", response.Code)
	}
	if class := decodeErrorClassFromString(t, response.Body.String()); class != "Conflict" {
		t.Fatalf("error class = %q, want Conflict", class)
	}
}

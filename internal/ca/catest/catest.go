// Package catest provides an in-process test certificate authority for
// Agent enrollment tests (ADR-0026). It mirrors the dashtest and
// devissuertest patterns: fresh key material per test, never written
// anywhere outside t.TempDir, and never imported by production code.
package catest

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tonymontoya/ceph-atlas/internal/ca"
)

// New generates a fresh ECDSA P-256 certificate authority. Every test
// gets its own; material never leaves the process.
func New(t *testing.T) *ca.Authority {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate test CA key: %v", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		t.Fatalf("generate test CA serial: %v", err)
	}
	template := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "atlas-test-ca", Organization: []string{"ceph-atlas-test"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, key.Public(), key)
	if err != nil {
		t.Fatalf("create test CA certificate: %v", err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse test CA certificate: %v", err)
	}
	authority, err := ca.New(certificate, key)
	if err != nil {
		t.Fatalf("build test authority: %v", err)
	}
	return authority
}

// NewCSR generates an Ed25519 key pair and returns its PEM certificate
// signing request, standing in for the Agent's locally generated key.
func NewCSR(t *testing.T) []byte {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate test agent key: %v", err)
	}
	template := x509.CertificateRequest{
		Subject: pkix.Name{CommonName: "atlas-agent-test", Organization: []string{"ceph-atlas-test"}},
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, &template, privateKey)
	if err != nil {
		t.Fatalf("create test csr: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})
}

// WriteFiles persists the authority's certificate and key as PEM files
// under t.TempDir, returning their paths — the exact shape of the
// ATLAS_ENROLLMENT_CA_CERT_PATH / ATLAS_ENROLLMENT_CA_KEY_PATH
// control-plane configuration.
func WriteFiles(t *testing.T, authority *ca.Authority) (certPath, keyPath string) {
	t.Helper()
	certPath = filepath.Join(t.TempDir(), "ca.crt")
	keyPath = filepath.Join(t.TempDir(), "ca.key")

	caCertificate := authority.CertificatePEM()
	keyPEM, err := authority.KeyPEM()
	if err != nil {
		t.Fatalf("encode test CA key: %v", err)
	}
	if err := os.WriteFile(certPath, caCertificate, 0o600); err != nil {
		t.Fatalf("write test CA certificate: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("write test CA key: %v", err)
	}
	return certPath, keyPath
}

// ParseChain splits a PEM chain into its certificates (leaf first).
func ParseChain(t *testing.T, chain []byte) []*x509.Certificate {
	t.Helper()
	var certificates []*x509.Certificate
	rest := chain
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		certificate, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			t.Fatalf("parse chain certificate: %v", err)
		}
		certificates = append(certificates, certificate)
	}
	if len(certificates) == 0 {
		t.Fatal("chain holds no certificates")
	}
	return certificates
}

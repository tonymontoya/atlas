package ca_test

import (
	"crypto/ed25519"
	"crypto/x509"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/tonymontoya/ceph-atlas/internal/apperr"
	"github.com/tonymontoya/ceph-atlas/internal/ca"
	"github.com/tonymontoya/ceph-atlas/internal/ca/catest"
)

func assertAppErrClass(t *testing.T, err error, want apperr.Class) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with class %q, got nil", want)
	}
	var appErr apperr.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("error type = %T, want apperr.Error", err)
	}
	if appErr.Class != want {
		t.Fatalf("error class = %q, want %q (message: %s)", appErr.Class, want, appErr.Message)
	}
}

func TestIssueProducesClientCertificateChainedToCA(t *testing.T) {
	authority := catest.New(t)
	issued, err := authority.Issue(catest.NewCSR(t))
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}

	chain := catest.ParseChain(t, issued.PEMChain)
	if len(chain) != 2 {
		t.Fatalf("chain holds %d certificates, want leaf + CA", len(chain))
	}
	leaf := chain[0]
	if leaf.Subject.CommonName != "atlas-agent" {
		t.Fatalf("leaf common name = %q, want atlas-agent", leaf.Subject.CommonName)
	}
	if leaf.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
		t.Fatal("leaf certificate lacks digital signature key usage")
	}
	foundClientAuth := false
	for _, eku := range leaf.ExtKeyUsage {
		if eku == x509.ExtKeyUsageClientAuth {
			foundClientAuth = true
		}
	}
	if !foundClientAuth {
		t.Fatal("leaf certificate lacks clientAuth extended key usage")
	}
	if leaf.IsCA {
		t.Fatal("leaf certificate must not be a CA")
	}
	if err := leaf.CheckSignatureFrom(chain[1]); err != nil {
		t.Fatalf("leaf certificate is not signed by the issuing CA: %v", err)
	}
	if leaf.NotAfter.Sub(leaf.NotBefore) < 364*24*time.Hour {
		t.Fatalf("validity = %s, want a long v0.7 lifetime", leaf.NotAfter.Sub(leaf.NotBefore))
	}
	if issued.SerialNumber == "" || issued.Fingerprint == "" || issued.CommonName != "atlas-agent" {
		t.Fatalf("issued identity handles incomplete: %+v", issued)
	}
}

// TestIssueBindsTheCSRsPublicKey proves the proof-of-possession
// property ADR-0026 exists for: the issued certificate carries exactly
// the public key from the CSR.
func TestIssueBindsTheCSRsPublicKey(t *testing.T) {
	authority := catest.New(t)
	csr, key := catest.NewCSRKeyPair(t)

	issued, err := authority.Issue(csr)
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}
	leaf := catest.ParseChain(t, issued.PEMChain)[0]
	leafKey, ok := leaf.PublicKey.(ed25519.PublicKey)
	if !ok {
		t.Fatalf("leaf public key type = %T, want ed25519", leaf.PublicKey)
	}
	if !leafKey.Equal(key.Public()) {
		t.Fatal("issued certificate does not carry the CSR public key")
	}
}

func TestIssueMintsFreshIdentityPerCertificate(t *testing.T) {
	authority := catest.New(t)
	csr := catest.NewCSR(t)

	first, err := authority.Issue(csr)
	if err != nil {
		t.Fatalf("first Issue returned error: %v", err)
	}
	second, err := authority.Issue(csr)
	if err != nil {
		t.Fatalf("second Issue returned error: %v", err)
	}
	if first.SerialNumber == second.SerialNumber {
		t.Fatal("two issues of the same CSR share a serial number")
	}
	if first.Fingerprint == second.Fingerprint {
		t.Fatal("two issues of the same CSR share a fingerprint")
	}
}

func TestIssueRejectsMalformedCSRs(t *testing.T) {
	authority := catest.New(t)

	_, err := authority.Issue([]byte("not pem"))
	assertAppErrClass(t, err, apperr.InvalidRequest)

	_, err = authority.Issue([]byte("-----BEGIN CERTIFICATE REQUEST-----\bnot-a-csr\n-----END CERTIFICATE REQUEST-----\n"))
	assertAppErrClass(t, err, apperr.InvalidRequest)
}

func TestIssueRejectsTamperedCSR(t *testing.T) {
	authority := catest.New(t)
	csr := catest.NewCSR(t)

	tampered := make([]byte, len(csr))
	copy(tampered, csr)
	tampered[len(tampered)-20] ^= 0xFF

	_, err := authority.Issue(tampered)
	assertAppErrClass(t, err, apperr.InvalidRequest)
}

func TestLoadReadsPEMFilesAndIssues(t *testing.T) {
	authority := catest.New(t)
	certPath, keyPath := authority.WriteFiles(t)

	loaded, err := ca.Load(certPath, keyPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	issued, err := loaded.Issue(catest.NewCSR(t))
	if err != nil {
		t.Fatalf("Issue from loaded authority returned error: %v", err)
	}
	chain := catest.ParseChain(t, issued.PEMChain)
	original := catest.ParseChain(t, authority.CertificatePEM())[0]
	if !chain[1].Equal(original) {
		t.Fatal("loaded authority issues chains to a different CA certificate")
	}
}

func TestLoadRejectsMissingOrMismatchedFiles(t *testing.T) {
	authority := catest.New(t)
	certPath, keyPath := authority.WriteFiles(t)

	if _, err := ca.Load(filepath.Join(filepath.Dir(certPath), "absent.crt"), keyPath); err == nil {
		t.Fatal("expected error for missing certificate file")
	}
	other := catest.New(t)
	otherCertPath, _ := other.WriteFiles(t)
	if _, err := ca.Load(otherCertPath, keyPath); err == nil {
		t.Fatal("expected error for key that does not match the certificate")
	}
}

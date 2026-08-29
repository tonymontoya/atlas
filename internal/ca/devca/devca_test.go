package devca

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/tonymontoya/ceph-atlas/internal/ca"
)

// TestGenerateProducesALoadableAuthorityAndServingPair pins the dev
// stack's real-mTLS contract: the enrollment CA loads through the
// production ca.Load path, and the API serving certificate pairs with
// its key and chains to that CA with server-auth usage and the
// expected names.
func TestGenerateProducesALoadableAuthorityAndServingPair(t *testing.T) {
	dir := t.TempDir()

	material, err := Generate(dir, []string{"api"})
	if err != nil {
		t.Fatalf("generate dev CA: %v", err)
	}

	authority, err := ca.Load(material.CACertPath, material.CAKeyPath)
	if err != nil {
		t.Fatalf("load generated CA through ca.Load: %v", err)
	}
	_ = authority

	pair, err := tls.LoadX509KeyPair(material.ServerCertPath, material.ServerKeyPath)
	if err != nil {
		t.Fatalf("load serving pair: %v", err)
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		t.Fatalf("parse serving leaf: %v", err)
	}

	roots := x509.NewCertPool()
	roots.AddCert(authorityCertificate(t, material.CACertPath))
	if _, err := leaf.Verify(x509.VerifyOptions{DNSName: "api", Roots: roots, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}); err != nil {
		t.Fatalf("verify serving certificate for api: %v", err)
	}
	for _, name := range []string{"api", "localhost"} {
		if !containsName(leaf.DNSNames, name) {
			t.Fatalf("serving certificate DNS names %v lack %s", leaf.DNSNames, name)
		}
	}
	if len(leaf.IPAddresses) == 0 {
		t.Fatal("serving certificate carries no IP SANs, want loopback")
	}
}

func TestGenerateIsEphemeralPerCall(t *testing.T) {
	root := t.TempDir()

	first, err := Generate(filepath.Join(root, "first"), []string{"api"})
	if err != nil {
		t.Fatalf("first generate: %v", err)
	}
	second, err := Generate(filepath.Join(root, "second"), []string{"api"})
	if err != nil {
		t.Fatalf("second generate: %v", err)
	}

	firstAuthority := authorityCertificate(t, first.CACertPath)
	secondAuthority := authorityCertificate(t, second.CACertPath)
	if firstAuthority.Equal(secondAuthority) {
		t.Fatal("two generates produced the same CA, want per-bring-up ephemerality")
	}
}

func authorityCertificate(t *testing.T, path string) *x509.Certificate {
	t.Helper()
	certificate, err := loadCertificatePEM(path)
	if err != nil {
		t.Fatalf("load certificate %s: %v", path, err)
	}
	return certificate
}

func containsName(names []string, want string) bool {
	for _, name := range names {
		if name == want {
			return true
		}
	}
	return false
}

func loadCertificatePEM(path string) (*x509.Certificate, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, errors.New("not a PEM certificate")
	}
	return x509.ParseCertificate(block.Bytes)
}

package atlasagent

import (
	"crypto/x509"
	"encoding/pem"
	"reflect"
	"testing"

	"github.com/tonymontoya/ceph-atlas/internal/ca/catest"
)

func TestNewKeyPairGeneratesFreshKeys(t *testing.T) {
	first, err := NewKeyPair()
	if err != nil {
		t.Fatalf("generate key pair: %v", err)
	}
	second, err := NewKeyPair()
	if err != nil {
		t.Fatalf("generate key pair: %v", err)
	}
	if reflect.DeepEqual(first.Public(), second.Public()) {
		t.Fatal("two generated key pairs are identical")
	}
}

func TestNewCSRPEMIsAcceptableToTheEnrollmentCA(t *testing.T) {
	key, err := NewKeyPair()
	if err != nil {
		t.Fatalf("generate key pair: %v", err)
	}
	csrPEM, err := NewCSRPEM(key)
	if err != nil {
		t.Fatalf("create csr: %v", err)
	}

	block, _ := pem.Decode(csrPEM)
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		t.Fatalf("csr is not a PEM CERTIFICATE REQUEST: %v", csrPEM)
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		t.Fatalf("parse csr: %v", err)
	}
	if err := csr.CheckSignature(); err != nil {
		t.Fatalf("csr signature: %v", err)
	}
	if csr.Subject.CommonName != "atlas-agent" {
		t.Fatalf("csr common name = %q, want atlas-agent", csr.Subject.CommonName)
	}

	// The production CSR must clear the same Issue path the server
	// applies (signature verification plus the key-size floor).
	authority := catest.New(t)
	if _, err := authority.Issue(csrPEM); err != nil {
		t.Fatalf("issue certificate for generated csr: %v", err)
	}
}

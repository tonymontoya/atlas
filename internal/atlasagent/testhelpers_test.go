package atlasagent

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/tonymontoya/ceph-atlas/internal/ca/catest"
	"github.com/tonymontoya/ceph-atlas/internal/fleet"
	"github.com/tonymontoya/ceph-atlas/internal/providers/ceph/dashtest"
)

// newServerCertificate mints a TLS serving certificate for a fake
// Atlas, signed by the enrollment test CA so a push client trusting
// that CA verifies the server — the shape of a control plane whose TLS
// is issued by its internal CA.
func newServerCertificate(t *testing.T, authority *catest.TestCA) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate server key: %v", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		t.Fatalf("generate server serial: %v", err)
	}
	caBlock, _ := pem.Decode(authority.CertificatePEM())
	caCertificate, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		t.Fatalf("parse CA certificate: %v", err)
	}
	template := x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "atlas-api-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, caCertificate, key.Public(), authority.Key)
	if err != nil {
		t.Fatalf("create server certificate: %v", err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse server certificate: %v", err)
	}
	return tls.Certificate{
		Certificate: [][]byte{leaf.Raw, caCertificate.Raw},
		PrivateKey:  key,
	}
}

// mintEnrollment issues a fresh client identity through the test CA.
func mintEnrollment(t *testing.T, authority *catest.TestCA) *Enrollment {
	t.Helper()
	csrPEM, key := catest.NewCSRKeyPair(t)
	issued, err := authority.Issue(csrPEM)
	if err != nil {
		t.Fatalf("issue enrollment: %v", err)
	}
	chain := catest.ParseChain(t, issued.PEMChain)
	return &Enrollment{ChainPEM: issued.PEMChain, Leaf: chain[0], Key: key.(crypto.Signer)}
}

// clusterIdentityFixture is the identity the dashtest dashboard
// reports for its Reef-shaped cluster.
func clusterIdentityFixture() fleet.ClusterIdentity {
	return fleet.ClusterIdentity{
		FSID:        dashtest.FSID,
		Name:        "ceph",
		CephVersion: dashtest.CephVersion,
		Type:        fleet.ClusterTypeBareMetal,
	}
}

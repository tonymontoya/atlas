package atlasagent

import (
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
)

// NewKeyPair generates the Agent's Ed25519 private key. The key is
// generated locally, never sent to Atlas — only its public half
// travels, inside the certificate signing request (ADR-0026). Ed25519
// is the same key family the enrollment tests exercise, and the CA
// accepts it without a size floor.
func NewKeyPair() (crypto.Signer, error) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate agent key pair: %w", err)
	}
	return privateKey, nil
}

// NewCSRPEM builds the PEM certificate signing request for enrollment.
// The server ignores the subject and issues its own, but a conventional
// common name keeps issued certificates recognizable.
func NewCSRPEM(key crypto.Signer) ([]byte, error) {
	template := x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   "atlas-agent",
			Organization: []string{"ceph-atlas"},
		},
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, &template, key)
	if err != nil {
		return nil, fmt.Errorf("create csr: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER}), nil
}

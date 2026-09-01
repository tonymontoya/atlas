// Package ca implements the Atlas internal certificate authority for
// Agent identity (ADR-0026). Enrolled Agents receive client
// certificates chaining to a control-plane-held CA; the CA key material
// lives in control-plane configuration and never in this repository or
// ordinary local development paths. Tests use the in-process test CA in
// internal/ca/catest.
package ca

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"time"

	"github.com/tonymontoya/ceph-atlas/internal/apperr"
)

// certificateTTL bounds the issued client certificates. v0.0.8 keeps
// lifetimes long and renewal manual (ADR-0026): rotation means
// re-enrollment with a fresh credential.
const certificateTTL = 365 * 24 * time.Hour

// certificateCommonName marks every issued certificate as an Atlas
// Agent. A certificate maps to exactly one registered Cluster through
// its recorded serial number, not through subject claims.
const certificateCommonName = "atlas-agent"

// minimumPublicKeyBits rejects legacy key material below today's
// baseline, whatever family the CSR uses.
const minimumPublicKeyBits = 2048

// IssuedCertificate is the outcome of signing one Agent CSR: the PEM
// chain handed to the Agent (leaf first, CA last) plus the identity
// handles Atlas records durably.
type IssuedCertificate struct {
	PEMChain     []byte
	SerialNumber string
	Fingerprint  string
	CommonName   string
	NotBefore    time.Time
	NotAfter     time.Time
}

type Authority struct {
	certificate *x509.Certificate
	key         crypto.Signer
}

// New builds an Authority from an already-parsed CA certificate and
// signing key; tests construct Authorities directly with generated
// material.
func New(certificate *x509.Certificate, key crypto.Signer) (*Authority, error) {
	if certificate == nil || key == nil {
		return nil, errors.New("invalid enrollment CA: certificate and key are required")
	}
	if !certificate.IsCA {
		return nil, errors.New("invalid enrollment CA: certificate is not a CA")
	}
	// Every stdlib public key type (Ed25519, ECDSA, RSA) implements
	// Equal(crypto.PublicKey) bool; the type assertion narrows the
	// stored interface to it.
	comparable, ok := certificate.PublicKey.(interface{ Equal(crypto.PublicKey) bool })
	if !ok || !comparable.Equal(key.Public()) {
		return nil, errors.New("invalid enrollment CA: key does not match the certificate")
	}
	return &Authority{certificate: certificate, key: key}, nil
}

// Load reads the CA certificate and signing key from PEM files. The
// paths are control-plane configuration (ATLAS_ENROLLMENT_CA_CERT_PATH
// and ATLAS_ENROLLMENT_CA_KEY_PATH); ordinary local development paths
// never set them.
func Load(certPath, keyPath string) (*Authority, error) {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, fmt.Errorf("read enrollment CA certificate %s: %w", certPath, err)
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("read enrollment CA key %s: %w", keyPath, err)
	}

	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil || certBlock.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("parse enrollment CA certificate %s: not a PEM certificate", certPath)
	}
	certificate, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse enrollment CA certificate %s: %w", certPath, err)
	}

	key, err := parseKeyPEM(keyPEM)
	if err != nil {
		return nil, fmt.Errorf("parse enrollment CA key %s: %w", keyPath, err)
	}
	return New(certificate, key)
}

func parseKeyPEM(keyPEM []byte) (crypto.Signer, error) {
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return nil, errors.New("not a PEM key")
	}
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		signer, ok := key.(crypto.Signer)
		if !ok {
			return nil, errors.New("PKCS8 key is not a signer")
		}
		return signer, nil
	}
	if key, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	return key, nil
}

// CertificatePEM returns the CA certificate in PEM form, for
// distribution to components that must verify Agent certificates.
func (a *Authority) CertificatePEM() []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: a.certificate.Raw})
}

// SerialNumberHex renders a certificate serial number the way Issue
// records it, so lookups against recorded serials stay consistent with
// issuance.
func SerialNumberHex(certificate *x509.Certificate) string {
	return certificate.SerialNumber.Text(16)
}

// Issue signs one Certificate Signing Request into an Atlas Agent
// client certificate and returns it with the CA appended as a chain.
func (a *Authority) Issue(csrPEM []byte) (IssuedCertificate, error) {
	block, _ := pem.Decode(csrPEM)
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		return IssuedCertificate{}, apperr.Error{Class: apperr.InvalidRequest, Message: "csr must be a PEM CERTIFICATE REQUEST"}
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return IssuedCertificate{}, apperr.Error{Class: apperr.InvalidRequest, Message: fmt.Sprintf("parse csr: %v", err)}
	}
	if err := csr.CheckSignature(); err != nil {
		return IssuedCertificate{}, apperr.Error{Class: apperr.InvalidRequest, Message: fmt.Sprintf("csr signature check failed: %v", err)}
	}
	if key, ok := csr.PublicKey.(*rsa.PublicKey); ok && key.N.BitLen() < minimumPublicKeyBits {
		return IssuedCertificate{}, apperr.Error{Class: apperr.InvalidRequest, Message: fmt.Sprintf("csr RSA key is %d bits, want at least %d", key.N.BitLen(), minimumPublicKeyBits)}
	}
	if key, ok := csr.PublicKey.(*ecdsa.PublicKey); ok && key.Curve.Params().BitSize < 224 {
		return IssuedCertificate{}, apperr.Error{Class: apperr.InvalidRequest, Message: fmt.Sprintf("csr ECDSA curve is %d bits, want at least 224", key.Curve.Params().BitSize)}
	}

	now := time.Now().UTC()
	serial, err := randomSerial()
	if err != nil {
		return IssuedCertificate{}, apperr.Error{Class: apperr.Internal, Message: fmt.Sprintf("generate certificate serial: %v", err)}
	}
	template := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   certificateCommonName,
			Organization: []string{"ceph-atlas"},
		},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(certificateTTL),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, a.certificate, csr.PublicKey, a.key)
	if err != nil {
		return IssuedCertificate{}, apperr.Error{Class: apperr.Internal, Message: fmt.Sprintf("sign certificate: %v", err)}
	}
	issued, err := x509.ParseCertificate(der)
	if err != nil {
		return IssuedCertificate{}, apperr.Error{Class: apperr.Internal, Message: fmt.Sprintf("parse issued certificate: %v", err)}
	}

	chain := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	chain = append(chain, a.CertificatePEM()...)
	digest := sha256.Sum256(der)
	return IssuedCertificate{
		PEMChain:     chain,
		SerialNumber: serial.Text(16),
		Fingerprint:  hex.EncodeToString(digest[:]),
		CommonName:   certificateCommonName,
		NotBefore:    issued.NotBefore,
		NotAfter:     issued.NotAfter,
	}, nil
}

func randomSerial() (*big.Int, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	return new(big.Int).SetBytes(raw), nil
}

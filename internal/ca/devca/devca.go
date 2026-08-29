// Package devca generates the ephemeral dev certificate authority for
// the local dev stack (#43): a fresh CA plus an API serving
// certificate on every bring-up, so the full enrolled loop runs over
// real mutual TLS with no real CA anywhere. Dev-only — production
// control planes bring their own CA material (ADR-0026), and this
// package never enters ordinary local development paths.
package devca

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// Material names the generated files: the enrollment CA pair the API
// loads (ATLAS_ENROLLMENT_CA_CERT_PATH / ATLAS_ENROLLMENT_CA_KEY_PATH)
// and the serving pair its TLS listener loads (ATLAS_API_TLS_CERT_PATH
// / ATLAS_API_TLS_KEY_PATH). Agents trust the CA certificate.
type Material struct {
	CACertPath     string
	CAKeyPath      string
	ServerCertPath string
	ServerKeyPath  string
}

const (
	caCertName     = "ca.crt"
	caKeyName      = "ca.key"
	serverCertName = "api.crt"
	serverKeyName  = "api.key"

	// validity only has to outlast one dev-stack bring-up, but a
	// margin keeps long-running dev stacks from tripping over expired
	// certificates mid-session.
	validity = 30 * 24 * time.Hour
)

// Generate writes a fresh CA plus an API serving certificate signed by
// it into outDir, overwriting any earlier generation — every bring-up
// gets its own trust domain. The serving certificate always carries
// the caller's DNS names plus localhost and loopback IPs so both
// in-network Agents and host-side probes can reach the API.
func Generate(outDir string, serverDNS []string) (Material, error) {
	if outDir == "" {
		return Material{}, fmt.Errorf("devca: output directory is required")
	}
	if err := os.MkdirAll(outDir, 0o700); err != nil {
		return Material{}, fmt.Errorf("devca: create %s: %w", outDir, err)
	}

	now := time.Now().UTC()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return Material{}, fmt.Errorf("devca: generate CA key: %w", err)
	}
	caTemplate := x509.Certificate{
		SerialNumber:          randomSerial(),
		Subject:               pkix.Name{CommonName: "atlas-dev-ca", Organization: []string{"ceph-atlas-dev"}},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(validity),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, &caTemplate, &caTemplate, caKey.Public(), caKey)
	if err != nil {
		return Material{}, fmt.Errorf("devca: create CA certificate: %w", err)
	}
	caCertificate, err := x509.ParseCertificate(caDER)
	if err != nil {
		return Material{}, fmt.Errorf("devca: parse CA certificate: %w", err)
	}

	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return Material{}, fmt.Errorf("devca: generate serving key: %w", err)
	}
	serverTemplate := x509.Certificate{
		SerialNumber: randomSerial(),
		Subject:      pkix.Name{CommonName: "atlas-dev-api", Organization: []string{"ceph-atlas-dev"}},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(validity),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     append([]string{"localhost"}, serverDNS...),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, &serverTemplate, caCertificate, serverKey.Public(), caKey)
	if err != nil {
		return Material{}, fmt.Errorf("devca: create serving certificate: %w", err)
	}

	material := Material{
		CACertPath:     filepath.Join(outDir, caCertName),
		CAKeyPath:      filepath.Join(outDir, caKeyName),
		ServerCertPath: filepath.Join(outDir, serverCertName),
		ServerKeyPath:  filepath.Join(outDir, serverKeyName),
	}
	if err := writePEM(material.CACertPath, "CERTIFICATE", caDER, 0o644); err != nil {
		return Material{}, err
	}
	if err := writeKey(material.CAKeyPath, caKey); err != nil {
		return Material{}, err
	}
	if err := writePEM(material.ServerCertPath, "CERTIFICATE", serverDER, 0o644); err != nil {
		return Material{}, err
	}
	if err := writeKey(material.ServerKeyPath, serverKey); err != nil {
		return Material{}, err
	}
	return material, nil
}

func randomSerial() *big.Int {
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		// crypto/rand never fails on the supported platforms; a
		// fixed serial keeps generation total instead of panicking.
		return big.NewInt(1)
	}
	return serial
}

func writePEM(path, blockType string, der []byte, mode os.FileMode) error {
	encoded := pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der})
	if err := os.WriteFile(path, encoded, mode); err != nil {
		return fmt.Errorf("devca: write %s: %w", path, err)
	}
	return nil
}

func writeKey(path string, key *ecdsa.PrivateKey) error {
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return fmt.Errorf("devca: encode key %s: %w", path, err)
	}
	return writePEM(path, "PRIVATE KEY", der, 0o600)
}

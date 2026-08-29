// Package atlasagent implements the Atlas Agent binary's runtime
// (ADR-0025, ADR-0026): enroll once against Atlas with a locally
// generated key and a one-time Enrollment Credential, persist the
// issued client certificate, then collect full inventory batches
// through the Ceph Dashboard read provider running inside the Agent and
// push them to Atlas over mutual TLS. The Agent is read-only by
// construction in v0.7: it exposes no dispatch or command surface, and
// Dashboard credentials never leave the Agent.
package atlasagent

import (
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
)

// State file names inside the Agent's state directory.
const (
	StateCertificateFile = "certificate.pem"
	StateKeyFile         = "key.pem"
)

// ErrNoEnrollment reports that the state directory holds no enrollment
// yet — the Agent must enroll before it can push observations.
var ErrNoEnrollment = errors.New("no stored enrollment in state directory")

// Enrollment is the Agent's persisted identity: the issued certificate
// chain (leaf first) and the locally generated private key that
// matches the leaf.
type Enrollment struct {
	ChainPEM []byte
	KeyPEM   []byte
	Leaf     *x509.Certificate
	Key      crypto.Signer
}

// StateStore persists the enrollment in a directory: certificate.pem
// holds the PEM chain exactly as issued, key.pem the PKCS#8 private
// key. A half-written pair is corrupt state, not a fresh start: the
// operator re-registers and re-enrolls rather than the Agent silently
// forking its identity.
type StateStore struct {
	Dir string
}

// Save persists the enrollment, creating the state directory with
// owner-only permissions and keeping the key file private.
func (s StateStore) Save(enrollment Enrollment) error {
	if len(enrollment.ChainPEM) == 0 || enrollment.Key == nil {
		return errors.New("cannot save an enrollment without a certificate chain and a private key")
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(enrollment.Key)
	if err != nil {
		return fmt.Errorf("encode private key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})

	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(s.Dir, StateKeyFile), keyPEM, 0o600); err != nil {
		return fmt.Errorf("write private key: %w", err)
	}
	if err := os.WriteFile(filepath.Join(s.Dir, StateCertificateFile), enrollment.ChainPEM, 0o644); err != nil {
		return fmt.Errorf("write certificate chain: %w", err)
	}
	return nil
}

// Load returns the stored enrollment, ErrNoEnrollment when nothing is
// stored, or an error for corrupt state: a half-written pair,
// unparseable material, or a key that does not match the leaf.
func (s StateStore) Load() (*Enrollment, error) {
	certPath := filepath.Join(s.Dir, StateCertificateFile)
	keyPath := filepath.Join(s.Dir, StateKeyFile)

	chainRaw, certErr := os.ReadFile(certPath)
	keyRaw, keyErr := os.ReadFile(keyPath)
	if os.IsNotExist(certErr) && os.IsNotExist(keyErr) {
		return nil, ErrNoEnrollment
	}
	if os.IsNotExist(certErr) || os.IsNotExist(keyErr) {
		return nil, fmt.Errorf("state directory %s is incomplete: it must hold both %s and %s — re-enroll with a fresh enrollment credential to recover", s.Dir, StateCertificateFile, StateKeyFile)
	}
	if certErr != nil {
		return nil, fmt.Errorf("read %s: %w", certPath, certErr)
	}
	if keyErr != nil {
		return nil, fmt.Errorf("read %s: %w", keyPath, keyErr)
	}

	leaf, _, err := parseCertificateChain(chainRaw)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", certPath, err)
	}

	keyBlock, _ := pem.Decode(keyRaw)
	if keyBlock == nil {
		return nil, fmt.Errorf("parse %s: no PEM block found", keyPath)
	}
	parsedKey, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", keyPath, err)
	}
	signer, ok := parsedKey.(crypto.Signer)
	if !ok {
		return nil, fmt.Errorf("parse %s: private key of type %T cannot sign", keyPath, parsedKey)
	}
	if !reflect.DeepEqual(leaf.PublicKey, signer.Public()) {
		return nil, fmt.Errorf("state directory %s holds a private key that does not match the certificate — re-enroll with a fresh enrollment credential to recover", s.Dir)
	}

	return &Enrollment{
		ChainPEM: chainRaw,
		KeyPEM:   pem.EncodeToMemory(keyBlock),
		Leaf:     leaf,
		Key:      signer,
	}, nil
}

// parseCertificateChain decodes the PEM chain and returns the leaf plus
// the full chain, leaf first.
func parseCertificateChain(chainPEM []byte) (*x509.Certificate, []*x509.Certificate, error) {
	var chain []*x509.Certificate
	rest := chainPEM
	for {
		block, more := pem.Decode(rest)
		if block == nil {
			break
		}
		certificate, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, nil, fmt.Errorf("certificate at offset %d: %w", len(chainPEM)-len(rest), err)
		}
		chain = append(chain, certificate)
		rest = more
	}
	if len(chain) == 0 {
		return nil, nil, errors.New("no certificates found")
	}
	return chain[0], chain, nil
}

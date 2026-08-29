package atlasagent

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/tonymontoya/ceph-atlas/internal/ca/catest"
)

// newTestEnrollment mints a real enrollment through the in-process test
// CA: issued chain plus a locally generated key, the same shape the
// enroll client persists.
func newTestEnrollment(t *testing.T) Enrollment {
	t.Helper()
	authority := catest.New(t)
	csrPEM, key := catest.NewCSRKeyPair(t)
	issued, err := authority.Issue(csrPEM)
	if err != nil {
		t.Fatalf("issue test certificate: %v", err)
	}
	chain := catest.ParseChain(t, issued.PEMChain)
	return Enrollment{
		ChainPEM: issued.PEMChain,
		Leaf:     chain[0],
		Key:      key,
	}
}

func TestStateStoreLoadWithoutStateReturnsErrNoEnrollment(t *testing.T) {
	store := StateStore{Dir: t.TempDir()}

	enrollment, err := store.Load()
	if !errors.Is(err, ErrNoEnrollment) {
		t.Fatalf("error = %v, want ErrNoEnrollment", err)
	}
	if enrollment != nil {
		t.Fatalf("enrollment = %+v, want nil", enrollment)
	}
}

func TestStateStoreRoundTrip(t *testing.T) {
	// A fresh nested path proves Save creates the state directory with
	// owner-only permissions; pre-existing directories keep their own.
	store := StateStore{Dir: filepath.Join(t.TempDir(), "nested", "state")}
	enrollment := newTestEnrollment(t)

	if err := store.Save(enrollment); err != nil {
		t.Fatalf("save enrollment: %v", err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("load enrollment: %v", err)
	}
	if !loaded.Leaf.Equal(enrollment.Leaf) {
		t.Fatal("loaded leaf differs from the saved leaf")
	}
	if !reflect.DeepEqual(loaded.Key.Public(), enrollment.Key.Public()) {
		t.Fatal("loaded key differs from the saved key")
	}
	if len(loaded.ChainPEM) == 0 {
		t.Fatal("loaded chain PEM is empty")
	}

	// The persisted chain keeps every block, leaf first.
	blocks := decodeBlocks(t, loaded.ChainPEM)
	if len(blocks) != 2 {
		t.Fatalf("stored chain holds %d blocks, want 2 (leaf + CA)", len(blocks))
	}
	if !blocks[0].Equal(enrollment.Leaf) {
		t.Fatal("first chain block is not the leaf")
	}

	// Key material stays private: 0700 directory, 0600 key file.
	info, err := os.Stat(store.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("state dir mode = %o, want 0700", info.Mode().Perm())
	}
	keyInfo, err := os.Stat(filepath.Join(store.Dir, StateKeyFile))
	if err != nil {
		t.Fatal(err)
	}
	if keyInfo.Mode().Perm() != 0o600 {
		t.Fatalf("key file mode = %o, want 0600", keyInfo.Mode().Perm())
	}
}

func TestStateStoreRejectsPartialState(t *testing.T) {
	enrollment := newTestEnrollment(t)

	cases := []struct {
		name  string
		setup func(t *testing.T, dir string)
	}{
		{
			name: "certificate without key",
			setup: func(t *testing.T, dir string) {
				if err := os.WriteFile(filepath.Join(dir, StateCertificateFile), enrollment.ChainPEM, 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "key without certificate",
			setup: func(t *testing.T, dir string) {
				keyDER, err := x509.MarshalPKCS8PrivateKey(enrollment.Key)
				if err != nil {
					t.Fatal(err)
				}
				keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
				if err := os.WriteFile(filepath.Join(dir, StateKeyFile), keyPEM, 0o600); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			tc.setup(t, dir)

			_, err := (StateStore{Dir: dir}).Load()
			if err == nil || errors.Is(err, ErrNoEnrollment) {
				t.Fatalf("error = %v, want a corrupt-state error", err)
			}
		})
	}
}

func TestStateStoreRejectsCorruptState(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, StateCertificateFile), []byte("not a pem"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, StateKeyFile), []byte("not a pem"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := (StateStore{Dir: dir}).Load()
	if err == nil || errors.Is(err, ErrNoEnrollment) {
		t.Fatalf("error = %v, want a corrupt-state error", err)
	}
}

func TestStateStoreRejectsMismatchedKey(t *testing.T) {
	enrollment := newTestEnrollment(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, StateCertificateFile), enrollment.ChainPEM, 0o644); err != nil {
		t.Fatal(err)
	}
	_, otherKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(otherKey)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(filepath.Join(dir, StateKeyFile), keyPEM, 0o600); err != nil {
		t.Fatal(err)
	}

	_, err = (StateStore{Dir: dir}).Load()
	if err == nil || errors.Is(err, ErrNoEnrollment) {
		t.Fatalf("error = %v, want a key mismatch error", err)
	}
}

func decodeBlocks(t *testing.T, pemBytes []byte) []*x509.Certificate {
	t.Helper()
	var certificates []*x509.Certificate
	rest := pemBytes
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		certificate, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			t.Fatalf("parse certificate: %v", err)
		}
		certificates = append(certificates, certificate)
	}
	return certificates
}

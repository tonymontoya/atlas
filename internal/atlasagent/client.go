package atlasagent

import (
	"bytes"
	"context"
	"crypto"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// requestTimeout bounds every Atlas request the Agent makes, matching
// the Dashboard provider's client timeout.
const requestTimeout = 30 * time.Second

// maxErrorBody bounds how much of an Atlas error response the Agent
// reads into logs.
const maxErrorBody = 4 * 1024

// permanentError marks failures retrying cannot fix: HTTP 4xx answers
// from Atlas. Everything else — transport errors, 429, 5xx — is
// transient and backs off.
type permanentError struct{ err error }

func (e *permanentError) Error() string { return e.err.Error() }
func (e *permanentError) Unwrap() error { return e.err }

// IsPermanent reports whether err is a failure the Agent must surface
// to the operator instead of retrying: an invalid or expired
// enrollment credential, an FSID conflict, a rejected certificate, or
// a malformed request.
func IsPermanent(err error) bool {
	var permanent *permanentError
	return errors.As(err, &permanent)
}

// TLSOptions configures how the Agent verifies the Atlas API's serving
// certificate: an optional PEM CA bundle for control planes issued by
// a private CA, or the dev-only insecure escape hatch.
type TLSOptions struct {
	RootCAPath         string
	InsecureSkipVerify bool
}

// EnrollRequest is the enrollment handshake payload: the one-time
// credential from the Cluster's registration, the FSID from the
// Agent's first Dashboard read, and the PEM certificate signing
// request carrying the locally generated public key.
type EnrollRequest struct {
	CredentialToken string
	FSID            string
	CSRPEM          []byte
}

// EnrollReceipt summarizes the accepted enrollment for logging.
type EnrollReceipt struct {
	ClusterID   int64
	ClusterName string
	FSID        string
}

type enrollResponseWire struct {
	Cluster struct {
		ID   int64   `json:"id"`
		FSID *string `json:"fsid"`
		Name string  `json:"name"`
	} `json:"cluster"`
	Certificate struct {
		PEM          string    `json:"pem"`
		SerialNumber string    `json:"serialNumber"`
		NotAfter     time.Time `json:"notAfter"`
	} `json:"certificate"`
}

type errorEnvelopeWire struct {
	Error struct {
		Class   string `json:"class"`
		Message string `json:"message"`
	} `json:"error"`
}

// EnrollClient posts the enrollment handshake. It carries no client
// certificate — the one-time credential in the body is the
// authentication (ADR-0026).
type EnrollClient struct {
	baseURL string
	client  *http.Client
}

func NewEnrollClient(baseURL string, opts TLSOptions) (*EnrollClient, error) {
	client, err := newHTTPClient(opts, nil)
	if err != nil {
		return nil, err
	}
	return &EnrollClient{baseURL: strings.TrimRight(baseURL, "/"), client: client}, nil
}

// Enroll submits the CSR and returns the enrollment to persist plus a
// receipt for logging. The returned Enrollment keeps the locally
// generated key alongside the issued chain.
func (c *EnrollClient) Enroll(ctx context.Context, request EnrollRequest, key crypto.Signer) (Enrollment, EnrollReceipt, error) {
	body, err := json.Marshal(map[string]string{
		"credentialToken": request.CredentialToken,
		"fsid":            request.FSID,
		"csr":             string(request.CSRPEM),
	})
	if err != nil {
		return Enrollment{}, EnrollReceipt{}, fmt.Errorf("encode enrollment request: %w", err)
	}

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/agent/enroll", bytes.NewReader(body))
	if err != nil {
		return Enrollment{}, EnrollReceipt{}, err
	}
	httpRequest.Header.Set("Content-Type", "application/json")

	response, err := c.client.Do(httpRequest)
	if err != nil {
		return Enrollment{}, EnrollReceipt{}, fmt.Errorf("post enrollment: %w", err)
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode < 200 || response.StatusCode > 299 {
		return Enrollment{}, EnrollReceipt{}, statusError("enroll", response)
	}

	var parsed enrollResponseWire
	if err := json.NewDecoder(response.Body).Decode(&parsed); err != nil {
		return Enrollment{}, EnrollReceipt{}, fmt.Errorf("decode enrollment response: %w", err)
	}
	leaf, _, err := parseCertificateChain([]byte(parsed.Certificate.PEM))
	if err != nil {
		return Enrollment{}, EnrollReceipt{}, fmt.Errorf("issued certificate chain: %w", err)
	}
	if parsed.Cluster.FSID == nil || *parsed.Cluster.FSID != request.FSID {
		return Enrollment{}, EnrollReceipt{}, fmt.Errorf("enrolled cluster fsid %v does not match the requested fsid", parsed.Cluster.FSID)
	}
	return Enrollment{
			ChainPEM: []byte(parsed.Certificate.PEM),
			Leaf:     leaf,
			Key:      key,
		}, EnrollReceipt{
			ClusterID:   parsed.Cluster.ID,
			ClusterName: parsed.Cluster.Name,
			FSID:        *parsed.Cluster.FSID,
		}, nil
}

// PushReceipt acknowledges one persisted Observation Batch.
type PushReceipt struct {
	ClusterID  int64
	SnapshotID int64
}

type pushReceiptWire struct {
	ClusterID  int64 `json:"clusterId"`
	SnapshotID int64 `json:"snapshotId"`
}

// PushClient posts Observation Batches over mutual TLS, presenting the
// enrolled client certificate. Atlas attributes the batch to the
// certificate's cluster, never to payload claims (ADR-0025).
type PushClient struct {
	baseURL string
	client  *http.Client
}

func NewPushClient(baseURL string, enrollment *Enrollment, opts TLSOptions) (*PushClient, error) {
	if enrollment == nil {
		return nil, errors.New("push client requires an enrollment")
	}
	rawChain := certificateBlocks(enrollment.ChainPEM)
	if len(rawChain) == 0 {
		return nil, errors.New("enrollment holds no certificate chain")
	}
	cert := tls.Certificate{
		Certificate: rawChain,
		PrivateKey:  enrollment.Key,
	}
	client, err := newHTTPClient(opts, &cert)
	if err != nil {
		return nil, err
	}
	return &PushClient{baseURL: strings.TrimRight(baseURL, "/"), client: client}, nil
}

// Push sends one whole batch. There is no partial path: the batch is
// marshaled completely before the request, and any failure leaves the
// next retry free to collect a fresh, consistent batch.
func (c *PushClient) Push(ctx context.Context, batch ObservationBatch) (PushReceipt, error) {
	body, err := json.Marshal(batch)
	if err != nil {
		return PushReceipt{}, fmt.Errorf("encode observation batch: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/agent/observations", bytes.NewReader(body))
	if err != nil {
		return PushReceipt{}, err
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := c.client.Do(request)
	if err != nil {
		return PushReceipt{}, fmt.Errorf("post observation batch: %w", err)
	}
	defer func() { _ = response.Body.Close() }()

	if response.StatusCode < 200 || response.StatusCode > 299 {
		return PushReceipt{}, statusError("push observations", response)
	}

	var receipt pushReceiptWire
	if err := json.NewDecoder(response.Body).Decode(&receipt); err != nil {
		return PushReceipt{}, fmt.Errorf("decode push receipt: %w", err)
	}
	return PushReceipt{ClusterID: receipt.ClusterID, SnapshotID: receipt.SnapshotID}, nil
}

// statusError turns a non-2xx Atlas answer into a classified error:
// 4xx answers are permanent, everything else is transient and retried
// with backoff. The server's message rides along.
func statusError(operation string, response *http.Response) error {
	raw, _ := io.ReadAll(io.LimitReader(response.Body, maxErrorBody))
	message := strings.TrimSpace(string(raw))
	var envelope errorEnvelopeWire
	if err := json.Unmarshal(raw, &envelope); err == nil && envelope.Error.Message != "" {
		message = fmt.Sprintf("%s (%s)", envelope.Error.Message, envelope.Error.Class)
	}
	err := fmt.Errorf("%s: atlas answered %d: %s", operation, response.StatusCode, message)
	if response.StatusCode >= 400 && response.StatusCode < 500 && response.StatusCode != http.StatusTooManyRequests {
		return &permanentError{err: err}
	}
	return err
}

// newHTTPClient builds the Agent's HTTP client: a 30-second timeout
// and optional Atlas TLS verification, plus the enrolled client
// certificate when one already exists.
func newHTTPClient(opts TLSOptions, clientCert *tls.Certificate) (*http.Client, error) {
	tlsConfig := &tls.Config{}
	if opts.InsecureSkipVerify {
		tlsConfig.InsecureSkipVerify = true
	} else if opts.RootCAPath != "" {
		caPEM, err := os.ReadFile(opts.RootCAPath)
		if err != nil {
			return nil, fmt.Errorf("read Atlas CA bundle %s: %w", opts.RootCAPath, err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caPEM) {
			return nil, fmt.Errorf("Atlas CA bundle %s holds no certificates", opts.RootCAPath)
		}
		tlsConfig.RootCAs = pool
	}
	if clientCert != nil {
		tlsConfig.Certificates = []tls.Certificate{*clientCert}
	}
	return &http.Client{
		Timeout:   requestTimeout,
		Transport: &http.Transport{TLSClientConfig: tlsConfig},
	}, nil
}

// certificateBlocks extracts each CERTIFICATE block's DER from a PEM
// chain, leaf first.
func certificateBlocks(chainPEM []byte) [][]byte {
	var blocks [][]byte
	rest := chainPEM
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		blocks = append(blocks, block.Bytes)
	}
	return blocks
}

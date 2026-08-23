package api

import (
	"crypto/tls"
	"crypto/x509"
)

// ClientCertTLSConfig returns the TLS configuration API serving uses
// when TLS is enabled: the enrollment CA (ADR-0026) becomes the client
// certificate authority, and client certificates are requested but not
// required (VerifyClientCertIfGiven) so ordinary bearer-token and
// public endpoints keep working on the same listener. Per-endpoint
// enforcement stays with the handlers — the agent observations endpoint
// requires a verified, enrolled certificate. Nil means no enrollment CA
// is configured, so no client certificate can verify at all.
func (s *Server) ClientCertTLSConfig() *tls.Config {
	if s.app.EnrollmentCA == nil {
		return nil
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(s.app.EnrollmentCA.CertificatePEM())
	return &tls.Config{
		ClientCAs:  pool,
		ClientAuth: tls.VerifyClientCertIfGiven,
	}
}

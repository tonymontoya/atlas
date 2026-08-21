package api

import (
	"net/http"
	"time"

	"github.com/tonymontoya/ceph-atlas/internal/apperr"
	"github.com/tonymontoya/ceph-atlas/internal/ca"
	"github.com/tonymontoya/ceph-atlas/internal/fleet"
	"github.com/tonymontoya/ceph-atlas/internal/store"
)

func agentEnrollmentUnsupported() apperr.Error {
	return apperr.Error{
		Class:   apperr.Unsupported,
		Message: "agent enrollment requires the enrollment CA and postgres read source",
	}
}

// enrollAgent runs the enrollment handshake (ADR-0026): an Atlas Agent
// presents a CSR, the one-time Enrollment Credential, and its
// self-reported FSID; Atlas burns the credential, issues a client
// certificate from the internal CA, and binds the FSID. The credential
// itself is the authentication — the endpoint deliberately sits outside
// requireIdentity.
func (s *Server) enrollAgent(w http.ResponseWriter, r *http.Request) {
	if s.app.ClusterRegistrations == nil || s.app.EnrollmentCA == nil {
		writeError(w, agentEnrollmentUnsupported())
		return
	}
	var request struct {
		CredentialToken string `json:"credentialToken"`
		FSID            string `json:"fsid"`
		CSR             string `json:"csr"`
	}
	if err := decodeJSONBody(w, r, &request); err != nil {
		return
	}
	if request.CredentialToken == "" {
		writeError(w, invalidRequest("credentialToken is required"))
		return
	}
	if request.FSID == "" {
		writeError(w, invalidRequest("fsid is required"))
		return
	}
	if request.CSR == "" {
		writeError(w, invalidRequest("csr is required"))
		return
	}

	authority := s.app.EnrollmentCA
	result, err := s.app.ClusterRegistrations.EnrollAgent(r.Context(), store.EnrollAgentInput{
		CredentialToken: request.CredentialToken,
		FSID:            request.FSID,
	}, func() (ca.IssuedCertificate, error) {
		return authority.Issue([]byte(request.CSR))
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, agentEnrollmentResponse{
		Cluster:     result.Cluster,
		Certificate: issuedCertificateResponseFrom(result.Certificate),
	})
}

type issuedCertificateResponse struct {
	PEMChain     string    `json:"pem"`
	SerialNumber string    `json:"serialNumber"`
	Fingerprint  string    `json:"fingerprint"`
	CommonName   string    `json:"commonName"`
	NotBefore    time.Time `json:"notBefore"`
	NotAfter     time.Time `json:"notAfter"`
}

func issuedCertificateResponseFrom(certificate ca.IssuedCertificate) issuedCertificateResponse {
	return issuedCertificateResponse{
		PEMChain:     string(certificate.PEMChain),
		SerialNumber: certificate.SerialNumber,
		Fingerprint:  certificate.Fingerprint,
		CommonName:   certificate.CommonName,
		NotBefore:    certificate.NotBefore,
		NotAfter:     certificate.NotAfter,
	}
}

type agentEnrollmentResponse struct {
	Cluster     fleet.ClusterRegistration `json:"cluster"`
	Certificate issuedCertificateResponse `json:"certificate"`
}

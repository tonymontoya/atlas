package api

import (
	"encoding/json"
	"net/http"

	"github.com/tonymontoya/ceph-atlas/internal/cases"
	"github.com/tonymontoya/ceph-atlas/internal/identity"
	"github.com/tonymontoya/ceph-atlas/internal/providers"
	"github.com/tonymontoya/ceph-atlas/internal/store"
)

type invalidRequestError struct {
	Message string
}

func (e invalidRequestError) Error() string {
	return e.Message
}

func caseWritesUnsupported() providers.ProviderError {
	return providers.ProviderError{
		Class:   providers.ErrorUnsupported,
		Message: "case writes require postgres read source",
	}
}

func (s *Server) createCase(w http.ResponseWriter, r *http.Request) {
	if s.app.CaseWrites == nil {
		writeError(w, caseWritesUnsupported())
		return
	}
	var request struct {
		Title       string `json:"title"`
		Summary     string `json:"summary"`
		Severity    string `json:"severity"`
		ClusterFSID string `json:"clusterFsid"`
	}
	if err := decodeJSONBody(w, r, &request); err != nil {
		return
	}
	if request.Title == "" {
		writeError(w, invalidRequestError{Message: "title is required"})
		return
	}
	if request.Summary == "" {
		writeError(w, invalidRequestError{Message: "summary is required"})
		return
	}
	if _, err := cases.ParseCaseSeverity(request.Severity); err != nil {
		writeError(w, invalidRequestError{Message: err.Error()})
		return
	}
	if request.ClusterFSID != "" && !store.IsUUIDShape(request.ClusterFSID) {
		writeError(w, invalidRequestError{Message: "clusterFsid must be a UUID"})
		return
	}

	actor, _ := identity.FromContext(r.Context())
	created, err := s.app.CaseWrites.CreateManualCase(r.Context(), store.ManualCaseInput{
		Title:       request.Title,
		Summary:     request.Summary,
		Severity:    request.Severity,
		ClusterFSID: request.ClusterFSID,
		Actor:       store.Actor{Subject: actor.Subject, DisplayName: actor.DisplayName},
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) transitionCase(w http.ResponseWriter, r *http.Request) {
	if s.app.CaseWrites == nil {
		writeError(w, caseWritesUnsupported())
		return
	}
	id, ok := parseCaseID(r)
	if !ok {
		writeError(w, caseNotFound())
		return
	}
	var request struct {
		Status string `json:"status"`
	}
	if err := decodeJSONBody(w, r, &request); err != nil {
		return
	}
	target, err := cases.ParseCaseStatus(request.Status)
	if err != nil {
		writeError(w, invalidRequestError{Message: err.Error()})
		return
	}

	actor, _ := identity.FromContext(r.Context())
	updated, err := s.app.CaseWrites.TransitionCase(r.Context(), store.CaseTransitionInput{
		CaseID: id,
		To:     target,
		Actor:  store.Actor{Subject: actor.Subject, DisplayName: actor.DisplayName},
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) assignCase(w http.ResponseWriter, r *http.Request) {
	if s.app.CaseWrites == nil {
		writeError(w, caseWritesUnsupported())
		return
	}
	id, ok := parseCaseID(r)
	if !ok {
		writeError(w, caseNotFound())
		return
	}
	var request struct {
		Assignee            string `json:"assignee"`
		AssigneeDisplayName string `json:"assigneeDisplayName"`
	}
	if err := decodeJSONBody(w, r, &request); err != nil {
		return
	}
	if request.Assignee != "" && request.AssigneeDisplayName == "" {
		writeError(w, invalidRequestError{Message: "assigneeDisplayName is required when assigning"})
		return
	}
	if request.Assignee == "" && request.AssigneeDisplayName != "" {
		writeError(w, invalidRequestError{Message: "assigneeDisplayName must be empty when unassigning"})
		return
	}

	actor, _ := identity.FromContext(r.Context())
	updated, err := s.app.CaseWrites.AssignCase(r.Context(), store.CaseAssignmentInput{
		CaseID:              id,
		Assignee:            request.Assignee,
		AssigneeDisplayName: request.AssigneeDisplayName,
		Actor:               store.Actor{Subject: actor.Subject, DisplayName: actor.DisplayName},
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) addCaseNote(w http.ResponseWriter, r *http.Request) {
	if s.app.CaseWrites == nil {
		writeError(w, caseWritesUnsupported())
		return
	}
	id, ok := parseCaseID(r)
	if !ok {
		writeError(w, caseNotFound())
		return
	}
	var request struct {
		Body string `json:"body"`
	}
	if err := decodeJSONBody(w, r, &request); err != nil {
		return
	}
	if request.Body == "" {
		writeError(w, invalidRequestError{Message: "body is required"})
		return
	}

	actor, _ := identity.FromContext(r.Context())
	note, err := s.app.CaseWrites.AddCaseNote(r.Context(), store.CaseNoteInput{
		CaseID: id,
		Body:   request.Body,
		Actor:  store.Actor{Subject: actor.Subject, DisplayName: actor.DisplayName},
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, note)
}

func (s *Server) caseNotes(w http.ResponseWriter, r *http.Request) {
	if s.app.CaseWrites == nil {
		writeError(w, caseReadsUnsupported())
		return
	}
	id, ok := parseCaseID(r)
	if !ok {
		writeError(w, caseNotFound())
		return
	}
	notes, err := s.app.CaseWrites.ListCaseNotes(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, notes)
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, target any) error {
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(target); err != nil {
		writeError(w, invalidRequestError{Message: "request body must be valid JSON: " + err.Error()})
		return err
	}
	return nil
}

package api

import (
	"net/http"
	"time"

	"github.com/tonymontoya/ceph-atlas/internal/identity"
	"github.com/tonymontoya/ceph-atlas/internal/providers"
	"github.com/tonymontoya/ceph-atlas/internal/store"
	"github.com/tonymontoya/ceph-atlas/internal/workflows"
)

func workflowWritesUnsupported() providers.ProviderError {
	return providers.ProviderError{
		Class:   providers.ErrorUnsupported,
		Message: "workflow writes require postgres read source",
	}
}

type workflowInstancePayload struct {
	ID              int64      `json:"id"`
	CaseID          int64      `json:"caseId"`
	WorkflowID      string     `json:"workflowId"`
	WorkflowVersion int        `json:"workflowVersion"`
	State           string     `json:"state"`
	CurrentStep     *string    `json:"currentStep"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
	FinishedAt      *time.Time `json:"finishedAt"`
}

func newWorkflowInstancePayload(instance store.WorkflowInstance) workflowInstancePayload {
	return workflowInstancePayload{
		ID:              instance.ID,
		CaseID:          instance.CaseID,
		WorkflowID:      instance.DefinitionID,
		WorkflowVersion: instance.DefinitionVersion,
		State:           string(instance.State),
		CurrentStep:     instance.CurrentStep,
		CreatedAt:       instance.CreatedAt,
		UpdatedAt:       instance.UpdatedAt,
		FinishedAt:      instance.FinishedAt,
	}
}

// attachWorkflow attaches a Workflow to a Case: the definition is resolved
// in the code registry (ADR-0017) before any store call, and only Job steps
// become Job rows — Gates and Tasks are not Jobs (ADR-0019).
func (s *Server) attachWorkflow(w http.ResponseWriter, r *http.Request) {
	if s.app.WorkflowWrites == nil || s.app.WorkflowRegistry == nil {
		writeError(w, workflowWritesUnsupported())
		return
	}
	id, ok := parseCaseID(r)
	if !ok {
		writeError(w, caseNotFound())
		return
	}
	var request struct {
		WorkflowID      string `json:"workflowId"`
		WorkflowVersion int    `json:"workflowVersion"`
	}
	if err := decodeJSONBody(w, r, &request); err != nil {
		return
	}
	if request.WorkflowID == "" {
		writeError(w, invalidRequestError{Message: "workflowId is required"})
		return
	}
	if request.WorkflowVersion < 1 {
		writeError(w, invalidRequestError{Message: "workflowVersion must be a positive integer"})
		return
	}

	definition, ok := s.app.WorkflowRegistry.Get(request.WorkflowID, request.WorkflowVersion)
	if !ok {
		writeError(w, providers.ProviderError{
			Class:   providers.ErrorNotFound,
			Message: "workflow definition not found",
		})
		return
	}

	jobs := make([]store.WorkflowJobInput, 0, len(definition.Steps))
	for _, step := range definition.Steps {
		if jobStep, ok := step.(workflows.JobStep); ok {
			jobs = append(jobs, store.WorkflowJobInput{
				StepID:        jobStep.ID,
				OperationType: jobStep.OperationType,
				MaxAttempts:   jobStep.Retry.MaxAttempts,
			})
		}
	}

	actor, _ := identity.FromContext(r.Context())
	instance, err := s.app.WorkflowWrites.CreateWorkflowInstance(r.Context(), store.CreateWorkflowInstanceInput{
		CaseID:            id,
		DefinitionID:      definition.ID,
		DefinitionVersion: definition.Version,
		Jobs:              jobs,
		Actor:             store.Actor{Subject: actor.Subject, DisplayName: actor.DisplayName},
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, newWorkflowInstancePayload(instance))
}

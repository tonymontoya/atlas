package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/tonymontoya/ceph-atlas/internal/providers"
	"github.com/tonymontoya/ceph-atlas/internal/store"
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

type approvalPayload struct {
	ID                 int64     `json:"id"`
	WorkflowInstanceID int64     `json:"workflowInstanceId"`
	GateID             string    `json:"gateId"`
	ApproverID         string    `json:"approverId"`
	ApproverName       string    `json:"approverDisplayName"`
	Reason             *string   `json:"reason"`
	CreatedAt          time.Time `json:"createdAt"`
}

func newApprovalPayload(record store.ApprovalRecord) approvalPayload {
	return approvalPayload{
		ID:                 record.ID,
		WorkflowInstanceID: record.WorkflowInstanceID,
		GateID:             record.GateID,
		ApproverID:         record.Approver.Subject,
		ApproverName:       record.Approver.DisplayName,
		Reason:             record.Reason,
		CreatedAt:          record.CreatedAt,
	}
}

// attachWorkflow attaches a Workflow to a Case through the Workflow
// Lifecycle (ADR-0017, ADR-0019): the definition is resolved in the
// code registry, only Job steps become Job rows, and the instance
// advances to a pause at the definition's first Approval Gate. The
// handler decodes and validates the request; the choreography lives in
// internal/workflowdispatch.
func (s *Server) attachWorkflow(w http.ResponseWriter, r *http.Request) {
	if s.app.WorkflowLifecycle == nil {
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

	instance, err := s.app.WorkflowLifecycle.Attach(r.Context(), actorFromRequest(r), id, request.WorkflowID, request.WorkflowVersion)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, newWorkflowInstancePayload(instance))
}

// listCaseWorkflows returns a Case's Workflow Instances in creation order.
func (s *Server) listCaseWorkflows(w http.ResponseWriter, r *http.Request) {
	if s.app.WorkflowLifecycle == nil {
		writeError(w, workflowWritesUnsupported())
		return
	}
	id, ok := parseCaseID(r)
	if !ok {
		writeError(w, caseNotFound())
		return
	}
	instances, err := s.app.WorkflowReads.ListWorkflowInstancesByCase(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	payload := make([]workflowInstancePayload, 0, len(instances))
	for _, instance := range instances {
		payload = append(payload, newWorkflowInstancePayload(instance))
	}
	writeJSON(w, http.StatusOK, payload)
}

// parseWorkflowInstanceID reads the instance id path segment; a missing
// or invalid id is a 404 like any unknown instance.
func parseWorkflowInstanceID(r *http.Request) (int64, bool) {
	instanceID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || instanceID <= 0 {
		return 0, false
	}
	return instanceID, true
}

func workflowInstanceNotFound() providers.ProviderError {
	return providers.ProviderError{
		Class:   providers.ErrorNotFound,
		Message: "workflow instance not found",
	}
}

// approveWorkflowGate records the durable, immutable Approval for the gate
// the instance is paused at and advances the instance past it through the
// Workflow Lifecycle (ADR-0020, ADR-0021). A second approval of an
// already-passed gate is an idempotent no-op returning the existing record
// without touching the instance — the handler answers 201 for the advancing
// first approval and 200 for the replay.
func (s *Server) approveWorkflowGate(w http.ResponseWriter, r *http.Request) {
	if s.app.WorkflowLifecycle == nil {
		writeError(w, workflowWritesUnsupported())
		return
	}
	instanceID, ok := parseWorkflowInstanceID(r)
	if !ok {
		writeError(w, workflowInstanceNotFound())
		return
	}
	var request struct {
		GateID string `json:"gateId"`
		Reason string `json:"reason"`
	}
	if err := decodeJSONBody(w, r, &request); err != nil {
		return
	}
	if request.GateID == "" {
		writeError(w, invalidRequestError{Message: "gateId is required"})
		return
	}

	result, err := s.app.WorkflowLifecycle.ApproveGate(r.Context(), actorFromRequest(r), instanceID, request.GateID, request.Reason)
	if err != nil {
		writeError(w, err)
		return
	}
	status := http.StatusOK
	if result.Advanced {
		status = http.StatusCreated
	}
	writeJSON(w, status, newApprovalPayload(result.Record))
}

// completeWorkflowTask records the durable, immutable Task completion
// for the human Task the instance is paused at and resumes the instance
// past it through the Workflow Lifecycle (ADR-0019). A second
// completion of an already-passed task is an idempotent no-op returning
// the existing record without touching the instance — the handler
// answers 201 for the resuming first completion and 200 for the replay.
func (s *Server) completeWorkflowTask(w http.ResponseWriter, r *http.Request) {
	if s.app.WorkflowLifecycle == nil {
		writeError(w, workflowWritesUnsupported())
		return
	}
	instanceID, ok := parseWorkflowInstanceID(r)
	if !ok {
		writeError(w, workflowInstanceNotFound())
		return
	}
	var request struct {
		TaskID string `json:"taskId"`
		Note   string `json:"note"`
	}
	if err := decodeJSONBody(w, r, &request); err != nil {
		return
	}
	if request.TaskID == "" {
		writeError(w, invalidRequestError{Message: "taskId is required"})
		return
	}

	result, err := s.app.WorkflowLifecycle.CompleteTask(r.Context(), actorFromRequest(r), instanceID, request.TaskID, request.Note)
	if err != nil {
		writeError(w, err)
		return
	}
	status := http.StatusOK
	if result.Advanced {
		status = http.StatusCreated
	}
	writeJSON(w, status, newTaskCompletionPayload(result.Record))
}

type taskCompletionPayload struct {
	ID                 int64     `json:"id"`
	WorkflowInstanceID int64     `json:"workflowInstanceId"`
	TaskID             string    `json:"taskId"`
	OperatorID         string    `json:"operatorId"`
	OperatorName       string    `json:"operatorDisplayName"`
	Note               *string   `json:"note"`
	CreatedAt          time.Time `json:"createdAt"`
}

func newTaskCompletionPayload(record store.TaskCompletionRecord) taskCompletionPayload {
	return taskCompletionPayload{
		ID:                 record.ID,
		WorkflowInstanceID: record.WorkflowInstanceID,
		TaskID:             record.TaskID,
		OperatorID:         record.Operator.Subject,
		OperatorName:       record.Operator.DisplayName,
		Note:               record.Note,
		CreatedAt:          record.CreatedAt,
	}
}

type workflowJobPayload struct {
	ID                 int64      `json:"id"`
	WorkflowInstanceID int64      `json:"workflowInstanceId"`
	Position           int        `json:"position"`
	StepID             string     `json:"stepId"`
	OperationType      string     `json:"operationType"`
	State              string     `json:"state"`
	Attempt            int        `json:"attempt"`
	MaxAttempts        int        `json:"maxAttempts"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
	FinishedAt         *time.Time `json:"finishedAt"`
}

// listWorkflowJobs returns a Workflow Instance's Jobs in definition
// order, exposing per-Job progress for the Case detail view.
func (s *Server) listWorkflowJobs(w http.ResponseWriter, r *http.Request) {
	if s.app.WorkflowLifecycle == nil {
		writeError(w, workflowWritesUnsupported())
		return
	}
	instanceID, ok := parseWorkflowInstanceID(r)
	if !ok {
		writeError(w, workflowInstanceNotFound())
		return
	}
	if _, err := s.app.WorkflowReads.GetWorkflowInstance(r.Context(), instanceID); err != nil {
		writeError(w, err)
		return
	}
	jobs, err := s.app.WorkflowReads.ListWorkflowJobs(r.Context(), instanceID)
	if err != nil {
		writeError(w, err)
		return
	}
	payload := make([]workflowJobPayload, 0, len(jobs))
	for _, job := range jobs {
		payload = append(payload, workflowJobPayload{
			ID:                 job.ID,
			WorkflowInstanceID: job.WorkflowInstanceID,
			Position:           job.Position,
			StepID:             job.StepID,
			OperationType:      job.OperationType,
			State:              string(job.State),
			Attempt:            job.Attempt,
			MaxAttempts:        job.MaxAttempts,
			CreatedAt:          job.CreatedAt,
			UpdatedAt:          job.UpdatedAt,
			FinishedAt:         job.FinishedAt,
		})
	}
	writeJSON(w, http.StatusOK, payload)
}

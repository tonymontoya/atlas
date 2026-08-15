package api

import (
	"context"
	"net/http"
	"strconv"
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

// attachWorkflow attaches a Workflow to a Case: the definition is resolved
// in the code registry (ADR-0017) before any store call, and only Job steps
// become Job rows — Gates and Tasks are not Jobs (ADR-0019). The instance
// then advances from pending through running to a pause at the definition's
// first Approval Gate, with the Atlas system actor attributing the
// advancement Timeline Events; without a gate the instance rests pending.
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

	instance, err = s.advanceToFirstGate(r.Context(), instance)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, newWorkflowInstancePayload(instance))
}

// advanceToFirstGate moves a freshly attached instance from pending through
// running to waiting_for_approval at the definition's first Approval Gate.
// Gateless definitions leave the instance where it is.
func (s *Server) advanceToFirstGate(ctx context.Context, instance store.WorkflowInstance) (store.WorkflowInstance, error) {
	var gateID string
	definition, ok := s.app.WorkflowRegistry.Get(instance.DefinitionID, instance.DefinitionVersion)
	if ok {
		for _, step := range definition.Steps {
			if gate, isGate := step.(workflows.ApprovalGate); isGate {
				gateID = gate.ID
				break
			}
		}
	}
	if gateID == "" || instance.State != workflows.InstancePending {
		return instance, nil
	}

	if _, err := s.app.WorkflowWrites.TransitionWorkflowInstance(ctx, store.WorkflowInstanceTransitionInput{
		InstanceID: instance.ID,
		To:         workflows.InstanceRunning,
	}); err != nil {
		return store.WorkflowInstance{}, err
	}
	return s.app.WorkflowWrites.TransitionWorkflowInstance(ctx, store.WorkflowInstanceTransitionInput{
		InstanceID: instance.ID,
		To:         workflows.InstanceWaitingForApproval,
		AtStep:     gateID,
	})
}

// listCaseWorkflows returns a Case's Workflow Instances in creation order.
func (s *Server) listCaseWorkflows(w http.ResponseWriter, r *http.Request) {
	if s.app.WorkflowReads == nil {
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
// the instance is paused at and advances the instance past it (ADR-0020,
// ADR-0021). A second approval of an already-passed gate is an idempotent
// no-op returning the existing record without touching the instance.
func (s *Server) approveWorkflowGate(w http.ResponseWriter, r *http.Request) {
	if s.app.WorkflowWrites == nil {
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

	actor, _ := identity.FromContext(r.Context())
	approval, err := s.app.WorkflowWrites.RecordApproval(r.Context(), store.RecordApprovalInput{
		InstanceID: instanceID,
		GateID:     request.GateID,
		Approver:   store.Actor{Subject: actor.Subject, DisplayName: actor.DisplayName},
		Reason:     request.Reason,
	})
	if err != nil {
		writeError(w, err)
		return
	}

	// The record already existing means the gate was passed before; the
	// approval authorizes nothing further (ADR-0020). Otherwise this call
	// recorded the first approval, and the instance is still paused at the
	// gate: advance it.
	instance, err := s.app.WorkflowWrites.GetWorkflowInstance(r.Context(), instanceID)
	if err != nil {
		writeError(w, err)
		return
	}
	if instance.State == workflows.InstanceWaitingForApproval && instance.CurrentStep != nil && *instance.CurrentStep == request.GateID {
		if _, err := s.app.WorkflowWrites.TransitionWorkflowInstance(r.Context(), store.WorkflowInstanceTransitionInput{
			InstanceID: instanceID,
			To:         workflows.InstanceRunning,
			Actor:      &store.Actor{Subject: actor.Subject, DisplayName: actor.DisplayName},
		}); err != nil {
			writeError(w, err)
			return
		}
		// With the gate passed, the fake agent loop (ADR-0022) drives the
		// instance through its Jobs synchronously; without a dispatcher
		// the instance rests running with pending Jobs.
		if s.app.WorkflowDispatch != nil {
			if _, err := s.app.WorkflowDispatch.Run(r.Context(), instanceID); err != nil {
				writeError(w, err)
				return
			}
		}
		writeJSON(w, http.StatusCreated, newApprovalPayload(approval))
		return
	}
	writeJSON(w, http.StatusOK, newApprovalPayload(approval))
}

// completeWorkflowTask records the durable, immutable Task completion
// for the human Task the instance is paused at and resumes the instance
// past it (ADR-0019). A second completion of an already-passed task is
// an idempotent no-op returning the existing record without touching
// the instance.
func (s *Server) completeWorkflowTask(w http.ResponseWriter, r *http.Request) {
	if s.app.WorkflowWrites == nil {
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

	actor, _ := identity.FromContext(r.Context())
	completion, err := s.app.WorkflowWrites.RecordTaskCompletion(r.Context(), store.RecordTaskCompletionInput{
		InstanceID: instanceID,
		TaskID:     request.TaskID,
		Operator:   store.Actor{Subject: actor.Subject, DisplayName: actor.DisplayName},
		Note:       request.Note,
	})
	if err != nil {
		writeError(w, err)
		return
	}

	// The record already existing means the task was passed before; the
	// completion authorizes nothing further. Otherwise this call
	// recorded the first completion, and the instance is still paused at
	// the task: resume it.
	instance, err := s.app.WorkflowWrites.GetWorkflowInstance(r.Context(), instanceID)
	if err != nil {
		writeError(w, err)
		return
	}
	if instance.State == workflows.InstanceWaitingForOperator && instance.CurrentStep != nil && *instance.CurrentStep == request.TaskID {
		if _, err := s.app.WorkflowWrites.TransitionWorkflowInstance(r.Context(), store.WorkflowInstanceTransitionInput{
			InstanceID: instanceID,
			To:         workflows.InstanceRunning,
			Actor:      &store.Actor{Subject: actor.Subject, DisplayName: actor.DisplayName},
		}); err != nil {
			writeError(w, err)
			return
		}
		// With the task done, the fake agent loop (ADR-0022) drives the
		// instance through its remaining Jobs synchronously; without a
		// dispatcher the instance rests running with pending Jobs.
		if s.app.WorkflowDispatch != nil {
			if _, err := s.app.WorkflowDispatch.Run(r.Context(), instanceID); err != nil {
				writeError(w, err)
				return
			}
		}
		writeJSON(w, http.StatusCreated, newTaskCompletionPayload(completion))
		return
	}
	writeJSON(w, http.StatusOK, newTaskCompletionPayload(completion))
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
	if s.app.WorkflowReads == nil {
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

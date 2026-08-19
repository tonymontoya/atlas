package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/tonymontoya/ceph-atlas/internal/actor"
	"github.com/tonymontoya/ceph-atlas/internal/cases"
	"github.com/tonymontoya/ceph-atlas/internal/workflows"
)

const workflowInstanceColumns = "id, case_id, definition_id, definition_version, current_step, state, created_at, updated_at, finished_at"

const workflowJobColumns = "id, workflow_instance_id, position, step_id, operation_type, state, attempt, max_attempts, created_at, updated_at, finished_at"

// WorkflowJobInput is one Job specification to persist for a Workflow
// Instance, derived from the Workflow definition's ordered Job steps.
type WorkflowJobInput struct {
	StepID        string
	OperationType string
	MaxAttempts   int
}

// CreateWorkflowInstanceInput creates a pending Workflow Instance with
// Job rows for every definition step (ADR-0017, ADR-0019).
type CreateWorkflowInstanceInput struct {
	CaseID            int64
	DefinitionID      string
	DefinitionVersion int
	Jobs              []WorkflowJobInput
	Actor             actor.Actor
}

// WorkflowInstance is the durable state machine row for one Workflow
// Instance (ADR-0019).
type WorkflowInstance struct {
	ID                int64
	CaseID            int64
	DefinitionID      string
	DefinitionVersion int
	State             workflows.InstanceState
	CurrentStep       *string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	FinishedAt        *time.Time
}

// WorkflowJob is the durable state machine row for one Job (ADR-0019).
type WorkflowJob struct {
	ID                 int64
	WorkflowInstanceID int64
	Position           int
	StepID             string
	OperationType      string
	State              workflows.JobState
	Attempt            int
	MaxAttempts        int
	CreatedAt          time.Time
	UpdatedAt          time.Time
	FinishedAt         *time.Time
}

// CreateWorkflowInstance attaches a Workflow to a Case: it writes the
// pending Workflow Instance, one pending Job row per ordered definition
// step, and a workflow_attached Timeline Event attributed to the acting
// Operator, in one transaction. Attaching to a closed Case is a conflict.
func (s *PostgresStore) CreateWorkflowInstance(ctx context.Context, input CreateWorkflowInstanceInput) (WorkflowInstance, error) {
	if err := validateCreateWorkflowInstanceInput(input); err != nil {
		return WorkflowInstance{}, err
	}

	occurredAt := time.Now().UTC()
	return runTransition(ctx, s.db,
		lockCase(input.CaseID),
		func(targetCase cases.Case) error {
			if targetCase.Status == cases.CaseStatusClosed {
				return errors.New("case is closed")
			}
			return nil
		},
		func(ctx context.Context, tx *sql.Tx, targetCase cases.Case) (WorkflowInstance, error) {
			var instance WorkflowInstance
			row := tx.QueryRowContext(ctx, `
				INSERT INTO workflow_instances (case_id, definition_id, definition_version, state, created_at, updated_at)
				VALUES ($1, $2, $3, 'pending', $4, $4)
				RETURNING `+workflowInstanceColumns,
				input.CaseID, input.DefinitionID, input.DefinitionVersion, occurredAt)
			instance, err := scanWorkflowInstance(row)
			if err != nil {
				return WorkflowInstance{}, err
			}

			for position, job := range input.Jobs {
				if _, err := tx.ExecContext(ctx, `
					INSERT INTO workflow_jobs (workflow_instance_id, position, step_id, operation_type, state, attempt, max_attempts, created_at, updated_at)
					VALUES ($1, $2, $3, $4, 'pending', 1, $5, $6, $6)
				`, instance.ID, position+1, job.StepID, job.OperationType, job.MaxAttempts, occurredAt); err != nil {
					return WorkflowInstance{}, err
				}
			}

			event := timelineEvent{
				Type:    cases.TimelineEventWorkflowAttached,
				Message: fmt.Sprintf("Workflow %s attached to case.", input.DefinitionID),
				Actor:   &input.Actor,
				Payload: struct {
					WorkflowID         string `json:"workflowId"`
					WorkflowInstanceID int64  `json:"workflowInstanceId"`
				}{WorkflowID: input.DefinitionID, WorkflowInstanceID: instance.ID},
			}
			if err := writeTimelineEvent(ctx, tx, input.CaseID, occurredAt, event); err != nil {
				return WorkflowInstance{}, err
			}
			return instance, nil
		},
	)
}

func validateCreateWorkflowInstanceInput(input CreateWorkflowInstanceInput) error {
	if input.DefinitionID == "" {
		return inputError("definition id is required")
	}
	if input.DefinitionVersion < 1 {
		return inputError("definition version must be positive")
	}
	if len(input.Jobs) == 0 {
		return inputError("at least one job is required")
	}
	stepIDs := make(map[string]bool, len(input.Jobs))
	for _, job := range input.Jobs {
		if job.StepID == "" {
			return inputError("job step id is required")
		}
		if stepIDs[job.StepID] {
			return inputError(fmt.Sprintf("job step id %q used twice", job.StepID))
		}
		stepIDs[job.StepID] = true
		if job.OperationType == "" {
			return inputError(fmt.Sprintf("job %s: operation type is required", job.StepID))
		}
		if job.MaxAttempts < 1 {
			return inputError(fmt.Sprintf("job %s: maxAttempts must be positive", job.StepID))
		}
	}
	return validateActor(input.Actor)
}

// GetWorkflowInstance returns a Workflow Instance by id.
func (s *PostgresStore) GetWorkflowInstance(ctx context.Context, instanceID int64) (WorkflowInstance, error) {
	if instanceID <= 0 {
		return WorkflowInstance{}, notFound("workflow instance not found")
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT `+workflowInstanceColumns+`
		FROM workflow_instances
		WHERE id = $1
	`, instanceID)
	instance, err := scanWorkflowInstance(row)
	if errors.Is(err, sql.ErrNoRows) {
		return WorkflowInstance{}, notFound("workflow instance not found")
	}
	return instance, err
}

// ListWorkflowInstancesByCase returns a Case's Workflow Instances in
// creation order.
func (s *PostgresStore) ListWorkflowInstancesByCase(ctx context.Context, caseID int64) ([]WorkflowInstance, error) {
	if caseID <= 0 {
		return nil, notFound("case not found")
	}

	var exists bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM cases
			WHERE id = $1
		)
	`, caseID).Scan(&exists)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, notFound("case not found")
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT `+workflowInstanceColumns+`
		FROM workflow_instances
		WHERE case_id = $1
		ORDER BY created_at ASC, id ASC
	`, caseID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	instances := make([]WorkflowInstance, 0)
	for rows.Next() {
		instance, err := scanWorkflowInstance(rows)
		if err != nil {
			return nil, err
		}
		instances = append(instances, instance)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return instances, nil
}

// ListWorkflowJobs returns a Workflow Instance's Jobs in definition
// order.
func (s *PostgresStore) ListWorkflowJobs(ctx context.Context, instanceID int64) ([]WorkflowJob, error) {
	if instanceID <= 0 {
		return nil, notFound("workflow instance not found")
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+workflowJobColumns+`
		FROM workflow_jobs
		WHERE workflow_instance_id = $1
		ORDER BY position ASC
	`, instanceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	jobs := make([]WorkflowJob, 0)
	for rows.Next() {
		job, err := scanWorkflowJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return jobs, nil
}

func scanWorkflowInstance(row rowScanner) (WorkflowInstance, error) {
	var instance WorkflowInstance
	var currentStep sql.NullString
	var finishedAt sql.NullTime
	err := row.Scan(
		&instance.ID, &instance.CaseID, &instance.DefinitionID, &instance.DefinitionVersion,
		&currentStep, &instance.State, &instance.CreatedAt, &instance.UpdatedAt, &finishedAt,
	)
	if err != nil {
		return WorkflowInstance{}, err
	}
	instance.CurrentStep = nullableStringPtr(currentStep)
	instance.FinishedAt = nullableTimePtr(finishedAt)
	return instance, nil
}

func scanWorkflowJob(row rowScanner) (WorkflowJob, error) {
	var job WorkflowJob
	var finishedAt sql.NullTime
	err := row.Scan(
		&job.ID, &job.WorkflowInstanceID, &job.Position, &job.StepID, &job.OperationType,
		&job.State, &job.Attempt, &job.MaxAttempts, &job.CreatedAt, &job.UpdatedAt, &finishedAt,
	)
	if err != nil {
		return WorkflowJob{}, err
	}
	job.FinishedAt = nullableTimePtr(finishedAt)
	return job, nil
}

func nullableStringPtr(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func nullableTimePtr(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	return &value.Time
}

// WorkflowInstanceTransitionInput advances a Workflow Instance's state.
// AtStep is the definition step id the instance pauses at; it is required
// when pausing at waiting_for_approval or waiting_for_operator and
// cleared on every other transition. Actor, when set, attributes the
// workflow_state_changed Timeline Event to the operating user; without
// it the event is attributed to the Atlas system actor.
type WorkflowInstanceTransitionInput struct {
	InstanceID int64
	To         workflows.InstanceState
	AtStep     string
	Actor      *actor.Actor
}

// TransitionWorkflowInstance advances a Workflow Instance under a row
// lock: the pure lifecycle rules decide the edge (ADR-0019), pausing
// records the paused-at step, and terminal states record finished_at.
// Every transition emits a workflow_state_changed Timeline Event on the
// owning Case in the same transaction.
func (s *PostgresStore) TransitionWorkflowInstance(ctx context.Context, input WorkflowInstanceTransitionInput) (WorkflowInstance, error) {
	target, err := workflows.ParseInstanceState(string(input.To))
	if err != nil {
		return WorkflowInstance{}, inputError(err.Error())
	}
	pausing := target == workflows.InstanceWaitingForApproval || target == workflows.InstanceWaitingForOperator
	if pausing && input.AtStep == "" {
		return WorkflowInstance{}, inputError("AtStep is required when pausing at a gate or task")
	}
	if !pausing && input.AtStep != "" {
		return WorkflowInstance{}, inputError("AtStep is only valid when pausing at a gate or task")
	}

	occurredAt := time.Now().UTC()
	return runTransition(ctx, s.db,
		lockInstance(input.InstanceID),
		func(current WorkflowInstance) error {
			return workflows.CanTransitionInstance(current.State, target)
		},
		func(ctx context.Context, tx *sql.Tx, current WorkflowInstance) (WorkflowInstance, error) {
			var currentStep sql.NullString
			if pausing {
				currentStep = sql.NullString{String: input.AtStep, Valid: true}
			}
			var finishedAt sql.NullTime
			if target == workflows.InstanceSucceeded || target == workflows.InstanceFailed || target == workflows.InstanceCancelled {
				finishedAt = sql.NullTime{Time: occurredAt, Valid: true}
			}
			row := tx.QueryRowContext(ctx, `
				UPDATE workflow_instances
				SET state = $2, current_step = $3, finished_at = $4, updated_at = $5
				WHERE id = $1
				RETURNING `+workflowInstanceColumns,
				input.InstanceID, target, currentStep, finishedAt, occurredAt)
			updated, err := scanWorkflowInstance(row)
			if err != nil {
				return WorkflowInstance{}, err
			}

			event := timelineEvent{
				Type:    cases.TimelineEventWorkflowStateChanged,
				Message: fmt.Sprintf("Workflow instance state changed to %s.", target),
				Actor:   input.Actor,
				Payload: struct {
					PreviousState workflows.InstanceState `json:"previousState"`
					NewState      workflows.InstanceState `json:"newState"`
					PausedAtStep  *string                 `json:"pausedAtStep,omitempty"`
				}{PreviousState: current.State, NewState: target, PausedAtStep: updated.CurrentStep},
			}
			if err := writeTimelineEvent(ctx, tx, current.CaseID, occurredAt, event); err != nil {
				return WorkflowInstance{}, err
			}
			return updated, nil
		},
	)
}

func lockWorkflowInstanceForUpdate(ctx context.Context, tx *sql.Tx, instanceID int64) (WorkflowInstance, error) {
	if instanceID <= 0 {
		return WorkflowInstance{}, notFound("workflow instance not found")
	}
	row := tx.QueryRowContext(ctx, `
		SELECT `+workflowInstanceColumns+`
		FROM workflow_instances
		WHERE id = $1
		FOR UPDATE
	`, instanceID)
	current, err := scanWorkflowInstance(row)
	if errors.Is(err, sql.ErrNoRows) {
		return WorkflowInstance{}, notFound("workflow instance not found")
	}
	return current, err
}

// WorkflowJobTransitionInput advances a Job's state.
type WorkflowJobTransitionInput struct {
	JobID int64
	To    workflows.JobState
}

// TransitionWorkflowJob advances a Job under a row lock: the pure
// lifecycle rules decide the edge, a retry (failed -> pending) consumes
// one attempt and is refused once the definition's retry budget is
// spent, and terminal states record finished_at (ADR-0019).
func (s *PostgresStore) TransitionWorkflowJob(ctx context.Context, input WorkflowJobTransitionInput) (WorkflowJob, error) {
	target, err := workflows.ParseJobState(string(input.To))
	if err != nil {
		return WorkflowJob{}, inputError(err.Error())
	}

	occurredAt := time.Now().UTC()
	return runTransition(ctx, s.db,
		func(ctx context.Context, tx *sql.Tx) (WorkflowJob, error) {
			return lockWorkflowJobForUpdate(ctx, tx, input.JobID)
		},
		func(current WorkflowJob) error {
			if err := workflows.CanTransitionJob(current.State, target); err != nil {
				return err
			}
			if current.State == workflows.JobFailed && target == workflows.JobPending && current.Attempt >= current.MaxAttempts {
				return fmt.Errorf("job %s exhausted its retry budget (%d attempts)", current.StepID, current.MaxAttempts)
			}
			return nil
		},
		func(ctx context.Context, tx *sql.Tx, current WorkflowJob) (WorkflowJob, error) {
			attempt := current.Attempt
			if current.State == workflows.JobFailed && target == workflows.JobPending {
				attempt = current.Attempt + 1
			}
			var finishedAt sql.NullTime
			if target == workflows.JobSucceeded || target == workflows.JobFailed {
				finishedAt = sql.NullTime{Time: occurredAt, Valid: true}
			}
			row := tx.QueryRowContext(ctx, `
				UPDATE workflow_jobs
				SET state = $2, attempt = $3, finished_at = $4, updated_at = $5
				WHERE id = $1
				RETURNING `+workflowJobColumns,
				input.JobID, target, attempt, finishedAt, occurredAt)
			updated, err := scanWorkflowJob(row)
			if err != nil {
				return WorkflowJob{}, err
			}
			return updated, nil
		},
	)
}

// lockInstance adapts lockWorkflowInstanceForUpdate into a runTransition
// lock function.
func lockInstance(instanceID int64) func(context.Context, *sql.Tx) (WorkflowInstance, error) {
	return func(ctx context.Context, tx *sql.Tx) (WorkflowInstance, error) {
		return lockWorkflowInstanceForUpdate(ctx, tx, instanceID)
	}
}

func lockWorkflowJobForUpdate(ctx context.Context, tx *sql.Tx, jobID int64) (WorkflowJob, error) {
	if jobID <= 0 {
		return WorkflowJob{}, notFound("workflow job not found")
	}
	row := tx.QueryRowContext(ctx, `
		SELECT `+workflowJobColumns+`
		FROM workflow_jobs
		WHERE id = $1
		FOR UPDATE
	`, jobID)
	current, err := scanWorkflowJob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return WorkflowJob{}, notFound("workflow job not found")
	}
	return current, err
}

// RecordApprovalInput records an Approval bound to a Workflow Instance
// gate (ADR-0020). Reason is optional.
type RecordApprovalInput struct {
	InstanceID int64
	GateID     string
	Approver   actor.Actor
	Reason     string
}

// ApprovalRecord is the durable, immutable record of one Approval
// (ADR-0020): it advanced one instance past one gate and authorizes
// nothing else. No update or delete paths exist.
type ApprovalRecord struct {
	ID                 int64
	WorkflowInstanceID int64
	GateID             string
	Approver           actor.Actor
	Reason             *string
	CreatedAt          time.Time
}

// RecordApproval writes the Approval record for a gate. The instance
// must be paused at that gate in waiting_for_approval; approving a gate
// that is not the instance's current one is rejected. A second approval
// of the same gate is an idempotent no-op returning the existing record.
func (s *PostgresStore) RecordApproval(ctx context.Context, input RecordApprovalInput) (ApprovalRecord, error) {
	if input.GateID == "" {
		return ApprovalRecord{}, inputError("gate id is required")
	}
	if err := validateActor(input.Approver); err != nil {
		return ApprovalRecord{}, err
	}

	type approvalState struct {
		Instance WorkflowInstance
		Existing *ApprovalRecord
	}

	occurredAt := time.Now().UTC()
	return runTransition(ctx, s.db,
		func(ctx context.Context, tx *sql.Tx) (approvalState, error) {
			instance, err := lockWorkflowInstanceForUpdate(ctx, tx, input.InstanceID)
			if err != nil {
				return approvalState{}, err
			}
			var existing ApprovalRecord
			row := tx.QueryRowContext(ctx, `
				SELECT id, workflow_instance_id, gate_id, approver_id, approver_display_name, reason, created_at
				FROM workflow_approvals
				WHERE workflow_instance_id = $1 AND gate_id = $2
			`, input.InstanceID, input.GateID)
			existing, err = scanApprovalRecord(row)
			if err == nil {
				return approvalState{Instance: instance, Existing: &existing}, nil
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return approvalState{}, err
			}
			return approvalState{Instance: instance}, nil
		},
		func(state approvalState) error {
			if state.Existing != nil {
				return nil
			}
			instance := state.Instance
			if instance.State != workflows.InstanceWaitingForApproval || instance.CurrentStep == nil || *instance.CurrentStep != input.GateID {
				return fmt.Errorf("workflow instance is not waiting for approval at gate %s", input.GateID)
			}
			return nil
		},
		func(ctx context.Context, tx *sql.Tx, state approvalState) (ApprovalRecord, error) {
			if state.Existing != nil {
				return *state.Existing, nil
			}

			var reason sql.NullString
			if input.Reason != "" {
				reason = sql.NullString{String: input.Reason, Valid: true}
			}
			inserted := tx.QueryRowContext(ctx, `
				INSERT INTO workflow_approvals (workflow_instance_id, gate_id, approver_id, approver_display_name, reason, created_at)
				VALUES ($1, $2, $3, $4, $5, $6)
				RETURNING id, workflow_instance_id, gate_id, approver_id, approver_display_name, reason, created_at
			`, input.InstanceID, input.GateID, input.Approver.Subject, input.Approver.DisplayName, reason, occurredAt)
			approval, err := scanApprovalRecord(inserted)
			if err != nil {
				return ApprovalRecord{}, err
			}
			return approval, nil
		},
	)
}

// ListWorkflowApprovals returns a Workflow Instance's Approval records
// in creation order.
func (s *PostgresStore) ListWorkflowApprovals(ctx context.Context, instanceID int64) ([]ApprovalRecord, error) {
	if instanceID <= 0 {
		return nil, notFound("workflow instance not found")
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, workflow_instance_id, gate_id, approver_id, approver_display_name, reason, created_at
		FROM workflow_approvals
		WHERE workflow_instance_id = $1
		ORDER BY created_at ASC, id ASC
	`, instanceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	approvals := make([]ApprovalRecord, 0)
	for rows.Next() {
		approval, err := scanApprovalRecord(rows)
		if err != nil {
			return nil, err
		}
		approvals = append(approvals, approval)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return approvals, nil
}

func scanApprovalRecord(row rowScanner) (ApprovalRecord, error) {
	var approval ApprovalRecord
	var reason sql.NullString
	err := row.Scan(
		&approval.ID, &approval.WorkflowInstanceID, &approval.GateID,
		&approval.Approver.Subject, &approval.Approver.DisplayName, &reason, &approval.CreatedAt,
	)
	if err != nil {
		return ApprovalRecord{}, err
	}
	approval.Reason = nullableStringPtr(reason)
	return approval, nil
}

const taskCompletionColumns = "id, workflow_instance_id, task_id, operator_id, operator_display_name, note, created_at"

// RecordTaskCompletionInput records an Operator's completion of a human
// Task bound to a Workflow Instance (ADR-0019). Note is optional.
type RecordTaskCompletionInput struct {
	InstanceID int64
	TaskID     string
	Operator   actor.Actor
	Note       string
}

// TaskCompletionRecord is the durable, immutable record that one human
// Task was performed (ADR-0019): it advances one instance past one Task
// and authorizes nothing else. No update or delete paths exist.
type TaskCompletionRecord struct {
	ID                 int64
	WorkflowInstanceID int64
	TaskID             string
	Operator           actor.Actor
	Note               *string
	CreatedAt          time.Time
}

// RecordTaskCompletion writes the Task completion record for a Task. The
// instance must be paused at that task in waiting_for_operator;
// completing a task that is not the instance's current one is rejected.
// A second completion of the same task is an idempotent no-op returning
// the existing record.
func (s *PostgresStore) RecordTaskCompletion(ctx context.Context, input RecordTaskCompletionInput) (TaskCompletionRecord, error) {
	if input.TaskID == "" {
		return TaskCompletionRecord{}, inputError("task id is required")
	}
	if err := validateActor(input.Operator); err != nil {
		return TaskCompletionRecord{}, err
	}

	type completionState struct {
		Instance WorkflowInstance
		Existing *TaskCompletionRecord
	}

	occurredAt := time.Now().UTC()
	return runTransition(ctx, s.db,
		func(ctx context.Context, tx *sql.Tx) (completionState, error) {
			instance, err := lockWorkflowInstanceForUpdate(ctx, tx, input.InstanceID)
			if err != nil {
				return completionState{}, err
			}
			var existing TaskCompletionRecord
			row := tx.QueryRowContext(ctx, `
				SELECT `+taskCompletionColumns+`
				FROM workflow_task_completions
				WHERE workflow_instance_id = $1 AND task_id = $2
			`, input.InstanceID, input.TaskID)
			existing, err = scanTaskCompletionRecord(row)
			if err == nil {
				return completionState{Instance: instance, Existing: &existing}, nil
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return completionState{}, err
			}
			return completionState{Instance: instance}, nil
		},
		func(state completionState) error {
			if state.Existing != nil {
				return nil
			}
			instance := state.Instance
			if instance.State != workflows.InstanceWaitingForOperator || instance.CurrentStep == nil || *instance.CurrentStep != input.TaskID {
				return fmt.Errorf("workflow instance is not waiting for operator at task %s", input.TaskID)
			}
			return nil
		},
		func(ctx context.Context, tx *sql.Tx, state completionState) (TaskCompletionRecord, error) {
			if state.Existing != nil {
				return *state.Existing, nil
			}

			var note sql.NullString
			if input.Note != "" {
				note = sql.NullString{String: input.Note, Valid: true}
			}
			inserted := tx.QueryRowContext(ctx, `
				INSERT INTO workflow_task_completions (workflow_instance_id, task_id, operator_id, operator_display_name, note, created_at)
				VALUES ($1, $2, $3, $4, $5, $6)
				RETURNING `+taskCompletionColumns,
				input.InstanceID, input.TaskID, input.Operator.Subject, input.Operator.DisplayName, note, occurredAt)
			completion, err := scanTaskCompletionRecord(inserted)
			if err != nil {
				return TaskCompletionRecord{}, err
			}
			return completion, nil
		},
	)
}

// ListWorkflowTaskCompletions returns a Workflow Instance's Task
// completion records in creation order.
func (s *PostgresStore) ListWorkflowTaskCompletions(ctx context.Context, instanceID int64) ([]TaskCompletionRecord, error) {
	if instanceID <= 0 {
		return nil, notFound("workflow instance not found")
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+taskCompletionColumns+`
		FROM workflow_task_completions
		WHERE workflow_instance_id = $1
		ORDER BY created_at ASC, id ASC
	`, instanceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	completions := make([]TaskCompletionRecord, 0)
	for rows.Next() {
		completion, err := scanTaskCompletionRecord(rows)
		if err != nil {
			return nil, err
		}
		completions = append(completions, completion)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return completions, nil
}

func scanTaskCompletionRecord(row rowScanner) (TaskCompletionRecord, error) {
	var completion TaskCompletionRecord
	var note sql.NullString
	err := row.Scan(
		&completion.ID, &completion.WorkflowInstanceID, &completion.TaskID,
		&completion.Operator.Subject, &completion.Operator.DisplayName, &note, &completion.CreatedAt,
	)
	if err != nil {
		return TaskCompletionRecord{}, err
	}
	completion.Note = nullableStringPtr(note)
	return completion, nil
}

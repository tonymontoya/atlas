package workflows

import "fmt"

// InstanceState is a Workflow Instance state (ADR-0019): pending,
// running, waiting_for_approval, waiting_for_operator, or the terminal
// succeeded, failed, cancelled.
type InstanceState string

const (
	InstancePending            InstanceState = "pending"
	InstanceRunning            InstanceState = "running"
	InstanceWaitingForApproval InstanceState = "waiting_for_approval"
	InstanceWaitingForOperator InstanceState = "waiting_for_operator"
	InstanceSucceeded          InstanceState = "succeeded"
	InstanceFailed             InstanceState = "failed"
	InstanceCancelled          InstanceState = "cancelled"
)

// JobState is a Job state (ADR-0019): pending, dispatched, or the
// terminal succeeded and failed, where failed may re-enter pending as a
// retry governed by the definition's retry policy.
type JobState string

const (
	JobPending    JobState = "pending"
	JobDispatched JobState = "dispatched"
	JobSucceeded  JobState = "succeeded"
	JobFailed     JobState = "failed"
)

// instanceTransitions is the Workflow Instance state machine (ADR-0019).
// Terminal states never revert, mirroring the Case closed-terminal
// precedent; cancellation is an instance-level terminal only.
var instanceTransitions = map[InstanceState]map[InstanceState]bool{
	InstancePending: {
		InstanceRunning:   true,
		InstanceCancelled: true,
	},
	InstanceRunning: {
		InstanceWaitingForApproval: true,
		InstanceWaitingForOperator: true,
		InstanceSucceeded:          true,
		InstanceFailed:             true,
		InstanceCancelled:          true,
	},
	InstanceWaitingForApproval: {
		InstanceRunning:   true,
		InstanceCancelled: true,
	},
	InstanceWaitingForOperator: {
		InstanceRunning:   true,
		InstanceCancelled: true,
	},
}

// jobTransitions is the Job state machine (ADR-0019). Retry is the
// failed -> pending edge; whether a retry is allowed is the definition's
// retry policy, enforced by the caller holding the Job row.
var jobTransitions = map[JobState]map[JobState]bool{
	JobPending: {
		JobDispatched: true,
	},
	JobDispatched: {
		JobSucceeded: true,
		JobFailed:    true,
	},
	JobFailed: {
		JobPending: true,
	},
}

// CanTransitionInstance reports whether a Workflow Instance may move
// from one state to another. Self-edges are rejected; terminal states
// never revert.
func CanTransitionInstance(from, to InstanceState) error {
	if from == to {
		return fmt.Errorf("workflow instance is already %s", from)
	}
	targets, ok := instanceTransitions[from]
	if !ok || !targets[to] {
		return fmt.Errorf("workflow instance transition %s -> %s is not allowed", from, to)
	}
	return nil
}

// CanTransitionJob reports whether a Job may move from one state to
// another. Self-edges are rejected; succeeded never reverts.
func CanTransitionJob(from, to JobState) error {
	if from == to {
		return fmt.Errorf("job is already %s", from)
	}
	targets, ok := jobTransitions[from]
	if !ok || !targets[to] {
		return fmt.Errorf("job transition %s -> %s is not allowed", from, to)
	}
	return nil
}

// ParseInstanceState parses a persisted instance state string.
func ParseInstanceState(value string) (InstanceState, error) {
	switch state := InstanceState(value); state {
	case InstancePending, InstanceRunning, InstanceWaitingForApproval, InstanceWaitingForOperator, InstanceSucceeded, InstanceFailed, InstanceCancelled:
		return state, nil
	default:
		return "", fmt.Errorf("unknown workflow instance state %q", value)
	}
}

// ParseJobState parses a persisted job state string.
func ParseJobState(value string) (JobState, error) {
	switch state := JobState(value); state {
	case JobPending, JobDispatched, JobSucceeded, JobFailed:
		return state, nil
	default:
		return "", fmt.Errorf("unknown job state %q", value)
	}
}

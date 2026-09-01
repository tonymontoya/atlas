// Package agent defines the Atlas Agent boundary (ADR-0022): the
// dispatcher hands serialized typed-operation request envelopes to an
// AgentAdapter and gets a job outcome back. The only v0.0.5
// implementation is the in-process fake; a network-speaking agent will
// slot behind the same interface.
package agent

import "context"

// Outcome is the terminal outcome an agent reports for one dispatched
// Job (ADR-0019): Jobs have no in-flight running state, so the adapter
// answers synchronously with succeeded or failed.
type Outcome string

const (
	OutcomeSucceeded Outcome = "succeeded"
	OutcomeFailed    Outcome = "failed"
)

// Result is the agent's answer for one dispatched Job.
type Result struct {
	Outcome Outcome
	Detail  string
}

// AgentAdapter executes one serialized agent request envelope
// (ADR-0018) and reports the outcome for the Job it carries. The
// serialized-bytes parameter is the wire contract: implementations must
// decode envelopes through the real typed-operation registry rather
// than accepting Go values, so the JSON contract is exercised
// honestly.
type AgentAdapter interface {
	Dispatch(ctx context.Context, envelope []byte) (Result, error)
}

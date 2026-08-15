package workflows

import (
	"testing"
)

func TestCanTransitionInstanceAllowsModelledEdges(t *testing.T) {
	allowed := []struct {
		from, to InstanceState
	}{
		{InstancePending, InstanceRunning},
		{InstancePending, InstanceCancelled},
		{InstanceRunning, InstanceWaitingForApproval},
		{InstanceRunning, InstanceWaitingForOperator},
		{InstanceRunning, InstanceSucceeded},
		{InstanceRunning, InstanceFailed},
		{InstanceRunning, InstanceCancelled},
		{InstanceWaitingForApproval, InstanceRunning},
		{InstanceWaitingForApproval, InstanceCancelled},
		{InstanceWaitingForOperator, InstanceRunning},
		{InstanceWaitingForOperator, InstanceCancelled},
	}
	for _, edge := range allowed {
		if err := CanTransitionInstance(edge.from, edge.to); err != nil {
			t.Fatalf("transition %s -> %s rejected: %v", edge.from, edge.to, err)
		}
	}
}

func TestCanTransitionInstanceRejectsSelfEdges(t *testing.T) {
	states := []InstanceState{
		InstancePending,
		InstanceRunning,
		InstanceWaitingForApproval,
		InstanceWaitingForOperator,
	}
	for _, state := range states {
		if err := CanTransitionInstance(state, state); err == nil {
			t.Fatalf("self-edge %s -> %s allowed", state, state)
		}
	}
}

func TestCanTransitionInstanceEnforcesTerminalFinality(t *testing.T) {
	terminal := []InstanceState{InstanceSucceeded, InstanceFailed, InstanceCancelled}
	targets := []InstanceState{
		InstancePending,
		InstanceRunning,
		InstanceWaitingForApproval,
		InstanceWaitingForOperator,
		InstanceSucceeded,
		InstanceFailed,
		InstanceCancelled,
	}
	for _, from := range terminal {
		for _, to := range targets {
			if err := CanTransitionInstance(from, to); err == nil {
				t.Fatalf("terminal transition %s -> %s allowed", from, to)
			}
		}
	}
}

func TestCanTransitionInstanceRejectsUnmodelledEdges(t *testing.T) {
	rejected := []struct {
		from, to InstanceState
	}{
		{InstancePending, InstanceWaitingForApproval},
		{InstancePending, InstanceSucceeded},
		{InstanceWaitingForApproval, InstanceWaitingForOperator},
		{InstanceWaitingForOperator, InstanceSucceeded},
	}
	for _, edge := range rejected {
		if err := CanTransitionInstance(edge.from, edge.to); err == nil {
			t.Fatalf("unmodelled transition %s -> %s allowed", edge.from, edge.to)
		}
	}
}

func TestCanTransitionJobAllowsModelledEdges(t *testing.T) {
	allowed := []struct {
		from, to JobState
	}{
		{JobPending, JobDispatched},
		{JobDispatched, JobSucceeded},
		{JobDispatched, JobFailed},
		{JobFailed, JobPending},
	}
	for _, edge := range allowed {
		if err := CanTransitionJob(edge.from, edge.to); err != nil {
			t.Fatalf("transition %s -> %s rejected: %v", edge.from, edge.to, err)
		}
	}
}

func TestCanTransitionJobRejectsSelfEdges(t *testing.T) {
	states := []JobState{JobPending, JobDispatched, JobFailed}
	for _, state := range states {
		if err := CanTransitionJob(state, state); err == nil {
			t.Fatalf("self-edge %s -> %s allowed", state, state)
		}
	}
}

func TestCanTransitionJobEnforcesTerminalFinality(t *testing.T) {
	targets := []JobState{JobPending, JobDispatched, JobSucceeded, JobFailed}
	for _, to := range targets {
		if err := CanTransitionJob(JobSucceeded, to); err == nil {
			t.Fatalf("terminal transition succeeded -> %s allowed", to)
		}
	}
}

func TestCanTransitionJobRejectsUnmodelledEdges(t *testing.T) {
	rejected := []struct {
		from, to JobState
	}{
		{JobPending, JobSucceeded},
		{JobPending, JobFailed},
		{JobFailed, JobDispatched},
	}
	for _, edge := range rejected {
		if err := CanTransitionJob(edge.from, edge.to); err == nil {
			t.Fatalf("unmodelled transition %s -> %s allowed", edge.from, edge.to)
		}
	}
}

func TestParseInstanceState(t *testing.T) {
	for _, state := range []InstanceState{
		InstancePending,
		InstanceRunning,
		InstanceWaitingForApproval,
		InstanceWaitingForOperator,
		InstanceSucceeded,
		InstanceFailed,
		InstanceCancelled,
	} {
		parsed, err := ParseInstanceState(string(state))
		if err != nil || parsed != state {
			t.Fatalf("parse %q = %q, %v", state, parsed, err)
		}
	}
	if _, err := ParseInstanceState("paused"); err == nil {
		t.Fatal("parsed unknown instance state")
	}
}

func TestParseJobState(t *testing.T) {
	for _, state := range []JobState{JobPending, JobDispatched, JobSucceeded, JobFailed} {
		parsed, err := ParseJobState(string(state))
		if err != nil || parsed != state {
			t.Fatalf("parse %q = %q, %v", state, parsed, err)
		}
	}
	if _, err := ParseJobState("running"); err == nil {
		t.Fatal("parsed unknown job state")
	}
}

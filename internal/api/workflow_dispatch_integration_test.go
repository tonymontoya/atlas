package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/tonymontoya/ceph-atlas/internal/cases"
	"github.com/tonymontoya/ceph-atlas/internal/providers"
)

// createManualCaseForAPIWithCluster creates a manual Case bound to a
// cluster fsid, as the replace-osd Workflow's mutating operations need
// one.
func createManualCaseForAPIWithCluster(t *testing.T, harness *writeHarness) int64 {
	t.Helper()
	response := harness.do(t, http.MethodPost, "/api/v1/cases", map[string]any{
		"title": "api-manual-test lifecycle", "summary": "api-manual-test summary", "severity": "low",
		"clusterFsid": "00000000-0000-4000-8000-000000000101",
	}, true)
	if response.Code != http.StatusCreated {
		t.Fatalf("create case status = %d; body=%s", response.Code, response.Body.String())
	}
	var created cases.Case
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatalf("decode case: %v", err)
	}
	return created.ID
}

type workflowJobResponse struct {
	ID            int64  `json:"id"`
	StepID        string `json:"stepId"`
	OperationType string `json:"operationType"`
	State         string `json:"state"`
	Position      int    `json:"position"`
	Attempt       int    `json:"attempt"`
	MaxAttempts   int    `json:"maxAttempts"`
}

func listWorkflowJobsForAPI(t *testing.T, harness *writeHarness, instanceID int64) []workflowJobResponse {
	t.Helper()
	path := "/api/v1/workflow-instances/" + strconv.FormatInt(instanceID, 10) + "/jobs"
	response := harness.do(t, http.MethodGet, path, nil, false)
	if response.Code != http.StatusOK {
		t.Fatalf("list jobs status = %d; body=%s", response.Code, response.Body.String())
	}
	var jobs []workflowJobResponse
	if err := json.NewDecoder(response.Body).Decode(&jobs); err != nil {
		t.Fatalf("decode jobs: %v", err)
	}
	return jobs
}

// attachWorkflowForAPIInFakeAgentMode attaches replace-osd to a fresh
// cluster-bound Case through the fake-agent harness and returns the
// parked instance's id.
func attachWorkflowForAPIInFakeAgentMode(t *testing.T, harness *writeHarness) (caseID int64, instanceID int64) {
	t.Helper()
	caseID = createManualCaseForAPIWithCluster(t, harness)
	response := harness.do(t, http.MethodPost, "/api/v1/cases/"+strconv.FormatInt(caseID, 10)+"/workflows", map[string]any{
		"workflowId": "replace-osd", "workflowVersion": 1,
	}, true)
	if response.Code != http.StatusCreated {
		t.Fatalf("attach status = %d; body=%s", response.Code, response.Body.String())
	}
	var instance attachWorkflowResponse
	if err := json.NewDecoder(response.Body).Decode(&instance); err != nil {
		t.Fatalf("decode instance: %v", err)
	}
	return caseID, instance.ID
}

func TestApproveWorkflowGateDrivesInstanceToSucceededWithFakeAgent(t *testing.T) {
	harness := newWriteHarnessWithAgentMode(t, "fake")
	caseID, instanceID := attachWorkflowForAPIInFakeAgentMode(t, harness)

	response := harness.do(t, http.MethodPost,
		"/api/v1/workflow-instances/"+strconv.FormatInt(instanceID, 10)+"/approvals",
		map[string]any{"gateId": "approve-destroy"}, true)
	if response.Code != http.StatusCreated {
		t.Fatalf("approve status = %d; body=%s", response.Code, response.Body.String())
	}

	instances := listCaseWorkflowInstances(t, harness, caseID)
	if len(instances) != 1 || instances[0].State != "succeeded" {
		t.Fatalf("instance = %+v, want succeeded", instances)
	}
	if instances[0].FinishedAt == nil {
		t.Fatal("succeeded instance has no finishedAt")
	}

	jobs := listWorkflowJobsForAPI(t, harness, instanceID)
	if len(jobs) != 3 {
		t.Fatalf("jobs = %d, want 3", len(jobs))
	}
	want := []struct {
		stepID    string
		operation string
	}{
		{"collect-evidence", "CollectHostEvidence"},
		{"destroy-osd", "DestroyOSD"},
		{"verify-osd", "VerifyOSD"},
	}
	for i, job := range jobs {
		if job.StepID != want[i].stepID || job.OperationType != want[i].operation || job.State != "succeeded" {
			t.Fatalf("job %d = %+v, want %s succeeded", i, job, want[i].stepID)
		}
	}

	events := timelineForCase(t, harness, caseID)
	var lastChange map[string]any
	changes := 0
	for _, event := range events {
		if event.Type == cases.TimelineEventWorkflowStateChanged {
			changes++
			lastChange = event.Payload
		}
	}
	// running, waiting_for_approval, running (resume), succeeded.
	if changes != 4 {
		t.Fatalf("workflow_state_changed events = %d, want 4; events=%+v", changes, events)
	}
	if lastChange["newState"] != "succeeded" {
		t.Fatalf("last state change = %+v, want succeeded", lastChange)
	}
}

func TestApproveWorkflowGateWithoutAgentLoopLeavesJobsPending(t *testing.T) {
	harness := newWriteHarness(t)
	caseID, instanceID := attachWorkflowForAPI(t, harness)

	response := harness.do(t, http.MethodPost,
		"/api/v1/workflow-instances/"+strconv.FormatInt(instanceID, 10)+"/approvals",
		map[string]any{"gateId": "approve-destroy"}, true)
	if response.Code != http.StatusCreated {
		t.Fatalf("approve status = %d; body=%s", response.Code, response.Body.String())
	}

	instances := listCaseWorkflowInstances(t, harness, caseID)
	if len(instances) != 1 || instances[0].State != "running" {
		t.Fatalf("instance = %+v, want running with nothing dispatched", instances)
	}
	jobs := listWorkflowJobsForAPI(t, harness, instanceID)
	if len(jobs) != 3 {
		t.Fatalf("jobs = %d, want 3", len(jobs))
	}
	for _, job := range jobs {
		if job.State != "pending" {
			t.Fatalf("job %s = %s, want pending in disabled mode", job.StepID, job.State)
		}
	}
}

func TestListWorkflowJobsUnknownInstance(t *testing.T) {
	harness := newWriteHarness(t)

	response := harness.do(t, http.MethodGet, "/api/v1/workflow-instances/999999/jobs", nil, false)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusNotFound, response.Body.String())
	}
	if class := decodeErrorClass(t, response); class != string(providers.ErrorNotFound) {
		t.Fatalf("error class = %q, want %q", class, providers.ErrorNotFound)
	}
}

func TestListWorkflowJobsRequiresPostgresReadSource(t *testing.T) {
	harness := newFakeWriteHarness(t)

	response := harness.do(t, http.MethodGet, "/api/v1/workflow-instances/1/jobs", nil, false)

	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusUnprocessableEntity, response.Body.String())
	}
	if class := decodeErrorClass(t, response); class != string(providers.ErrorUnsupported) {
		t.Fatalf("error class = %q, want %q", class, providers.ErrorUnsupported)
	}
}

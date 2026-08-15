package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/tonymontoya/ceph-atlas/internal/agent"
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

func TestApproveWorkflowGatePausesAtTaskUntilOperatorResumes(t *testing.T) {
	harness := newWriteHarnessWithAgentScenario(t, agent.ScenarioDispatchFailsOnce)
	caseID, instanceID := attachWorkflowForAPIInFakeAgentMode(t, harness)

	response := harness.do(t, http.MethodPost,
		"/api/v1/workflow-instances/"+strconv.FormatInt(instanceID, 10)+"/approvals",
		map[string]any{"gateId": "approve-destroy"}, true)
	if response.Code != http.StatusCreated {
		t.Fatalf("approve status = %d; body=%s", response.Code, response.Body.String())
	}

	// The dispatch loop drove the Jobs (collect-evidence retried past
	// its scripted transient failure) and paused at the human Task.
	instances := listCaseWorkflowInstances(t, harness, caseID)
	if len(instances) != 1 || instances[0].State != "waiting_for_operator" {
		t.Fatalf("instance = %+v, want waiting_for_operator at the task", instances)
	}
	if instances[0].CurrentStep == nil || *instances[0].CurrentStep != "replace-device" {
		t.Fatalf("current step = %v, want replace-device", instances[0].CurrentStep)
	}
	jobs := listWorkflowJobsForAPI(t, harness, instanceID)
	wantJobStates := []string{"succeeded", "succeeded", "pending"}
	for i, job := range jobs {
		if job.State != wantJobStates[i] {
			t.Fatalf("job %s = %s, want %s at the task pause", job.StepID, job.State, wantJobStates[i])
		}
	}
	if jobs[0].Attempt != 2 {
		t.Fatalf("collect-evidence attempt = %d, want 2 after the scripted retry", jobs[0].Attempt)
	}

	// The Operator resumes: the completion drives the instance to
	// terminal succeeded within the request.
	resume := harness.do(t, http.MethodPost,
		"/api/v1/workflow-instances/"+strconv.FormatInt(instanceID, 10)+"/task-completions",
		map[string]any{"taskId": "replace-device", "note": "device swapped"}, true)
	if resume.Code != http.StatusCreated {
		t.Fatalf("task completion status = %d; body=%s", resume.Code, resume.Body.String())
	}
	var completion struct {
		ID     int64   `json:"id"`
		TaskID string  `json:"taskId"`
		Note   *string `json:"note"`
	}
	if err := json.NewDecoder(resume.Body).Decode(&completion); err != nil {
		t.Fatalf("decode completion: %v", err)
	}
	if completion.ID == 0 || completion.TaskID != "replace-device" || completion.Note == nil || *completion.Note != "device swapped" {
		t.Fatalf("completion = %+v", completion)
	}

	instances = listCaseWorkflowInstances(t, harness, caseID)
	if len(instances) != 1 || instances[0].State != "succeeded" {
		t.Fatalf("instance = %+v, want succeeded after the resume", instances)
	}
	if instances[0].FinishedAt == nil {
		t.Fatal("succeeded instance has no finishedAt")
	}
	jobs = listWorkflowJobsForAPI(t, harness, instanceID)
	for i, job := range jobs {
		if job.State != "succeeded" {
			t.Fatalf("job %d = %+v, want succeeded", i, job)
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
	// running, waiting_for_approval, running (gate resume),
	// waiting_for_operator, running (task resume), succeeded.
	if changes != 6 {
		t.Fatalf("workflow_state_changed events = %d, want 6; events=%+v", changes, events)
	}
	if lastChange["newState"] != "succeeded" {
		t.Fatalf("last state change = %+v, want succeeded", lastChange)
	}
}

func TestCompleteWorkflowTaskIsIdempotent(t *testing.T) {
	harness := newWriteHarnessWithAgentMode(t, "fake")
	caseID, instanceID := attachWorkflowForAPIInFakeAgentMode(t, harness)

	if _, instanceID = approveGateForAPI(t, harness, caseID, instanceID); instanceID == 0 {
		t.Fatal("unreachable")
	}
	resume := harness.do(t, http.MethodPost,
		"/api/v1/workflow-instances/"+strconv.FormatInt(instanceID, 10)+"/task-completions",
		map[string]any{"taskId": "replace-device"}, true)
	if resume.Code != http.StatusCreated {
		t.Fatalf("first completion status = %d; body=%s", resume.Code, resume.Body.String())
	}
	var first struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resume.Body).Decode(&first); err != nil {
		t.Fatalf("decode first completion: %v", err)
	}

	again := harness.do(t, http.MethodPost,
		"/api/v1/workflow-instances/"+strconv.FormatInt(instanceID, 10)+"/task-completions",
		map[string]any{"taskId": "replace-device"}, true)
	if again.Code != http.StatusOK {
		t.Fatalf("second completion status = %d; body=%s", again.Code, again.Body.String())
	}
	var second struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(again.Body).Decode(&second); err != nil {
		t.Fatalf("decode second completion: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("second completion id = %d, want the idempotent %d", second.ID, first.ID)
	}

	// The terminal instance was untouched by the replay.
	instances := listCaseWorkflowInstances(t, harness, caseID)
	if len(instances) != 1 || instances[0].State != "succeeded" {
		t.Fatalf("instance = %+v, want still succeeded after the replay", instances)
	}
}

func TestCompleteWorkflowTaskRejectsInvalidRequests(t *testing.T) {
	harness := newWriteHarnessWithAgentMode(t, "fake")
	caseID, instanceID := attachWorkflowForAPIInFakeAgentMode(t, harness)
	approveGateForAPI(t, harness, caseID, instanceID)

	// The instance is waiting_for_operator at replace-device, not at
	// any other task.
	wrongTask := harness.do(t, http.MethodPost,
		"/api/v1/workflow-instances/"+strconv.FormatInt(instanceID, 10)+"/task-completions",
		map[string]any{"taskId": "rotate-devices"}, true)
	if wrongTask.Code != http.StatusConflict {
		t.Fatalf("wrong task status = %d; body=%s", wrongTask.Code, wrongTask.Body.String())
	}
	if class := decodeErrorClass(t, wrongTask); class != string(providers.ErrorConflict) {
		t.Fatalf("error class = %q, want %q", class, providers.ErrorConflict)
	}

	missingTask := harness.do(t, http.MethodPost,
		"/api/v1/workflow-instances/"+strconv.FormatInt(instanceID, 10)+"/task-completions",
		map[string]any{}, true)
	if missingTask.Code != http.StatusBadRequest {
		t.Fatalf("missing task status = %d; body=%s", missingTask.Code, missingTask.Body.String())
	}

	unknown := harness.do(t, http.MethodPost,
		"/api/v1/workflow-instances/999999/task-completions",
		map[string]any{"taskId": "replace-device"}, true)
	if unknown.Code != http.StatusNotFound {
		t.Fatalf("unknown instance status = %d; body=%s", unknown.Code, unknown.Body.String())
	}

	unauthenticated := harness.do(t, http.MethodPost,
		"/api/v1/workflow-instances/"+strconv.FormatInt(instanceID, 10)+"/task-completions",
		map[string]any{"taskId": "replace-device"}, false)
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d; body=%s", unauthenticated.Code, unauthenticated.Body.String())
	}

	fakeHarness := newFakeWriteHarness(t)
	unsupported := fakeHarness.do(t, http.MethodPost,
		"/api/v1/workflow-instances/1/task-completions",
		map[string]any{"taskId": "replace-device"}, true)
	if unsupported.Code != http.StatusUnprocessableEntity {
		t.Fatalf("fake source status = %d; body=%s", unsupported.Code, unsupported.Body.String())
	}
	if class := decodeErrorClass(t, unsupported); class != string(providers.ErrorUnsupported) {
		t.Fatalf("error class = %q, want %q", class, providers.ErrorUnsupported)
	}
}

// approveGateForAPI approves the gate and returns the paused-at-task
// instance id.
func approveGateForAPI(t *testing.T, harness *writeHarness, caseID int64, instanceID int64) (int64, int64) {
	t.Helper()
	response := harness.do(t, http.MethodPost,
		"/api/v1/workflow-instances/"+strconv.FormatInt(instanceID, 10)+"/approvals",
		map[string]any{"gateId": "approve-destroy"}, true)
	if response.Code != http.StatusCreated {
		t.Fatalf("approve status = %d; body=%s", response.Code, response.Body.String())
	}
	instances := listCaseWorkflowInstances(t, harness, caseID)
	if len(instances) != 1 || instances[0].State != "waiting_for_operator" {
		t.Fatalf("instance = %+v, want waiting_for_operator after approval dispatch", instances)
	}
	return caseID, instances[0].ID
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

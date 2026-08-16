package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/tonymontoya/ceph-atlas/internal/app"
	"github.com/tonymontoya/ceph-atlas/internal/cases"
	"github.com/tonymontoya/ceph-atlas/internal/config"
	"github.com/tonymontoya/ceph-atlas/internal/identity/devissuer"
	"github.com/tonymontoya/ceph-atlas/internal/providers"
)

type attachWorkflowResponse struct {
	ID              int64      `json:"id"`
	CaseID          int64      `json:"caseId"`
	WorkflowID      string     `json:"workflowId"`
	WorkflowVersion int        `json:"workflowVersion"`
	State           string     `json:"state"`
	CurrentStep     *string    `json:"currentStep"`
	FinishedAt      *time.Time `json:"finishedAt"`
}

func workflowJobsForInstance(t *testing.T, instanceID int64) []struct {
	Position    int
	StepID      string
	State       string
	Attempt     int
	MaxAttempts int
} {
	t.Helper()
	db := openTestDB(t, context.Background(), testDatabaseURL(t))
	t.Cleanup(func() { _ = db.Close() })
	rows, err := db.Query(`
		SELECT position, step_id, state, attempt, max_attempts
		FROM workflow_jobs
		WHERE workflow_instance_id = $1
		ORDER BY position ASC
	`, instanceID)
	if err != nil {
		t.Fatalf("query workflow jobs: %v", err)
	}
	defer func() { _ = rows.Close() }()

	jobs := make([]struct {
		Position    int
		StepID      string
		State       string
		Attempt     int
		MaxAttempts int
	}, 0)
	for rows.Next() {
		var job struct {
			Position    int
			StepID      string
			State       string
			Attempt     int
			MaxAttempts int
		}
		if err := rows.Scan(&job.Position, &job.StepID, &job.State, &job.Attempt, &job.MaxAttempts); err != nil {
			t.Fatalf("scan workflow job: %v", err)
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate workflow jobs: %v", err)
	}
	return jobs
}

func TestAttachWorkflowRequiresAuthentication(t *testing.T) {
	harness := newWriteHarness(t)
	created := createManualCaseForAPI(t, harness)
	path := "/api/v1/cases/" + strconv.FormatInt(created, 10) + "/workflows"

	response := harness.do(t, http.MethodPost, path, map[string]any{"workflowId": "replace-osd", "workflowVersion": 1}, false)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusUnauthorized, response.Body.String())
	}
	if class := decodeErrorClass(t, response); class != string(providers.ErrorUnauthorized) {
		t.Fatalf("error class = %q, want %q", class, providers.ErrorUnauthorized)
	}
}

func TestAttachWorkflowCreatesInstanceJobsAndTimelineEvent(t *testing.T) {
	harness := newWriteHarness(t)
	created := createManualCaseForAPI(t, harness)
	path := "/api/v1/cases/" + strconv.FormatInt(created, 10) + "/workflows"

	response := harness.do(t, http.MethodPost, path, map[string]any{"workflowId": "replace-osd", "workflowVersion": 1}, true)
	if response.Code != http.StatusCreated {
		t.Fatalf("attach status = %d, want %d; body=%s", response.Code, http.StatusCreated, response.Body.String())
	}
	var instance attachWorkflowResponse
	if err := json.NewDecoder(response.Body).Decode(&instance); err != nil {
		t.Fatalf("decode instance: %v", err)
	}
	if instance.ID == 0 || instance.CaseID != created {
		t.Fatalf("instance = %+v, want id and case %d", instance, created)
	}
	if instance.WorkflowID != "replace-osd" || instance.WorkflowVersion != 1 {
		t.Fatalf("instance = %+v, want replace-osd v1", instance)
	}
	if instance.State != "waiting_for_approval" {
		t.Fatalf("state = %q, want waiting_for_approval", instance.State)
	}
	if instance.CurrentStep == nil || *instance.CurrentStep != "approve-destroy" {
		t.Fatalf("currentStep = %v, want approve-destroy", instance.CurrentStep)
	}

	jobs := workflowJobsForInstance(t, instance.ID)
	wantSteps := []struct {
		StepID      string
		MaxAttempts int
	}{
		{"collect-evidence", 3},
		{"destroy-osd", 1},
		{"verify-osd", 3},
	}
	if len(jobs) != len(wantSteps) {
		t.Fatalf("jobs = %+v, want %d Job rows (Gates and Tasks are not Jobs)", jobs, len(wantSteps))
	}
	for index, want := range wantSteps {
		job := jobs[index]
		if job.Position != index+1 || job.StepID != want.StepID || job.MaxAttempts != want.MaxAttempts {
			t.Fatalf("job %d = %+v, want position %d step %s maxAttempts %d", index, job, index+1, want.StepID, want.MaxAttempts)
		}
		if job.State != "pending" || job.Attempt != 1 {
			t.Fatalf("job %s = %+v, want pending attempt 1", job.StepID, job)
		}
	}

	timeline := harness.do(t, http.MethodGet, "/api/v1/cases/"+strconv.FormatInt(created, 10)+"/timeline", nil, false)
	if timeline.Code != http.StatusOK {
		t.Fatalf("timeline status = %d; body=%s", timeline.Code, timeline.Body.String())
	}
	var events []cases.TimelineEvent
	if err := json.NewDecoder(timeline.Body).Decode(&events); err != nil {
		t.Fatalf("decode timeline: %v", err)
	}
	if len(events) != 4 || events[1].Type != cases.TimelineEventWorkflowAttached {
		t.Fatalf("events = %+v, want detected, attached, then two advancement events", events)
	}
	attach := events[1]
	if attach.Actor.Type != cases.TimelineActorUser || attach.Actor.ID != "write-operator" || attach.Actor.DisplayName != "Write Operator" {
		t.Fatalf("actor = %+v, want the verified operator", attach.Actor)
	}
	if attach.Payload["workflowId"] != "replace-osd" {
		t.Fatalf("payload = %+v, want workflowId replace-osd", attach.Payload)
	}
	if instanceID, ok := attach.Payload["workflowInstanceId"].(float64); !ok || int64(instanceID) != instance.ID {
		t.Fatalf("payload = %+v, want workflowInstanceId %d", attach.Payload, instance.ID)
	}
}

func TestAttachWorkflowRejectsUnknownDefinitions(t *testing.T) {
	harness := newWriteHarness(t)
	created := createManualCaseForAPI(t, harness)
	path := "/api/v1/cases/" + strconv.FormatInt(created, 10) + "/workflows"

	for name, body := range map[string]map[string]any{
		"unknown workflow id": {"workflowId": "not-a-workflow", "workflowVersion": 1},
		"unknown version":     {"workflowId": "replace-osd", "workflowVersion": 2},
	} {
		t.Run(name, func(t *testing.T) {
			response := harness.do(t, http.MethodPost, path, body, true)
			if response.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusNotFound, response.Body.String())
			}
			if class := decodeErrorClass(t, response); class != string(providers.ErrorNotFound) {
				t.Fatalf("error class = %q, want %q", class, providers.ErrorNotFound)
			}
		})
	}
}

func TestAttachWorkflowRejectsInvalidRequests(t *testing.T) {
	harness := newWriteHarness(t)
	created := createManualCaseForAPI(t, harness)
	path := "/api/v1/cases/" + strconv.FormatInt(created, 10) + "/workflows"

	for name, body := range map[string]map[string]any{
		"missing workflow id": {"workflowVersion": 1},
		"empty workflow id":   {"workflowId": "", "workflowVersion": 1},
		"zero version":        {"workflowId": "replace-osd", "workflowVersion": 0},
		"negative version":    {"workflowId": "replace-osd", "workflowVersion": -1},
	} {
		t.Run(name, func(t *testing.T) {
			response := harness.do(t, http.MethodPost, path, body, true)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusBadRequest, response.Body.String())
			}
			if class := decodeErrorClass(t, response); class != "InvalidRequest" {
				t.Fatalf("error class = %q, want InvalidRequest", class)
			}
		})
	}

	missing := harness.do(t, http.MethodPost, "/api/v1/cases/999999/workflows", map[string]any{"workflowId": "replace-osd", "workflowVersion": 1}, true)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing case status = %d, want %d; body=%s", missing.Code, http.StatusNotFound, missing.Body.String())
	}
	if class := decodeErrorClass(t, missing); class != string(providers.ErrorNotFound) {
		t.Fatalf("error class = %q, want %q", class, providers.ErrorNotFound)
	}
}

func TestAttachWorkflowRejectsClosedCase(t *testing.T) {
	harness := newWriteHarness(t)
	created := createManualCaseForAPI(t, harness)
	closed := harness.do(t, http.MethodPost, "/api/v1/cases/"+strconv.FormatInt(created, 10)+"/transitions", map[string]any{"status": "closed"}, true)
	if closed.Code != http.StatusOK {
		t.Fatalf("close status = %d; body=%s", closed.Code, closed.Body.String())
	}

	response := harness.do(t, http.MethodPost, "/api/v1/cases/"+strconv.FormatInt(created, 10)+"/workflows", map[string]any{"workflowId": "replace-osd", "workflowVersion": 1}, true)
	if response.Code != http.StatusConflict {
		t.Fatalf("attach status = %d, want %d; body=%s", response.Code, http.StatusConflict, response.Body.String())
	}
	if class := decodeErrorClass(t, response); class != string(providers.ErrorConflict) {
		t.Fatalf("error class = %q, want %q", class, providers.ErrorConflict)
	}
}

func TestAttachWorkflowRequiresPostgresWriteSource(t *testing.T) {
	harness := newFakeWriteHarness(t)

	response := harness.do(t, http.MethodPost, "/api/v1/cases/1/workflows", map[string]any{"workflowId": "replace-osd", "workflowVersion": 1}, true)

	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusUnprocessableEntity, response.Body.String())
	}
	if class := decodeErrorClass(t, response); class != string(providers.ErrorUnsupported) {
		t.Fatalf("error class = %q, want %q", class, providers.ErrorUnsupported)
	}
}

func newFakeWriteHarness(t *testing.T) *writeHarness {
	t.Helper()
	issuer, err := devissuer.New("https://atlas-dev-issuer.local", "atlas-api")
	if err != nil {
		t.Fatalf("create dev issuer: %v", err)
	}
	jwks := httptest.NewServer(issuer.Handler())
	t.Cleanup(jwks.Close)
	token, err := issuer.IssueToken("op", "Op", 15*time.Minute)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	application, err := app.NewFromConfig(context.Background(), config.Config{
		FakeScenario: "reef-healthy-baremetal",
		ReadSource:   config.ReadSourceProvider,
		AgentMode:    config.AgentModeDisabled,
		OIDCIssuer:   "https://atlas-dev-issuer.local",
		OIDCAudience: "atlas-api",
		OIDCJWKSURL:  jwks.URL + "/.well-known/jwks.json",
	})
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	return &writeHarness{server: NewServer(application), token: token}
}

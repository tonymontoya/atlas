package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"github.com/tonymontoya/ceph-atlas/internal/app"
	"github.com/tonymontoya/ceph-atlas/internal/apperr"
	"github.com/tonymontoya/ceph-atlas/internal/cases"
	"github.com/tonymontoya/ceph-atlas/internal/config"
	"github.com/tonymontoya/ceph-atlas/internal/identity/devissuer/devissuertest"
	"github.com/tonymontoya/ceph-atlas/internal/testdb"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

type writeHarness struct {
	server *Server
	token  string
}

func newWriteHarness(t *testing.T) *writeHarness {
	t.Helper()
	return newWriteHarnessWithAgentMode(t, "disabled")
}

// newWriteHarnessWithAgentMode builds the PostgreSQL write harness with
// an explicit ATLAS_AGENT_MODE: "fake" wires the in-process fake agent
// loop (ADR-0022); anything else dispatches nothing.
func newWriteHarnessWithAgentMode(t *testing.T, agentMode string) *writeHarness {
	t.Helper()
	return newWriteHarnessWithOptions(t, agentMode, "")
}

// newWriteHarnessWithAgentScenario builds the fake-agent harness with a
// failure scenario (ATLAS_FAKE_AGENT_SCENARIO) driving the dispatch
// loop.
func newWriteHarnessWithAgentScenario(t *testing.T, fakeAgentScenario string) *writeHarness {
	t.Helper()
	return newWriteHarnessWithOptions(t, "fake", fakeAgentScenario)
}

func newWriteHarnessWithOptions(t *testing.T, agentMode string, fakeAgentScenario string) *writeHarness {
	t.Helper()
	ctx := context.Background()

	issuer := devissuertest.Start(t)
	token := issuer.Token(t, "write-operator", "Write Operator")

	db, databaseURL := testdb.Open(t)
	cleanupManualAPIRows(t, db)
	t.Cleanup(func() { cleanupManualAPIRows(t, db) })

	application, err := app.NewFromConfig(ctx, config.Config{
		DatabaseURL:       databaseURL,
		ReadSource:        "postgres",
		AgentMode:         config.AgentMode(agentMode),
		FakeAgentScenario: fakeAgentScenario,
		OIDCIssuer:        devissuertest.IssuerURL,
		OIDCAudience:      devissuertest.Audience,
		OIDCJWKSURL:       issuer.JWKSURL(),
	})
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	t.Cleanup(func() { _ = application.Close() })
	return &writeHarness{server: NewServer(application), token: token}
}

func cleanupManualAPIRows(t *testing.T, db *sql.DB) {
	t.Helper()
	testdb.DeleteCases(t, db, "title LIKE 'api-manual-test%'")
}

func (h *writeHarness) do(t *testing.T, method, path string, body any, withToken bool) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("encode body: %v", err)
		}
		reader = bytes.NewReader(encoded)
	} else {
		reader = bytes.NewReader(nil)
	}
	request := httptest.NewRequest(method, path, reader)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if withToken {
		request.Header.Set("Authorization", "Bearer "+h.token)
	}
	response := httptest.NewRecorder()
	h.server.Routes().ServeHTTP(response, request)
	return response
}

func decodeErrorClass(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Class   string `json:"class"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if body.Error.Class == "" || body.Error.Message == "" {
		t.Fatalf("expected structured error envelope, got %s", response.Body.String())
	}
	return body.Error.Class
}

func TestCreateCaseRequiresAuthentication(t *testing.T) {
	harness := newWriteHarness(t)

	response := harness.do(t, http.MethodPost, "/api/v1/cases", map[string]any{
		"title": "api-manual-test case", "summary": "s", "severity": "low",
	}, false)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusUnauthorized, response.Body.String())
	}
	if class := decodeErrorClass(t, response); class != string(apperr.Unauthorized) {
		t.Fatalf("error class = %q, want %q", class, apperr.Unauthorized)
	}
}

func TestCreateCaseCreatesManualCase(t *testing.T) {
	harness := newWriteHarness(t)

	response := harness.do(t, http.MethodPost, "/api/v1/cases", map[string]any{
		"title":       "api-manual-test operator case",
		"summary":     "api-manual-test summary",
		"severity":    "high",
		"clusterFsid": "00000000-0000-4000-8000-000000000101",
	}, true)
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusCreated, response.Body.String())
	}
	var created cases.Case
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatalf("decode case: %v", err)
	}
	if created.ID == 0 || created.Status != cases.CaseStatusDetected || created.Source != cases.CaseSourceManual {
		t.Fatalf("case = %+v, want detected manual case", created)
	}
	if created.Severity != cases.CaseSeverityHigh {
		t.Fatalf("severity = %q, want high", created.Severity)
	}

	timeline := harness.do(t, http.MethodGet, "/api/v1/cases/"+strconv.FormatInt(created.ID, 10)+"/timeline", nil, false)
	if timeline.Code != http.StatusOK {
		t.Fatalf("timeline status = %d; body=%s", timeline.Code, timeline.Body.String())
	}
	var events []cases.TimelineEvent
	if err := json.NewDecoder(timeline.Body).Decode(&events); err != nil {
		t.Fatalf("decode timeline: %v", err)
	}
	if len(events) != 1 || events[0].Type != cases.TimelineEventCaseDetected {
		t.Fatalf("events = %+v, want one case_detected", events)
	}
	if events[0].Actor.Type != cases.TimelineActorUser || events[0].Actor.ID != "write-operator" || events[0].Actor.DisplayName != "Write Operator" {
		t.Fatalf("actor = %+v, want the verified operator", events[0].Actor)
	}
	if events[0].Payload["source"] != "manual" {
		t.Fatalf("payload = %+v, want source manual", events[0].Payload)
	}
}

func TestCreateCaseRejectsInvalidBodies(t *testing.T) {
	harness := newWriteHarness(t)

	for name, body := range map[string]map[string]any{
		"empty title":      {"title": "", "summary": "s", "severity": "low"},
		"empty summary":    {"title": "api-manual-test t", "summary": "", "severity": "low"},
		"unknown severity": {"title": "api-manual-test t", "summary": "s", "severity": "severe"},
		"missing severity": {"title": "api-manual-test t", "summary": "s"},
		"invalid cluster":  {"title": "api-manual-test t", "summary": "s", "severity": "low", "clusterFsid": "not-a-uuid"},
	} {
		t.Run(name, func(t *testing.T) {
			response := harness.do(t, http.MethodPost, "/api/v1/cases", body, true)
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusBadRequest, response.Body.String())
			}
			if class := decodeErrorClass(t, response); class != "InvalidRequest" {
				t.Fatalf("error class = %q, want InvalidRequest", class)
			}
		})
	}
}

func TestCreateCaseRequiresPostgresWriteSource(t *testing.T) {
	ctx := context.Background()
	issuer := devissuertest.Start(t)
	token := issuer.Token(t, "op", "Op")
	application, err := app.NewFromConfig(ctx, config.Config{
		FakeScenario: "reef-healthy-baremetal",
		ReadSource:   config.ReadSourceProvider,
		AgentMode:    config.AgentModeDisabled,
		OIDCIssuer:   devissuertest.IssuerURL,
		OIDCAudience: devissuertest.Audience,
		OIDCJWKSURL:  issuer.JWKSURL(),
	})
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	server := NewServer(application)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/cases", bytes.NewReader([]byte(`{"title":"t","summary":"s","severity":"low"}`)))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	server.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusUnprocessableEntity, response.Body.String())
	}
	if class := decodeErrorClass(t, response); class != string(apperr.Unsupported) {
		t.Fatalf("error class = %q, want %q", class, apperr.Unsupported)
	}
}

func TestTransitionCaseLifecycle(t *testing.T) {
	harness := newWriteHarness(t)
	created := createManualCaseForAPI(t, harness)

	triaged := harness.do(t, http.MethodPost, "/api/v1/cases/"+strconv.FormatInt(created, 10)+"/transitions", map[string]any{"status": "triaged"}, true)
	if triaged.Code != http.StatusOK {
		t.Fatalf("triage status = %d; body=%s", triaged.Code, triaged.Body.String())
	}
	var triagedCase cases.Case
	if err := json.NewDecoder(triaged.Body).Decode(&triagedCase); err != nil {
		t.Fatalf("decode triaged case: %v", err)
	}
	if triagedCase.Status != cases.CaseStatusTriaged {
		t.Fatalf("status = %q, want triaged", triagedCase.Status)
	}

	closed := harness.do(t, http.MethodPost, "/api/v1/cases/"+strconv.FormatInt(created, 10)+"/transitions", map[string]any{"status": "closed"}, true)
	if closed.Code != http.StatusOK {
		t.Fatalf("close status = %d; body=%s", closed.Code, closed.Body.String())
	}
	var closedCase cases.Case
	if err := json.NewDecoder(closed.Body).Decode(&closedCase); err != nil {
		t.Fatalf("decode closed case: %v", err)
	}
	if closedCase.ClosedAt == nil {
		t.Fatal("closed_at not set")
	}

	reopen := harness.do(t, http.MethodPost, "/api/v1/cases/"+strconv.FormatInt(created, 10)+"/transitions", map[string]any{"status": "triaged"}, true)
	if reopen.Code != http.StatusConflict {
		t.Fatalf("reopen status = %d, want %d; body=%s", reopen.Code, http.StatusConflict, reopen.Body.String())
	}
	if class := decodeErrorClass(t, reopen); class != string(apperr.Conflict) {
		t.Fatalf("error class = %q, want %q", class, apperr.Conflict)
	}
}

func TestTransitionCaseRejectsInvalidRequests(t *testing.T) {
	harness := newWriteHarness(t)
	created := createManualCaseForAPI(t, harness)
	path := "/api/v1/cases/" + strconv.FormatInt(created, 10) + "/transitions"

	invalid := harness.do(t, http.MethodPost, path, map[string]any{"status": "mitigated"}, true)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid status code = %d, want %d; body=%s", invalid.Code, http.StatusBadRequest, invalid.Body.String())
	}
	unauthorized := harness.do(t, http.MethodPost, path, map[string]any{"status": "triaged"}, false)
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want %d", unauthorized.Code, http.StatusUnauthorized)
	}
	missing := harness.do(t, http.MethodPost, "/api/v1/cases/999999/transitions", map[string]any{"status": "triaged"}, true)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing case status = %d, want %d", missing.Code, http.StatusNotFound)
	}
}

func TestAssignCaseEndpoint(t *testing.T) {
	harness := newWriteHarness(t)
	created := createManualCaseForAPI(t, harness)
	path := "/api/v1/cases/" + strconv.FormatInt(created, 10) + "/assignment"

	assigned := harness.do(t, http.MethodPost, path, map[string]any{"assignee": "target-subject", "assigneeDisplayName": "Target Operator"}, true)
	if assigned.Code != http.StatusOK {
		t.Fatalf("assign status = %d; body=%s", assigned.Code, assigned.Body.String())
	}
	var assignedCase cases.Case
	if err := json.NewDecoder(assigned.Body).Decode(&assignedCase); err != nil {
		t.Fatalf("decode assigned case: %v", err)
	}
	if assignedCase.Assignee != "target-subject" || assignedCase.AssigneeDisplayName != "Target Operator" {
		t.Fatalf("case = %+v, want assignee snapshot", assignedCase)
	}

	unassigned := harness.do(t, http.MethodPost, path, map[string]any{}, true)
	if unassigned.Code != http.StatusOK {
		t.Fatalf("unassign status = %d; body=%s", unassigned.Code, unassigned.Body.String())
	}
	var unassignedCase cases.Case
	if err := json.NewDecoder(unassigned.Body).Decode(&unassignedCase); err != nil {
		t.Fatalf("decode unassigned case: %v", err)
	}
	if unassignedCase.Assignee != "" || unassignedCase.AssigneeDisplayName != "" {
		t.Fatalf("case = %+v, want cleared assignee", unassignedCase)
	}

	timeline := harness.do(t, http.MethodGet, "/api/v1/cases/"+strconv.FormatInt(created, 10)+"/timeline", nil, false)
	var events []cases.TimelineEvent
	if err := json.NewDecoder(timeline.Body).Decode(&events); err != nil {
		t.Fatalf("decode timeline: %v", err)
	}
	if len(events) != 3 || events[1].Type != cases.TimelineEventCaseAssigned || events[2].Type != cases.TimelineEventCaseAssigned {
		t.Fatalf("events = %+v, want assignment events", events)
	}
}

func TestCaseNotesEndpoints(t *testing.T) {
	harness := newWriteHarness(t)
	created := createManualCaseForAPI(t, harness)
	path := "/api/v1/cases/" + strconv.FormatInt(created, 10) + "/notes"

	created_note := harness.do(t, http.MethodPost, path, map[string]any{"body": "api-manual-test investigated."}, true)
	if created_note.Code != http.StatusCreated {
		t.Fatalf("note status = %d; body=%s", created_note.Code, created_note.Body.String())
	}
	var note cases.CaseNote
	if err := json.NewDecoder(created_note.Body).Decode(&note); err != nil {
		t.Fatalf("decode note: %v", err)
	}
	if note.ID == 0 || note.AuthorID != "write-operator" || note.AuthorDisplayName != "Write Operator" {
		t.Fatalf("note = %+v, want author context", note)
	}

	listed := harness.do(t, http.MethodGet, path, nil, false)
	if listed.Code != http.StatusOK {
		t.Fatalf("list notes status = %d; body=%s", listed.Code, listed.Body.String())
	}
	var notes []cases.CaseNote
	if err := json.NewDecoder(listed.Body).Decode(&notes); err != nil {
		t.Fatalf("decode notes: %v", err)
	}
	if len(notes) != 1 || notes[0].Body != "api-manual-test investigated." {
		t.Fatalf("notes = %+v, want created note", notes)
	}

	invalid := harness.do(t, http.MethodPost, path, map[string]any{"body": ""}, true)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid note status = %d, want %d; body=%s", invalid.Code, http.StatusBadRequest, invalid.Body.String())
	}
}

func createManualCaseForAPI(t *testing.T, harness *writeHarness) int64 {
	t.Helper()
	response := harness.do(t, http.MethodPost, "/api/v1/cases", map[string]any{
		"title": "api-manual-test lifecycle", "summary": "api-manual-test summary", "severity": "low",
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

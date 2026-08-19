package api

import (
	"encoding/json"
	"github.com/tonymontoya/ceph-atlas/internal/apperr"
	"github.com/tonymontoya/ceph-atlas/internal/cases"
	"github.com/tonymontoya/ceph-atlas/internal/testdb"
	"net/http"
	"strconv"
	"testing"
)

type approvalRecordPayload struct {
	ID                 int64   `json:"id"`
	WorkflowInstanceID int64   `json:"workflowInstanceId"`
	GateID             string  `json:"gateId"`
	ApproverID         string  `json:"approverId"`
	ApproverName       string  `json:"approverDisplayName"`
	Reason             *string `json:"reason"`
}

// attachWorkflowForAPI attaches the replace-osd workflow to a fresh case and
// returns the parked instance's id.
func attachWorkflowForAPI(t *testing.T, harness *writeHarness) (caseID int64, instanceID int64) {
	t.Helper()
	caseID = createManualCaseForAPI(t, harness)
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

func approvalCountForInstance(t *testing.T, instanceID int64) int {
	t.Helper()
	db, _ := testdb.Open(t)
	rows, err := db.Query(`SELECT id FROM workflow_approvals WHERE workflow_instance_id = $1`, instanceID)
	if err != nil {
		t.Fatalf("query approvals: %v", err)
	}
	defer func() { _ = rows.Close() }()
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan approval: %v", err)
		}
		ids = append(ids, id)
	}
	return len(ids)
}

func listCaseWorkflowInstances(t *testing.T, harness *writeHarness, caseID int64) []attachWorkflowResponse {
	t.Helper()
	response := harness.do(t, http.MethodGet, "/api/v1/cases/"+strconv.FormatInt(caseID, 10)+"/workflows", nil, false)
	if response.Code != http.StatusOK {
		t.Fatalf("list workflows status = %d; body=%s", response.Code, response.Body.String())
	}
	var instances []attachWorkflowResponse
	if err := json.NewDecoder(response.Body).Decode(&instances); err != nil {
		t.Fatalf("decode instances: %v", err)
	}
	return instances
}

func timelineForCase(t *testing.T, harness *writeHarness, caseID int64) []cases.TimelineEvent {
	t.Helper()
	response := harness.do(t, http.MethodGet, "/api/v1/cases/"+strconv.FormatInt(caseID, 10)+"/timeline", nil, false)
	if response.Code != http.StatusOK {
		t.Fatalf("timeline status = %d; body=%s", response.Code, response.Body.String())
	}
	var events []cases.TimelineEvent
	if err := json.NewDecoder(response.Body).Decode(&events); err != nil {
		t.Fatalf("decode timeline: %v", err)
	}
	return events
}

func TestAttachWorkflowParksInstanceAtApprovalGate(t *testing.T) {
	harness := newWriteHarness(t)
	caseID, instanceID := attachWorkflowForAPI(t, harness)

	instances := listCaseWorkflowInstances(t, harness, caseID)
	if len(instances) != 1 {
		t.Fatalf("instances = %+v, want one", instances)
	}
	instance := instances[0]
	if instance.ID != instanceID {
		t.Fatalf("instance id = %d, want %d", instance.ID, instanceID)
	}
	if instance.State != "waiting_for_approval" {
		t.Fatalf("state = %q, want waiting_for_approval", instance.State)
	}
	if instance.CurrentStep == nil || *instance.CurrentStep != "approve-destroy" {
		t.Fatalf("currentStep = %v, want approve-destroy", instance.CurrentStep)
	}

	events := timelineForCase(t, harness, caseID)
	if len(events) != 4 {
		t.Fatalf("events = %d, want 4 (detected, attached, running, waiting); got %+v", len(events), events)
	}
	for _, event := range events[2:] {
		if event.Type != cases.TimelineEventWorkflowStateChanged {
			t.Fatalf("event = %+v, want workflow_state_changed", event)
		}
		if event.Actor.Type != cases.TimelineActorSystem || event.Actor.DisplayName != "Atlas" {
			t.Fatalf("actor = %+v, want the Atlas system actor", event.Actor)
		}
	}
	if events[2].Payload["previousState"] != "pending" || events[2].Payload["newState"] != "running" {
		t.Fatalf("first advancement payload = %+v", events[2].Payload)
	}
	third := events[3].Payload
	if third["previousState"] != "running" || third["newState"] != "waiting_for_approval" || third["pausedAtStep"] != "approve-destroy" {
		t.Fatalf("gate parking payload = %+v", third)
	}
}

func TestApproveWorkflowGateRequiresAuthentication(t *testing.T) {
	harness := newWriteHarness(t)
	_, instanceID := attachWorkflowForAPI(t, harness)
	path := "/api/v1/workflow-instances/" + strconv.FormatInt(instanceID, 10) + "/approvals"

	response := harness.do(t, http.MethodPost, path, map[string]any{"gateId": "approve-destroy"}, false)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusUnauthorized, response.Body.String())
	}
	if class := decodeErrorClass(t, response); class != string(apperr.Unauthorized) {
		t.Fatalf("error class = %q, want %q", class, apperr.Unauthorized)
	}
}

func TestApproveWorkflowGateRecordsApprovalAndResumes(t *testing.T) {
	harness := newWriteHarness(t)
	caseID, instanceID := attachWorkflowForAPI(t, harness)
	path := "/api/v1/workflow-instances/" + strconv.FormatInt(instanceID, 10) + "/approvals"

	response := harness.do(t, http.MethodPost, path, map[string]any{"gateId": "approve-destroy", "reason": "replacement approved"}, true)
	if response.Code != http.StatusCreated {
		t.Fatalf("approve status = %d, want %d; body=%s", response.Code, http.StatusCreated, response.Body.String())
	}
	var approval approvalRecordPayload
	if err := json.NewDecoder(response.Body).Decode(&approval); err != nil {
		t.Fatalf("decode approval: %v", err)
	}
	if approval.ID == 0 || approval.WorkflowInstanceID != instanceID || approval.GateID != "approve-destroy" {
		t.Fatalf("approval = %+v, want record bound to the gate", approval)
	}
	if approval.ApproverID != "write-operator" || approval.ApproverName != "Write Operator" {
		t.Fatalf("approver = %s / %s, want the verified operator", approval.ApproverID, approval.ApproverName)
	}
	if approval.Reason == nil || *approval.Reason != "replacement approved" {
		t.Fatalf("reason = %v, want snapshot", approval.Reason)
	}

	instances := listCaseWorkflowInstances(t, harness, caseID)
	if len(instances) != 1 || instances[0].State != "running" || instances[0].CurrentStep != nil {
		t.Fatalf("instance = %+v, want running with no current step", instances)
	}

	events := timelineForCase(t, harness, caseID)
	if len(events) != 5 || events[4].Type != cases.TimelineEventWorkflowStateChanged {
		t.Fatalf("events = %+v, want a fifth workflow_state_changed", events)
	}
	resume := events[4]
	if resume.Payload["previousState"] != "waiting_for_approval" || resume.Payload["newState"] != "running" {
		t.Fatalf("resume payload = %+v", resume.Payload)
	}
	if resume.Actor.Type != cases.TimelineActorUser || resume.Actor.ID != "write-operator" || resume.Actor.DisplayName != "Write Operator" {
		t.Fatalf("actor = %+v, want the verified operator", resume.Actor)
	}

	if count := approvalCountForInstance(t, instanceID); count != 1 {
		t.Fatalf("approval rows = %d, want exactly one durable record", count)
	}
}

func TestApproveWorkflowGateRejectsWrongGate(t *testing.T) {
	harness := newWriteHarness(t)
	_, instanceID := attachWorkflowForAPI(t, harness)
	path := "/api/v1/workflow-instances/" + strconv.FormatInt(instanceID, 10) + "/approvals"

	for name, gateID := range map[string]string{
		"job step":   "collect-evidence",
		"other gate": "not-the-gate",
	} {
		t.Run(name, func(t *testing.T) {
			response := harness.do(t, http.MethodPost, path, map[string]any{"gateId": gateID}, true)
			if response.Code != http.StatusConflict {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusConflict, response.Body.String())
			}
			if class := decodeErrorClass(t, response); class != string(apperr.Conflict) {
				t.Fatalf("error class = %q, want %q", class, apperr.Conflict)
			}
		})
	}

	if count := approvalCountForInstance(t, instanceID); count != 0 {
		t.Fatalf("approval rows = %d, want none recorded", count)
	}
}

func TestApproveWorkflowGateDoubleApproveIsIdempotentNoOp(t *testing.T) {
	harness := newWriteHarness(t)
	caseID, instanceID := attachWorkflowForAPI(t, harness)
	path := "/api/v1/workflow-instances/" + strconv.FormatInt(instanceID, 10) + "/approvals"

	first := harness.do(t, http.MethodPost, path, map[string]any{"gateId": "approve-destroy"}, true)
	if first.Code != http.StatusCreated {
		t.Fatalf("first approve status = %d; body=%s", first.Code, first.Body.String())
	}
	var firstApproval approvalRecordPayload
	if err := json.NewDecoder(first.Body).Decode(&firstApproval); err != nil {
		t.Fatalf("decode first approval: %v", err)
	}

	second := harness.do(t, http.MethodPost, path, map[string]any{"gateId": "approve-destroy"}, true)
	if second.Code != http.StatusOK {
		t.Fatalf("second approve status = %d, want %d; body=%s", second.Code, http.StatusOK, second.Body.String())
	}
	var secondApproval approvalRecordPayload
	if err := json.NewDecoder(second.Body).Decode(&secondApproval); err != nil {
		t.Fatalf("decode second approval: %v", err)
	}
	if secondApproval.ID != firstApproval.ID {
		t.Fatalf("second approval id = %d, want existing record %d", secondApproval.ID, firstApproval.ID)
	}

	if count := approvalCountForInstance(t, instanceID); count != 1 {
		t.Fatalf("approval rows = %d, want one", count)
	}
	events := timelineForCase(t, harness, caseID)
	stateChanges := 0
	for _, event := range events {
		if event.Type == cases.TimelineEventWorkflowStateChanged {
			stateChanges++
		}
	}
	if stateChanges != 3 {
		t.Fatalf("workflow_state_changed events = %d, want 3 (double approve adds none)", stateChanges)
	}
	instances := listCaseWorkflowInstances(t, harness, caseID)
	if len(instances) != 1 || instances[0].State != "running" {
		t.Fatalf("instance = %+v, want still running", instances)
	}
}

func TestApproveWorkflowGateRejectsInvalidRequests(t *testing.T) {
	harness := newWriteHarness(t)
	_, instanceID := attachWorkflowForAPI(t, harness)
	path := "/api/v1/workflow-instances/" + strconv.FormatInt(instanceID, 10) + "/approvals"

	for name, body := range map[string]map[string]any{
		"missing gate id": {"reason": "no gate"},
		"empty gate id":   {"gateId": ""},
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

	missing := harness.do(t, http.MethodPost, "/api/v1/workflow-instances/999999/approvals", map[string]any{"gateId": "approve-destroy"}, true)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("unknown instance status = %d, want %d; body=%s", missing.Code, http.StatusNotFound, missing.Body.String())
	}
	if class := decodeErrorClass(t, missing); class != string(apperr.NotFound) {
		t.Fatalf("error class = %q, want %q", class, apperr.NotFound)
	}
}

func TestApproveWorkflowGateRequiresPostgresWriteSource(t *testing.T) {
	harness := newFakeWriteHarness(t)

	response := harness.do(t, http.MethodPost, "/api/v1/workflow-instances/1/approvals", map[string]any{"gateId": "approve-destroy"}, true)

	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusUnprocessableEntity, response.Body.String())
	}
	if class := decodeErrorClass(t, response); class != string(apperr.Unsupported) {
		t.Fatalf("error class = %q, want %q", class, apperr.Unsupported)
	}
}

func TestListCaseWorkflowsRequiresPostgresReadSource(t *testing.T) {
	harness := newFakeWriteHarness(t)

	response := harness.do(t, http.MethodGet, "/api/v1/cases/1/workflows", nil, false)

	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusUnprocessableEntity, response.Body.String())
	}
	if class := decodeErrorClass(t, response); class != string(apperr.Unsupported) {
		t.Fatalf("error class = %q, want %q", class, apperr.Unsupported)
	}
}

func TestListCaseWorkflowsUnknownCase(t *testing.T) {
	harness := newWriteHarness(t)

	response := harness.do(t, http.MethodGet, "/api/v1/cases/999999/workflows", nil, false)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusNotFound, response.Body.String())
	}
	if class := decodeErrorClass(t, response); class != string(apperr.NotFound) {
		t.Fatalf("error class = %q, want %q", class, apperr.NotFound)
	}
}

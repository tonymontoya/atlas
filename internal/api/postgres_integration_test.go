package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tonymontoya/ceph-atlas/internal/app"
	"github.com/tonymontoya/ceph-atlas/internal/apperr"
	"github.com/tonymontoya/ceph-atlas/internal/casedetection"
	"github.com/tonymontoya/ceph-atlas/internal/cases"
	"github.com/tonymontoya/ceph-atlas/internal/config"
	"github.com/tonymontoya/ceph-atlas/internal/inventory"
	"github.com/tonymontoya/ceph-atlas/internal/inventorysync"
	"github.com/tonymontoya/ceph-atlas/internal/store"
	"github.com/tonymontoya/ceph-atlas/internal/testdb"
)

func TestPostgresReadSourceUsesPersistedInventory(t *testing.T) {
	ctx := context.Background()
	db, databaseURL := testdb.Open(t)
	resetInventoryTables(t, db)

	if _, err := inventorysync.RunFakeOnce(ctx, store.NewPostgres(db), inventorysync.Options{
		Scenario:   "reef-osd-down-baremetal",
		ObservedAt: time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("sync fake inventory: %v", err)
	}

	application, err := app.NewFromConfig(ctx, config.Config{
		DatabaseURL: databaseURL,
		ReadSource:  "postgres",
		AgentMode:   config.AgentModeDisabled,
	})
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	t.Cleanup(func() { _ = application.Close() })

	server := NewServer(application)

	indexResponse := serve(server, http.MethodGet, "/api/v1/clusters")
	if indexResponse.Code != http.StatusOK {
		t.Fatalf("cluster index status = %d, want %d; body=%s", indexResponse.Code, http.StatusOK, indexResponse.Body.String())
	}
	var index struct {
		Total    int `json:"total"`
		Clusters []struct {
			FSID         *string `json:"fsid"`
			Name         string  `json:"name"`
			HealthStatus *string `json:"healthStatus"`
		} `json:"clusters"`
	}
	if err := json.NewDecoder(indexResponse.Body).Decode(&index); err != nil {
		t.Fatalf("decode cluster index: %v", err)
	}
	if index.Total != 1 || len(index.Clusters) != 1 {
		t.Fatalf("cluster index = %+v, want exactly the synced cluster", index)
	}
	if index.Clusters[0].FSID == nil || *index.Clusters[0].FSID != "00000000-0000-4000-8000-000000000102" {
		t.Fatalf("index fsid = %v, want the synced cluster", index.Clusters[0].FSID)
	}
	if index.Clusters[0].HealthStatus == nil || *index.Clusters[0].HealthStatus != "HEALTH_WARN" {
		t.Fatalf("index health = %v, want HEALTH_WARN", index.Clusters[0].HealthStatus)
	}

	healthResponse := serve(server, http.MethodGet, "/api/v1/clusters/00000000-0000-4000-8000-000000000102/health")
	if healthResponse.Code != http.StatusOK {
		t.Fatalf("health status = %d, want %d; body=%s", healthResponse.Code, http.StatusOK, healthResponse.Body.String())
	}
	var health inventory.Health
	if err := json.NewDecoder(healthResponse.Body).Decode(&health); err != nil {
		t.Fatalf("decode health: %v", err)
	}
	if health.Status != inventory.HealthWarn || len(health.Checks) != 1 {
		t.Fatalf("health = %+v, want warning with one check", health)
	}

	osdsResponse := serve(server, http.MethodGet, "/api/v1/clusters/00000000-0000-4000-8000-000000000102/osds")
	if osdsResponse.Code != http.StatusOK {
		t.Fatalf("osds status = %d, want %d; body=%s", osdsResponse.Code, http.StatusOK, osdsResponse.Body.String())
	}
	var osds []inventory.OSD
	if err := json.NewDecoder(osdsResponse.Body).Decode(&osds); err != nil {
		t.Fatalf("decode osds: %v", err)
	}
	if len(osds) != 2 || osds[1].Up {
		t.Fatalf("osds = %+v, want two OSDs with second down", osds)
	}
}

func TestPostgresReadSourceServesMultipleClustersWithoutBleeding(t *testing.T) {
	ctx := context.Background()
	db, databaseURL := testdb.Open(t)
	resetInventoryTables(t, db)

	writer := store.NewPostgres(db)
	scenarios := []struct {
		scenario string
		fsid     string
	}{
		{"reef-healthy-baremetal", "00000000-0000-4000-8000-000000000101"},
		{"reef-osd-down-baremetal", "00000000-0000-4000-8000-000000000102"},
	}
	for _, sync := range scenarios {
		if _, err := inventorysync.RunFakeOnce(ctx, writer, inventorysync.Options{
			Scenario:   sync.scenario,
			ObservedAt: time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC),
		}); err != nil {
			t.Fatalf("sync %s: %v", sync.scenario, err)
		}
	}

	application, err := app.NewFromConfig(ctx, config.Config{
		DatabaseURL: databaseURL,
		ReadSource:  "postgres",
		AgentMode:   config.AgentModeDisabled,
	})
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	t.Cleanup(func() { _ = application.Close() })

	server := NewServer(application)

	indexResponse := serve(server, http.MethodGet, "/api/v1/clusters")
	if indexResponse.Code != http.StatusOK {
		t.Fatalf("cluster index status = %d; body=%s", indexResponse.Code, indexResponse.Body.String())
	}
	var index struct {
		Total    int `json:"total"`
		Clusters []struct {
			FSID         *string `json:"fsid"`
			HealthStatus *string `json:"healthStatus"`
		} `json:"clusters"`
	}
	if err := json.NewDecoder(indexResponse.Body).Decode(&index); err != nil {
		t.Fatalf("decode cluster index: %v", err)
	}
	if index.Total != 2 {
		t.Fatalf("cluster index total = %d, want both synced clusters", index.Total)
	}

	// Each cluster's OSDs come from its own latest snapshot.
	healthyOSDs := serve(server, http.MethodGet, "/api/v1/clusters/00000000-0000-4000-8000-000000000101/osds")
	if healthyOSDs.Code != http.StatusOK {
		t.Fatalf("healthy osds status = %d", healthyOSDs.Code)
	}
	var healthy []inventory.OSD
	if err := json.NewDecoder(healthyOSDs.Body).Decode(&healthy); err != nil {
		t.Fatalf("decode healthy osds: %v", err)
	}
	for _, osd := range healthy {
		if !osd.Up {
			t.Fatalf("healthy cluster lists down OSD %d — cross-cluster bleeding", osd.ID)
		}
	}

	downOSDs := serve(server, http.MethodGet, "/api/v1/clusters/00000000-0000-4000-8000-000000000102/osds")
	if downOSDs.Code != http.StatusOK {
		t.Fatalf("osd-down osds status = %d", downOSDs.Code)
	}
	var down []inventory.OSD
	if err := json.NewDecoder(downOSDs.Body).Decode(&down); err != nil {
		t.Fatalf("decode osd-down osds: %v", err)
	}
	if len(down) != 2 || down[1].Up {
		t.Fatalf("osd-down cluster osds = %+v, want two with the second down", down)
	}

	unknown := serve(server, http.MethodGet, "/api/v1/clusters/00000000-0000-4000-8000-0000000009ff/osds")
	if unknown.Code != http.StatusNotFound {
		t.Fatalf("unknown cluster osds status = %d, want 404", unknown.Code)
	}
}

func TestPostgresReadSourceReturnsNotFoundForEmptyReadModel(t *testing.T) {
	ctx := context.Background()
	db, databaseURL := testdb.Open(t)
	resetInventoryTables(t, db)

	application, err := app.NewFromConfig(ctx, config.Config{
		DatabaseURL: databaseURL,
		ReadSource:  "postgres",
		AgentMode:   config.AgentModeDisabled,
	})
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	t.Cleanup(func() { _ = application.Close() })

	server := NewServer(application)
	paths := []string{
		"/api/v1/clusters/00000000-0000-4000-8000-000000000101/health",
		"/api/v1/clusters/00000000-0000-4000-8000-000000000101/osds",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			response := serve(server, http.MethodGet, path)
			if response.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusNotFound, response.Body.String())
			}
			var body struct {
				Error struct {
					Class string `json:"class"`
				} `json:"error"`
			}
			if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
				t.Fatalf("decode error: %v", err)
			}
			if body.Error.Class != string(apperr.NotFound) {
				t.Fatalf("error class = %q, want %q", body.Error.Class, apperr.NotFound)
			}
		})
	}
}

func TestPostgresReadSourceListsEmptySyncRuns(t *testing.T) {
	ctx := context.Background()
	db, _ := testdb.Open(t)
	resetInventoryTables(t, db)

	server := newPostgresServer(t, ctx)
	response := serve(server, http.MethodGet, "/api/v1/inventory-sync-runs")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	if strings.TrimSpace(response.Body.String()) != "[]" {
		t.Fatalf("body = %s, want []", response.Body.String())
	}
	var runs []map[string]any
	if err := json.NewDecoder(response.Body).Decode(&runs); err != nil {
		t.Fatalf("decode runs: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("run count = %d, want 0", len(runs))
	}
}

func TestPostgresReadSourceListsSucceededAndFailedSyncRuns(t *testing.T) {
	ctx := context.Background()
	db, _ := testdb.Open(t)
	resetInventoryTables(t, db)

	writer := store.NewPostgres(db)
	if _, err := inventorysync.RunFakeOnce(ctx, writer, inventorysync.Options{
		Scenario:   "reef-osd-down-baremetal",
		ObservedAt: time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("sync fake inventory: %v", err)
	}
	if _, err := inventorysync.RunFakeOnce(ctx, writer, inventorysync.Options{
		Scenario:   "missing",
		ObservedAt: time.Date(2026, 8, 13, 12, 1, 0, 0, time.UTC),
	}); err == nil {
		t.Fatal("expected failed fake sync")
	}

	server := newPostgresServer(t, ctx)
	response := serve(server, http.MethodGet, "/api/v1/inventory-sync-runs")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	var runs []struct {
		Provider     string `json:"provider"`
		Scenario     string `json:"scenario"`
		Status       string `json:"status"`
		SnapshotID   *int64 `json:"snapshotId,omitempty"`
		ErrorClass   string `json:"errorClass,omitempty"`
		ErrorMessage string `json:"errorMessage,omitempty"`
	}
	if err := json.NewDecoder(response.Body).Decode(&runs); err != nil {
		t.Fatalf("decode runs: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("run count = %d, want 2; runs=%+v", len(runs), runs)
	}
	if runs[0].Status != "failed" || runs[0].ErrorClass != "Unavailable" || runs[0].ErrorMessage == "" {
		t.Fatalf("first run = %+v, want failed unavailable run", runs[0])
	}
	if runs[1].Status != "succeeded" || runs[1].SnapshotID == nil {
		t.Fatalf("second run = %+v, want succeeded run with snapshot", runs[1])
	}
}

func TestPostgresReadSourceListsAndGetsSeedCases(t *testing.T) {
	ctx := context.Background()
	server := newPostgresServer(t, ctx)

	listResponse := serve(server, http.MethodGet, "/api/v1/cases")
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d; body=%s", listResponse.Code, http.StatusOK, listResponse.Body.String())
	}
	var listed []cases.Case
	if err := json.NewDecoder(listResponse.Body).Decode(&listed); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listed) < 2 {
		t.Fatalf("case count = %d, want at least 2; cases=%+v", len(listed), listed)
	}
	if listed[0].Title != "Review weekly capacity trend" || listed[0].Status != cases.CaseStatusTriaged {
		t.Fatalf("first case = %+v, want newest seeded triaged case", listed[0])
	}

	getResponse := serve(server, http.MethodGet, "/api/v1/cases/"+strconv.FormatInt(listed[0].ID, 10))
	if getResponse.Code != http.StatusOK {
		t.Fatalf("get status = %d, want %d; body=%s", getResponse.Code, http.StatusOK, getResponse.Body.String())
	}
	var item cases.Case
	if err := json.NewDecoder(getResponse.Body).Decode(&item); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	if item.ID != listed[0].ID || item.Title != listed[0].Title {
		t.Fatalf("case = %+v, want listed case %+v", item, listed[0])
	}
}

func TestPostgresReadSourceListsSeedCaseTimeline(t *testing.T) {
	ctx := context.Background()
	server := newPostgresServer(t, ctx)

	listResponse := serve(server, http.MethodGet, "/api/v1/cases")
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d; body=%s", listResponse.Code, http.StatusOK, listResponse.Body.String())
	}
	var listed []cases.Case
	if err := json.NewDecoder(listResponse.Body).Decode(&listed); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listed) == 0 {
		t.Fatal("expected seeded cases")
	}

	timelineResponse := serve(server, http.MethodGet, "/api/v1/cases/"+strconv.FormatInt(listed[0].ID, 10)+"/timeline")
	if timelineResponse.Code != http.StatusOK {
		t.Fatalf("timeline status = %d, want %d; body=%s", timelineResponse.Code, http.StatusOK, timelineResponse.Body.String())
	}
	var timeline []cases.TimelineEvent
	if err := json.NewDecoder(timelineResponse.Body).Decode(&timeline); err != nil {
		t.Fatalf("decode timeline: %v", err)
	}
	if len(timeline) != 2 {
		t.Fatalf("timeline event count = %d, want 2; timeline=%+v", len(timeline), timeline)
	}
	if timeline[0].CaseID != listed[0].ID || timeline[0].Type != cases.TimelineEventCaseDetected {
		t.Fatalf("first timeline event = %+v, want case_detected for case %d", timeline[0], listed[0].ID)
	}
	if timeline[1].Type != cases.TimelineEventCaseTriaged {
		t.Fatalf("second timeline event = %+v, want case_triaged", timeline[1])
	}
}

func TestPostgresReadSourceReturnsNotFoundForMissingCase(t *testing.T) {
	ctx := context.Background()
	server := newPostgresServer(t, ctx)

	for _, path := range []string{"/api/v1/cases/0", "/api/v1/cases/not-a-number", "/api/v1/cases/999999"} {
		t.Run(path, func(t *testing.T) {
			response := serve(server, http.MethodGet, path)
			if response.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusNotFound, response.Body.String())
			}
			var body struct {
				Error struct {
					Class string `json:"class"`
				} `json:"error"`
			}
			if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
				t.Fatalf("decode error: %v", err)
			}
			if body.Error.Class != string(apperr.NotFound) {
				t.Fatalf("error class = %q, want %q", body.Error.Class, apperr.NotFound)
			}
		})
	}
}

func TestPostgresReadSourceReturnsNotFoundForMissingCaseTimeline(t *testing.T) {
	ctx := context.Background()
	server := newPostgresServer(t, ctx)

	for _, path := range []string{"/api/v1/cases/0/timeline", "/api/v1/cases/not-a-number/timeline", "/api/v1/cases/999999/timeline"} {
		t.Run(path, func(t *testing.T) {
			response := serve(server, http.MethodGet, path)
			if response.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusNotFound, response.Body.String())
			}
			var body struct {
				Error struct {
					Class string `json:"class"`
				} `json:"error"`
			}
			if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
				t.Fatalf("decode error: %v", err)
			}
			if body.Error.Class != string(apperr.NotFound) {
				t.Fatalf("error class = %q, want %q", body.Error.Class, apperr.NotFound)
			}
		})
	}
}

func newPostgresServer(t *testing.T, ctx context.Context) *Server {
	t.Helper()
	application, err := app.NewFromConfig(ctx, config.Config{
		DatabaseURL: testdb.URL(t),
		ReadSource:  "postgres",
		AgentMode:   config.AgentModeDisabled,
	})
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	t.Cleanup(func() {
		_ = application.Close()
	})
	return NewServer(application)
}

func resetInventoryTables(t *testing.T, db *sql.DB) {
	t.Helper()
	testdb.DeleteSyncRuns(t, db, "TRUE")
	testdb.DeleteClusters(t, db, "TRUE")
}

func serve(server *Server, method, path string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, nil)
	response := httptest.NewRecorder()
	server.Routes().ServeHTTP(response, request)
	return response
}

func TestPostgresReadSourceListsAlertEvaluationRuns(t *testing.T) {
	ctx := context.Background()
	db, _ := testdb.Open(t)
	resetDetectionTables(t, db)
	t.Cleanup(func() { resetDetectionTables(t, db) })

	writer := store.NewPostgres(db)
	if _, err := casedetection.RunFakeOnce(ctx, writer, casedetection.Options{
		Scenario:    "osd-down-alert",
		EvaluatedAt: time.Date(2026, 8, 14, 9, 20, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("run fake alert evaluation: %v", err)
	}
	if _, err := casedetection.RunFakeOnce(ctx, writer, casedetection.Options{
		Scenario: "provider-unauthorized",
	}); err == nil {
		t.Fatal("expected failed fake alert evaluation")
	}

	server := newPostgresServer(t, ctx)
	response := serve(server, http.MethodGet, "/api/v1/alert-evaluation-runs")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	var runs []struct {
		Provider        string `json:"provider"`
		Scenario        string `json:"scenario"`
		Status          string `json:"status"`
		ErrorClass      string `json:"errorClass,omitempty"`
		ErrorMessage    string `json:"errorMessage,omitempty"`
		AlertsEvaluated *int   `json:"alertsEvaluated,omitempty"`
		CasesCreated    *int   `json:"casesCreated,omitempty"`
	}
	if err := json.NewDecoder(response.Body).Decode(&runs); err != nil {
		t.Fatalf("decode runs: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("run count = %d, want 2; runs=%+v", len(runs), runs)
	}
	if runs[0].Status != "failed" || runs[0].ErrorClass != "Unauthorized" || runs[0].ErrorMessage == "" {
		t.Fatalf("first run = %+v, want failed unauthorized run", runs[0])
	}
	if runs[1].Status != "succeeded" || runs[1].Scenario != "osd-down-alert" {
		t.Fatalf("second run = %+v, want succeeded osd-down-alert run", runs[1])
	}
	if runs[1].AlertsEvaluated == nil || *runs[1].AlertsEvaluated != 1 {
		t.Fatalf("second run alertsEvaluated = %+v, want 1", runs[1].AlertsEvaluated)
	}
	if runs[1].CasesCreated == nil || *runs[1].CasesCreated != 1 {
		t.Fatalf("second run casesCreated = %+v, want 1", runs[1].CasesCreated)
	}
}

func resetDetectionTables(t *testing.T, db *sql.DB) {
	t.Helper()
	testdb.DeleteCases(t, db, "title = 'CephOSDDown on osd=1'")
	testdb.DeleteAlertRuns(t, db, "TRUE")
}

func TestAlertEvaluationRunsEndpointRequiresPostgresReadSource(t *testing.T) {
	server := NewServer(app.New(config.Config{FakeScenario: "reef-healthy-baremetal"}))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/alert-evaluation-runs", nil)
	response := httptest.NewRecorder()

	server.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusUnprocessableEntity, response.Body.String())
	}
	var body struct {
		Error struct {
			Class string `json:"class"`
		} `json:"error"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if body.Error.Class != string(apperr.Unsupported) {
		t.Fatalf("error class = %q, want %q", body.Error.Class, apperr.Unsupported)
	}
}

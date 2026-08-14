package casedetection

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tonymontoya/ceph-atlas/internal/cases"
	"github.com/tonymontoya/ceph-atlas/internal/observability"
)

func firingOSDDownAlert() observability.Alert {
	return observability.Alert{
		Name:        "CephOSDDown",
		Severity:    "warning",
		Labels:      map[string]string{"cluster": "reef-baremetal-osd-down", "osd": "1"},
		Annotations: map[string]string{"summary": "OSD 1 is down"},
		StartedAt:   time.Date(2026, 8, 14, 9, 15, 0, 0, time.UTC),
		State:       observability.AlertStateFiring,
		Source:      "prometheus",
	}
}

func TestMapSeverity(t *testing.T) {
	tests := []struct {
		alertSeverity string
		want          cases.CaseSeverity
	}{
		{"critical", cases.CaseSeverityCritical},
		{"warning", cases.CaseSeverityHigh},
		{"info", cases.CaseSeverityLow},
		{"none", cases.CaseSeverityMedium},
		{"", cases.CaseSeverityMedium},
		{"sev0", cases.CaseSeverityMedium},
		{"  WARNING  ", cases.CaseSeverityHigh},
	}
	for _, tt := range tests {
		if got := MapSeverity(tt.alertSeverity); got != tt.want {
			t.Errorf("MapSeverity(%q) = %q, want %q", tt.alertSeverity, got, tt.want)
		}
	}
}

func TestTitle(t *testing.T) {
	alert := firingOSDDownAlert()
	if got, want := Title(alert), "CephOSDDown on osd=1"; got != want {
		t.Fatalf("Title = %q, want %q", got, want)
	}
	noTargets := alert
	noTargets.Labels = map[string]string{"cluster": "reef-baremetal-osd-down"}
	if got, want := Title(noTargets), "CephOSDDown"; got != want {
		t.Fatalf("Title with no target labels = %q, want %q", got, want)
	}
}

func TestSummaryPrefersAnnotation(t *testing.T) {
	alert := firingOSDDownAlert()
	if got, want := Summary(alert), "OSD 1 is down"; got != want {
		t.Fatalf("Summary = %q, want %q", got, want)
	}
	noAnnotation := alert
	noAnnotation.Annotations = nil
	if got, want := Summary(noAnnotation), "prometheus alert CephOSDDown (warning) firing on cluster reef-baremetal-osd-down"; got != want {
		t.Fatalf("Summary fallback = %q, want %q", got, want)
	}
}

func TestSignal(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"CephOSDDown", "CEPH_OSD_DOWN"},
		{"CephOSDFlapping", "CEPH_OSD_FLAPPING"},
		{"DiskAlmostFull", "DISK_ALMOST_FULL"},
		{"HostPackageManagerFailure", "HOST_PACKAGE_MANAGER_FAILURE"},
	}
	for _, tt := range tests {
		alert := observability.Alert{Name: tt.name}
		if got := Signal(alert); got != tt.want {
			t.Errorf("Signal(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestNormalizeMatchesGoldenFixture(t *testing.T) {
	alertsPath := filepath.Join("..", "..", "dev", "fixtures", "prometheus", "osd-down-alert", "alerts.json")
	data, err := os.ReadFile(alertsPath)
	if err != nil {
		t.Fatalf("read alerts fixture: %v", err)
	}
	var alerts []observability.Alert
	if err := json.Unmarshal(data, &alerts); err != nil {
		t.Fatalf("parse alerts fixture: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("alerts fixture holds %d alerts, want 1", len(alerts))
	}

	goldenPath := filepath.Join("..", "..", "dev", "fixtures", "normalized", "case-input", "osd-down-alert.json")
	goldenData, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden fixture: %v", err)
	}
	var want CaseInput
	if err := json.Unmarshal(goldenData, &want); err != nil {
		t.Fatalf("parse golden fixture: %v", err)
	}

	got := Normalize(alerts[0])
	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal normalized input: %v", err)
	}
	var gotRenormalized CaseInput
	if err := json.Unmarshal(gotJSON, &gotRenormalized); err != nil {
		t.Fatalf("re-parse normalized input: %v", err)
	}
	if gotRenormalized != want {
		t.Fatalf("Normalize(alerts fixture) = %+v, want golden %+v", gotRenormalized, want)
	}
}

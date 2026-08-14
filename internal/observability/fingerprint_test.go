package observability

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func alertWithLabels(labels map[string]string) Alert {
	return Alert{
		Name:        "CephOSDDown",
		Severity:    "warning",
		Labels:      labels,
		Annotations: map[string]string{"summary": "OSD 1 is down"},
		StartedAt:   time.Date(2026, 8, 14, 9, 15, 0, 0, time.UTC),
		State:       AlertStateFiring,
		Source:      "prometheus",
	}
}

func TestDeriveFingerprintStableAcrossLabelOrder(t *testing.T) {
	first := DeriveFingerprint(alertWithLabels(map[string]string{"cluster": "a", "osd": "1"}))
	second := DeriveFingerprint(alertWithLabels(map[string]string{"osd": "1", "cluster": "a"}))
	if first != second {
		t.Fatalf("fingerprint changed with label order: %q vs %q", first, second)
	}
}

func TestDeriveFingerprintIgnoresContextLabels(t *testing.T) {
	base := alertWithLabels(map[string]string{"cluster": "a", "osd": "1"})
	base.Severity = "critical"
	regraded := alertWithLabels(map[string]string{"cluster": "a", "osd": "1", "severity": "critical", "summary": "worse now"})
	if DeriveFingerprint(base) != DeriveFingerprint(regraded) {
		t.Fatal("fingerprint changed when only context labels or severity changed")
	}
}

func TestDeriveFingerprintIncludesNameAndTargetLabels(t *testing.T) {
	base := alertWithLabels(map[string]string{"cluster": "a", "osd": "1"})

	otherOSD := alertWithLabels(map[string]string{"cluster": "a", "osd": "2"})
	if DeriveFingerprint(base) == DeriveFingerprint(otherOSD) {
		t.Fatal("fingerprint collided across target objects")
	}

	renamed := base
	renamed.Name = "CephOSDFlapping"
	if DeriveFingerprint(base) == DeriveFingerprint(renamed) {
		t.Fatal("fingerprint collided across alert names")
	}

	otherCluster := alertWithLabels(map[string]string{"cluster": "b", "osd": "1"})
	if DeriveFingerprint(base) == DeriveFingerprint(otherCluster) {
		t.Fatal("fingerprint collided across clusters")
	}
}

func TestFingerprintFixturesAreWellFormed(t *testing.T) {
	alertsPath := filepath.Join("..", "..", "dev", "fixtures", "prometheus", "osd-down-alert", "alerts.json")
	data, err := os.ReadFile(alertsPath)
	if err != nil {
		t.Fatalf("read alerts fixture: %v", err)
	}
	var alerts []Alert
	if err := json.Unmarshal(data, &alerts); err != nil {
		t.Fatalf("parse alerts fixture: %v", err)
	}
	for _, alert := range alerts {
		if DeriveFingerprint(alert) == "" {
			t.Fatalf("fixture alert %q produced empty fingerprint", alert.Name)
		}
	}
}

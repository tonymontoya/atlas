package main

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tonymontoya/ceph-atlas/internal/config"
	"github.com/tonymontoya/ceph-atlas/internal/providers/prometheus/promtest"
)

func TestBuildAlertSourceFakeMode(t *testing.T) {
	provider, name, err := buildAlertSource(config.Config{
		AlertSource:       config.AlertSourceFake,
		FakeAlertScenario: "osd-down-alert",
	})
	if err != nil {
		t.Fatalf("buildAlertSource returned error: %v", err)
	}
	if name != "fake" {
		t.Fatalf("provider name = %q, want fake", name)
	}
	alerts, err := provider.CurrentAlerts(context.Background())
	if err != nil {
		t.Fatalf("CurrentAlerts returned error: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("CurrentAlerts returned %d alerts, want the osd-down-alert fixture alert", len(alerts))
	}
}

func TestBuildAlertSourcePrometheusMode(t *testing.T) {
	server := promtest.New(t, promtest.ModeSuccess)

	provider, name, err := buildAlertSource(config.Config{
		AlertSource:           config.AlertSourcePrometheus,
		PrometheusURL:         server.URL(),
		PrometheusBearerToken: promtest.Token,
	})
	if err != nil {
		t.Fatalf("buildAlertSource returned error: %v", err)
	}
	if name != "prometheus" {
		t.Fatalf("provider name = %q, want prometheus", name)
	}
	alerts, err := provider.CurrentAlerts(context.Background())
	if err != nil {
		t.Fatalf("CurrentAlerts returned error: %v", err)
	}
	if len(alerts) != 1 || alerts[0].Name != promtest.AlertName {
		t.Fatalf("CurrentAlerts = %+v, want the promtest firing alert", alerts)
	}
}

func TestBuildAlertSourcePrometheusModeRejectsMissingURL(t *testing.T) {
	_, _, err := buildAlertSource(config.Config{
		AlertSource: config.AlertSourcePrometheus,
	})
	if err == nil {
		t.Fatal("expected error for missing URL")
	}
	if !strings.Contains(err.Error(), "BaseURL") {
		t.Fatalf("error %q does not name the missing field", err)
	}
}

func TestBuildAlertSourceRejectsUnknownSource(t *testing.T) {
	_, _, err := buildAlertSource(config.Config{
		AlertSource: config.AlertSource("bogus"),
	})
	if err == nil {
		t.Fatal("expected error for unknown alert source")
	}
	if !strings.Contains(err.Error(), "bogus") {
		t.Fatalf("error %q does not name the rejected source", err)
	}
}

func TestRunLoopEvaluatesThenStopsOnContextEnd(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var mu sync.Mutex
	count := 0
	evaluate := func() {
		mu.Lock()
		count++
		if count == 3 {
			cancel()
		}
		mu.Unlock()
	}

	runLoop(ctx, time.Millisecond, evaluate)

	mu.Lock()
	defer mu.Unlock()
	if count < 3 {
		t.Fatalf("runLoop evaluated %d times, want at least 3 before returning", count)
	}
}

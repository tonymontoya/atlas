package casedetection

import (
	"context"
	"errors"
	"time"

	"github.com/tonymontoya/ceph-atlas/internal/apperr"
	"github.com/tonymontoya/ceph-atlas/internal/cases"
	"github.com/tonymontoya/ceph-atlas/internal/observability"
	"github.com/tonymontoya/ceph-atlas/internal/providers"
	"github.com/tonymontoya/ceph-atlas/internal/providers/fake"
	"github.com/tonymontoya/ceph-atlas/internal/store"
)

type Writer interface {
	BeginAlertEvaluationRun(ctx context.Context, run store.BeginEvaluationRun) (int64, error)
	DetectFromAlerts(ctx context.Context, detection store.AlertDetection) (store.DetectionResult, error)
	SucceedAlertEvaluationRun(ctx context.Context, result store.EvaluationRunResult) error
	FailAlertEvaluationRun(ctx context.Context, failure store.EvaluationRunFailure) error
}

type Options struct {
	Scenario    string
	EvaluatedAt time.Time
}

// RunOptions names one evaluation pass for any alert source: Provider is
// the run-table name ("fake", "prometheus"), Scenario is recorded only
// for fixture-backed sources.
type RunOptions struct {
	Provider    string
	Scenario    string
	EvaluatedAt time.Time
}

func RunFakeOnce(ctx context.Context, writer Writer, opts Options) (store.DetectionResult, error) {
	return RunOnce(ctx, writer, fake.NewObservability(fake.DefaultFixtureRoot(), opts.Scenario), RunOptions{
		Provider:    "fake",
		Scenario:    opts.Scenario,
		EvaluatedAt: opts.EvaluatedAt,
	})
}

// RunOnce evaluates one pass of alerts from any ObservabilityProvider
// (ADR-0027): the fake fixtures and a real Prometheus flow through the
// same normalization, fingerprint dedup, and Case creation.
func RunOnce(ctx context.Context, writer Writer, provider providers.ObservabilityProvider, opts RunOptions) (store.DetectionResult, error) {
	runID, err := writer.BeginAlertEvaluationRun(ctx, store.BeginEvaluationRun{
		Provider: opts.Provider,
		Scenario: opts.Scenario,
	})
	if err != nil {
		return store.DetectionResult{}, err
	}

	result, err := runOnce(ctx, writer, provider, opts)
	if err != nil {
		failErr := writer.FailAlertEvaluationRun(ctx, failureFromError(runID, err))
		if failErr != nil {
			return store.DetectionResult{}, errors.Join(err, failErr)
		}
		return store.DetectionResult{}, err
	}
	if err := writer.SucceedAlertEvaluationRun(ctx, store.EvaluationRunResult{
		RunID:           runID,
		AlertsEvaluated: result.AlertsEvaluated,
		CasesCreated:    result.CasesCreated,
	}); err != nil {
		return store.DetectionResult{}, err
	}
	return result, nil
}

func runOnce(ctx context.Context, writer Writer, provider providers.ObservabilityProvider, opts RunOptions) (store.DetectionResult, error) {
	alerts, err := provider.CurrentAlerts(ctx)
	if err != nil {
		return store.DetectionResult{}, err
	}

	evaluatedAt := opts.EvaluatedAt
	if evaluatedAt.IsZero() {
		evaluatedAt = time.Now().UTC()
	}

	return writer.DetectFromAlerts(ctx, store.AlertDetection{
		Provider:    opts.Provider,
		Scenario:    opts.Scenario,
		EvaluatedAt: evaluatedAt,
		Candidates:  candidatesFromAlerts(alerts),
	})
}

func candidatesFromAlerts(alerts []observability.Alert) []store.AlertCandidate {
	candidates := make([]store.AlertCandidate, 0, len(alerts))
	for _, alert := range alerts {
		if alert.Name == "" {
			continue
		}
		if _, err := cases.ParseCaseSource(alert.Source); err != nil {
			continue
		}
		input := Normalize(alert)
		candidates = append(candidates, store.AlertCandidate{
			Fingerprint:  string(input.Fingerprint),
			Name:         input.Name,
			Title:        input.Title,
			Summary:      input.Summary,
			Severity:     string(input.Severity),
			Source:       string(input.Source),
			Signal:       input.Signal,
			ClusterLabel: input.ClusterLabel,
			OSDLabel:     alert.Labels["osd"],
			State:        string(alert.State),
			StartedAt:    alert.StartedAt,
		})
	}
	return candidates
}

func failureFromError(runID int64, err error) store.EvaluationRunFailure {
	failure := store.EvaluationRunFailure{
		RunID:        runID,
		ErrorMessage: err.Error(),
	}
	var appErr apperr.Error
	if errors.As(err, &appErr) {
		failure.ErrorClass = string(appErr.Class)
	} else {
		failure.ErrorClass = string(apperr.Internal)
	}
	return failure
}

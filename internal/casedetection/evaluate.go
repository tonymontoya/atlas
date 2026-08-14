package casedetection

import (
	"context"
	"errors"
	"time"

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

func RunFakeOnce(ctx context.Context, writer Writer, opts Options) (store.DetectionResult, error) {
	runID, err := writer.BeginAlertEvaluationRun(ctx, store.BeginEvaluationRun{
		Provider: "fake",
		Scenario: opts.Scenario,
	})
	if err != nil {
		return store.DetectionResult{}, err
	}

	result, err := runFakeOnce(ctx, writer, opts)
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

func runFakeOnce(ctx context.Context, writer Writer, opts Options) (store.DetectionResult, error) {
	provider := fake.NewObservability(fake.DefaultFixtureRoot(), opts.Scenario)

	alerts, err := provider.CurrentAlerts(ctx)
	if err != nil {
		return store.DetectionResult{}, err
	}

	evaluatedAt := opts.EvaluatedAt
	if evaluatedAt.IsZero() {
		evaluatedAt = time.Now().UTC()
	}

	return writer.DetectFromAlerts(ctx, store.AlertDetection{
		Provider:    "fake",
		Scenario:    opts.Scenario,
		EvaluatedAt: evaluatedAt,
		Candidates:  candidatesFromAlerts(alerts),
	})
}

func candidatesFromAlerts(alerts []observability.Alert) []store.AlertCandidate {
	candidates := make([]store.AlertCandidate, 0, len(alerts))
	for _, alert := range alerts {
		if alert.Name == "" || !validCaseSource(alert.Source) {
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
			State:        string(alert.State),
			StartedAt:    alert.StartedAt,
		})
	}
	return candidates
}

func validCaseSource(source string) bool {
	switch cases.CaseSource(source) {
	case cases.CaseSourceManual, cases.CaseSourcePrometheus, cases.CaseSourceCeph, cases.CaseSourceRook, cases.CaseSourceAtlas:
		return true
	}
	return false
}

func failureFromError(runID int64, err error) store.EvaluationRunFailure {
	failure := store.EvaluationRunFailure{
		RunID:        runID,
		ErrorMessage: err.Error(),
	}
	var providerErr providers.ProviderError
	if errors.As(err, &providerErr) {
		failure.ErrorClass = string(providerErr.Class)
	} else {
		failure.ErrorClass = "Internal"
	}
	return failure
}

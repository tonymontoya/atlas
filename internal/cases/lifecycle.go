package cases

import "fmt"

// CanTransition reports whether a manual status transition from one Case
// status to another is allowed. Closed is terminal: reopening a condition
// means creating a new Case, mirroring detection reopen semantics.
func CanTransition(from, to CaseStatus) error {
	if from == to {
		return fmt.Errorf("case status is already %s", from)
	}
	allowed := map[CaseStatus]map[CaseStatus]bool{
		CaseStatusDetected: {
			CaseStatusTriaged: true,
			CaseStatusClosed:  true,
		},
		CaseStatusTriaged: {
			CaseStatusClosed: true,
		},
	}
	targets, ok := allowed[from]
	if !ok || !targets[to] {
		return fmt.Errorf("transition %s -> %s is not allowed", from, to)
	}
	return nil
}

func ParseCaseStatus(value string) (CaseStatus, error) {
	switch status := CaseStatus(value); status {
	case CaseStatusDetected, CaseStatusTriaged, CaseStatusClosed:
		return status, nil
	default:
		return "", fmt.Errorf("unknown case status %q", value)
	}
}

func ParseCaseSeverity(value string) (CaseSeverity, error) {
	switch severity := CaseSeverity(value); severity {
	case CaseSeverityInfo, CaseSeverityLow, CaseSeverityMedium, CaseSeverityHigh, CaseSeverityCritical:
		return severity, nil
	default:
		return "", fmt.Errorf("unknown case severity %q", value)
	}
}

func ParseCaseSource(value string) (CaseSource, error) {
	switch source := CaseSource(value); source {
	case CaseSourceManual, CaseSourcePrometheus, CaseSourceCeph, CaseSourceRook, CaseSourceAtlas:
		return source, nil
	default:
		return "", fmt.Errorf("unknown case source %q", value)
	}
}

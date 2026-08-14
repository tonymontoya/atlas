package cases

import "testing"

func TestCanTransitionAllowsForwardEdges(t *testing.T) {
	allowed := []struct {
		from, to CaseStatus
	}{
		{CaseStatusDetected, CaseStatusTriaged},
		{CaseStatusDetected, CaseStatusClosed},
		{CaseStatusTriaged, CaseStatusClosed},
	}
	for _, edge := range allowed {
		if err := CanTransition(edge.from, edge.to); err != nil {
			t.Errorf("CanTransition(%s, %s) = %v, want nil", edge.from, edge.to, err)
		}
	}
}

func TestCanTransitionRejectsClosedAsTerminal(t *testing.T) {
	for _, to := range []CaseStatus{CaseStatusDetected, CaseStatusTriaged, CaseStatusClosed} {
		if err := CanTransition(CaseStatusClosed, to); err == nil {
			t.Errorf("CanTransition(closed, %s) accepted a transition out of a terminal status", to)
		}
	}
}

func TestCanTransitionRejectsBackwardAndSelfEdges(t *testing.T) {
	denied := []struct {
		from, to CaseStatus
	}{
		{CaseStatusTriaged, CaseStatusDetected},
		{CaseStatusDetected, CaseStatusDetected},
		{CaseStatusTriaged, CaseStatusTriaged},
	}
	for _, edge := range denied {
		if err := CanTransition(edge.from, edge.to); err == nil {
			t.Errorf("CanTransition(%s, %s) accepted a backward or self transition", edge.from, edge.to)
		}
	}
}

func TestParseCaseStatus(t *testing.T) {
	for _, status := range []CaseStatus{CaseStatusDetected, CaseStatusTriaged, CaseStatusClosed} {
		parsed, err := ParseCaseStatus(string(status))
		if err != nil || parsed != status {
			t.Errorf("ParseCaseStatus(%q) = (%q, %v), want (%q, nil)", status, parsed, err, status)
		}
	}
	if _, err := ParseCaseStatus("mitigated"); err == nil {
		t.Error("ParseCaseStatus accepted an unknown status")
	}
}

func TestParseCaseSeverity(t *testing.T) {
	for _, severity := range []CaseSeverity{
		CaseSeverityInfo, CaseSeverityLow, CaseSeverityMedium, CaseSeverityHigh, CaseSeverityCritical,
	} {
		parsed, err := ParseCaseSeverity(string(severity))
		if err != nil || parsed != severity {
			t.Errorf("ParseCaseSeverity(%q) = (%q, %v), want (%q, nil)", severity, parsed, err, severity)
		}
	}
	if _, err := ParseCaseSeverity("severe"); err == nil {
		t.Error("ParseCaseSeverity accepted an unknown severity")
	}
}

func TestParseCaseSource(t *testing.T) {
	for _, source := range []CaseSource{
		CaseSourceManual, CaseSourcePrometheus, CaseSourceCeph, CaseSourceRook, CaseSourceAtlas,
	} {
		parsed, err := ParseCaseSource(string(source))
		if err != nil || parsed != source {
			t.Errorf("ParseCaseSource(%q) = (%q, %v), want (%q, nil)", source, parsed, err, source)
		}
	}
	if _, err := ParseCaseSource("operator"); err == nil {
		t.Error("ParseCaseSource accepted an unknown source")
	}
}

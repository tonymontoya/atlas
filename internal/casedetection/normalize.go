package casedetection

import (
	"sort"
	"strings"
	"unicode"

	"github.com/tonymontoya/ceph-atlas/internal/cases"
	"github.com/tonymontoya/ceph-atlas/internal/observability"
)

type CaseInput struct {
	Name         string                    `json:"name"`
	Title        string                    `json:"title"`
	Summary      string                    `json:"summary"`
	Severity     cases.CaseSeverity        `json:"severity"`
	Source       cases.CaseSource          `json:"source"`
	Signal       string                    `json:"signal"`
	ClusterLabel string                    `json:"clusterLabel"`
	Fingerprint  observability.Fingerprint `json:"fingerprint"`
}

func Normalize(alert observability.Alert) CaseInput {
	return CaseInput{
		Name:         alert.Name,
		Title:        Title(alert),
		Summary:      Summary(alert),
		Severity:     MapSeverity(alert.Severity),
		Source:       cases.CaseSource(alert.Source),
		Signal:       Signal(alert),
		ClusterLabel: alert.Labels["cluster"],
		Fingerprint:  observability.DeriveFingerprint(alert),
	}
}

func MapSeverity(alertSeverity string) cases.CaseSeverity {
	switch strings.ToLower(strings.TrimSpace(alertSeverity)) {
	case "critical":
		return cases.CaseSeverityCritical
	case "warning":
		return cases.CaseSeverityHigh
	case "info":
		return cases.CaseSeverityLow
	default:
		return cases.CaseSeverityMedium
	}
}

func Title(alert observability.Alert) string {
	pairs := make([]string, 0, len(alert.Labels))
	for key, value := range alert.Labels {
		if key == "cluster" {
			continue
		}
		pairs = append(pairs, key+"="+value)
	}
	sort.Strings(pairs)
	if len(pairs) == 0 {
		return alert.Name
	}
	return alert.Name + " on " + strings.Join(pairs, ", ")
}

func Summary(alert observability.Alert) string {
	if summary := strings.TrimSpace(alert.Annotations["summary"]); summary != "" {
		return summary
	}
	parts := []string{alert.Source + " alert " + alert.Name + " (" + alert.Severity + ") firing"}
	if cluster := alert.Labels["cluster"]; cluster != "" {
		parts = append(parts, "on cluster "+cluster)
	}
	return strings.Join(parts, " ")
}

func Signal(alert observability.Alert) string {
	return camelToUpperSnake(alert.Name)
}

func camelToUpperSnake(s string) string {
	runes := []rune(s)
	var out []rune
	for i, r := range runes {
		if i > 0 {
			prev := runes[i-1]
			boundary := unicode.IsUpper(r) && (unicode.IsLower(prev) || unicode.IsDigit(prev))
			if !boundary && unicode.IsUpper(r) && unicode.IsUpper(prev) && i+1 < len(runes) && unicode.IsLower(runes[i+1]) {
				boundary = true
			}
			if r == '_' || r == '-' || r == ' ' {
				out = append(out, '_')
				continue
			}
			if boundary {
				out = append(out, '_')
			}
		}
		out = append(out, unicode.ToUpper(r))
	}
	return string(out)
}

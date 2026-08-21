package prometheus

import (
	"testing"

	"github.com/tonymontoya/ceph-atlas/internal/providers/contracttest"
	"github.com/tonymontoya/ceph-atlas/internal/providers/prometheus/promtest"
)

func TestObservabilityProviderContract(t *testing.T) {
	contracttest.RunObservabilityProviderSuite(t, scenarioFactory)
}

func scenarioFactory(t *testing.T, scenario contracttest.Scenario) contracttest.ObservabilityProvider {
	var mode promtest.Mode
	switch scenario {
	case contracttest.ScenarioSuccess:
		mode = promtest.ModeSuccess
	case contracttest.ScenarioUnavailable:
		mode = promtest.ModeUnavailable
	case contracttest.ScenarioUnauthorized:
		mode = promtest.ModeUnauthorized
	case contracttest.ScenarioMalformed:
		mode = promtest.ModeMalformed
	default:
		t.Fatalf("unhandled scenario %q", scenario)
	}
	server := promtest.New(t, mode)
	provider, err := New(Config{
		BaseURL:     server.URL(),
		BearerToken: promtest.Token,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	return provider
}

package fake

import (
	"testing"

	"github.com/tonymontoya/ceph-atlas/internal/providers/contracttest"
)

func TestCephReadProviderContract(t *testing.T) {
	contracttest.RunReadProviderSuite(t, scenarioFactory)
}

func TestObservabilityProviderContract(t *testing.T) {
	contracttest.RunObservabilityProviderSuite(t, observabilityScenarioFactory)
}

func observabilityScenarioFactory(t *testing.T, scenario contracttest.Scenario) contracttest.ObservabilityProvider {
	switch scenario {
	case contracttest.ScenarioSuccess:
		return NewObservability(DefaultFixtureRoot(), "osd-down-alert")
	case contracttest.ScenarioUnavailable:
		return NewObservability(DefaultFixtureRoot(), "missing-scenario")
	case contracttest.ScenarioUnauthorized:
		return NewObservability(DefaultFixtureRoot(), "provider-unauthorized")
	case contracttest.ScenarioMalformed:
		return NewObservability(DefaultFixtureRoot(), "provider-malformed")
	}
	t.Fatalf("unhandled scenario %q", scenario)
	return nil
}

func scenarioFactory(t *testing.T, scenario contracttest.Scenario) contracttest.ReadProvider {
	switch scenario {
	case contracttest.ScenarioSuccess:
		return New(DefaultFixtureRoot(), "reef-healthy-baremetal")
	case contracttest.ScenarioUnavailable:
		return New(DefaultFixtureRoot(), "missing-scenario")
	case contracttest.ScenarioUnauthorized:
		return New(DefaultFixtureRoot(), "provider-unauthorized")
	case contracttest.ScenarioMalformed:
		return New(DefaultFixtureRoot(), "provider-malformed")
	case contracttest.ScenarioPartial:
		return New(DefaultFixtureRoot(), "provider-partial")
	}
	t.Fatalf("unhandled scenario %q", scenario)
	return nil
}

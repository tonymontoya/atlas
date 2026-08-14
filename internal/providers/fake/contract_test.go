package fake

import (
	"testing"

	"github.com/tonymontoya/ceph-atlas/internal/providers"
	"github.com/tonymontoya/ceph-atlas/internal/providers/contracttest"
)

func TestCephReadProviderContract(t *testing.T) {
	contracttest.RunCephReadProviderSuite(t, scenarioFactory)
}

func scenarioFactory(t *testing.T, scenario contracttest.Scenario) providers.CephReadProvider {
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

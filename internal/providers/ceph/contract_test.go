package ceph

import (
	"testing"

	"github.com/tonymontoya/ceph-atlas/internal/providers/contracttest"
)

func TestCephReadProviderContract(t *testing.T) {
	contracttest.RunReadProviderSuite(t, scenarioFactory)
}

func scenarioFactory(t *testing.T, scenario contracttest.Scenario) contracttest.ReadProvider {
	switch scenario {
	case contracttest.ScenarioSuccess:
		return newFakeDashboard(t, modeSuccess).provider(t)
	case contracttest.ScenarioUnavailable:
		return newFakeDashboard(t, modeUnavailable).provider(t)
	case contracttest.ScenarioUnauthorized:
		return newFakeDashboard(t, modeUnauthorized).provider(t)
	case contracttest.ScenarioMalformed:
		return newFakeDashboard(t, modeMalformed).provider(t)
	case contracttest.ScenarioPartial:
		return nil
	}
	t.Fatalf("unhandled scenario %q", scenario)
	return nil
}

package ceph

import (
	"testing"

	"github.com/tonymontoya/ceph-atlas/internal/providers/ceph/dashtest"
	"github.com/tonymontoya/ceph-atlas/internal/providers/contracttest"
)

func TestCephReadProviderContract(t *testing.T) {
	contracttest.RunReadProviderSuite(t, scenarioFactory)
}

func scenarioFactory(t *testing.T, scenario contracttest.Scenario) contracttest.ReadProvider {
	var mode dashtest.Mode
	switch scenario {
	case contracttest.ScenarioSuccess:
		mode = dashtest.ModeSuccess
	case contracttest.ScenarioUnavailable:
		mode = dashtest.ModeUnavailable
	case contracttest.ScenarioUnauthorized:
		mode = dashtest.ModeUnauthorized
	case contracttest.ScenarioMalformed:
		mode = dashtest.ModeMalformed
	case contracttest.ScenarioPartial:
		return nil
	}
	provider, _ := newTestProvider(t, mode)
	return provider
}

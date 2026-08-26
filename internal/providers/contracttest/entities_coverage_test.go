package contracttest

import (
	"slices"
	"strings"
	"testing"

	"github.com/tonymontoya/ceph-atlas/internal/inventory/entities"
)

// TestSuiteRequiresCoverageForEveryDeclaredEntity proves a newly
// declared entity cannot silently skip contract testing: the suite's
// coverage build fails when the declaration holds an entity the suite
// has no exercise for. The reverse sweep keeps the exercise table from
// drifting: an exercise for an undeclared entity is dead coverage.
func TestSuiteRequiresCoverageForEveryDeclaredEntity(t *testing.T) {
	_, err := declaredEntityReads()
	if err != nil {
		t.Fatalf("declared entities should have full coverage: %v", err)
	}
	for entity := range entityReads {
		if !slices.Contains(entities.All, entity) {
			t.Errorf("contract coverage exists for %q, which is not declared in entities.All", entity.Noun)
		}
	}

	entities.All = append(entities.All, entities.Entity{Noun: "coverage-probe"})
	t.Cleanup(func() { entities.All = entities.All[:len(entities.All)-1] })

	_, err = declaredEntityReads()
	if err == nil || !strings.Contains(err.Error(), "coverage-probe") {
		t.Fatalf("error = %v, want a declared entity without contract coverage to fail the suite build", err)
	}
}

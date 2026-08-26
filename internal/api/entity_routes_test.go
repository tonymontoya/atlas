package api

import (
	"testing"

	"github.com/tonymontoya/ceph-atlas/internal/inventory/entities"
)

// TestEntityRouteGenerationFailsWhenADeclaredEntityLacksBinding proves
// a newly declared entity cannot silently skip the API surface: a
// binding table missing any declared entity panics at route-table
// construction, which the OpenAPI parity test (and process start)
// surfaces.
func TestEntityRouteGenerationFailsWhenADeclaredEntityLacksBinding(t *testing.T) {
	partial := make(map[entities.Entity]inventoryRead, len(entities.All)-1)
	for _, entity := range entities.All[1:] {
		partial[entity] = inventoryReadBindings[entity]
	}

	defer func() {
		if recover() == nil {
			t.Fatal("expected route generation to panic when a declared entity lacks a binding")
		}
	}()
	validateInventoryReadBindings(partial)
}

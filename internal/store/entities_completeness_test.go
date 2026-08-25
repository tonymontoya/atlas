package store

import (
	"context"
	"reflect"
	"testing"

	"github.com/tonymontoya/ceph-atlas/internal/inventory/entities"
)

// TestEveryDeclaredEntityHasStoreReadMethod proves the entity
// declaration and the Postgres read model cannot drift: every declared
// entity must have its named store method with the scoped-read
// signature, and every list-returning cluster read must be declared
// (or explicitly allowed, like the singleton-shaped health read).
func TestEveryDeclaredEntityHasStoreReadMethod(t *testing.T) {
	storeType := reflect.TypeOf(&PostgresStore{})
	declared := make(map[string]bool, len(entities.All))
	for _, entity := range entities.All {
		method, ok := storeType.MethodByName(entity.StoreMethod)
		if !ok {
			t.Errorf("declared entity %q names store method %s, which does not exist", entity.Noun, entity.StoreMethod)
			continue
		}
		declared[entity.StoreMethod] = true

		typ := method.Type
		if typ.NumIn() != 3 || typ.In(1) != reflect.TypeOf((*context.Context)(nil)).Elem() || typ.In(2) != reflect.TypeOf("") {
			t.Errorf("%s must take (context.Context, string), got %v", entity.StoreMethod, typ)
		}
		if typ.NumOut() != 2 || typ.Out(0).Kind() != reflect.Slice || typ.Out(1) != reflect.TypeOf((*error)(nil)).Elem() {
			t.Errorf("%s must return ([]T, error), got %v", entity.StoreMethod, typ)
		}
		if entity.View == "" || entity.Columns == "" || entity.OrderBy == "" || entity.NotFound == "" || entity.Noun == "" {
			t.Errorf("entity %q has an incomplete declaration", entity.Noun)
		}
	}

	// Health is deliberately undeclared: singleton-shaped, not a list.
	allowed := map[string]bool{"ClusterHealth": true}
	for i := range storeType.NumMethod() {
		name := storeType.Method(i).Name
		if declared[name] || allowed[name] || !isClusterReadMethod(storeType.Method(i).Type) {
			continue
		}
		t.Errorf("cluster read %s returns a list but is not declared in entities.All", name)
	}
}

// isClusterReadMethod matches the scoped-read shape: takes an FSID
// string and returns a slice.
func isClusterReadMethod(typ reflect.Type) bool {
	return typ.NumIn() == 3 && typ.In(2) == reflect.TypeOf("") &&
		typ.NumOut() == 2 && typ.Out(0).Kind() == reflect.Slice
}

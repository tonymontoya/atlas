package singlecluster

import (
	"context"
	"reflect"
	"testing"

	"github.com/tonymontoya/ceph-atlas/internal/inventory/entities"
)

// TestEveryDeclaredEntityHasReaderMethod proves the entity declaration
// and the adapter cannot drift: every declared entity must keep its
// named adapter method with the scoped-read signature (the seam app and
// api depend on), and every list-returning cluster read on the adapter
// must be declared (health stays singleton-shaped and undeclared).
func TestEveryDeclaredEntityHasReaderMethod(t *testing.T) {
	readerType := reflect.TypeOf(&Reader{})
	declared := make(map[string]bool, len(entities.All))
	for _, entity := range entities.All {
		method, ok := readerType.MethodByName(entity.StoreMethod)
		if !ok {
			t.Errorf("declared entity %q names reader method %s, which does not exist", entity.Noun, entity.StoreMethod)
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
	}

	for i := range readerType.NumMethod() {
		name := readerType.Method(i).Name
		if declared[name] || !isClusterReadMethod(readerType.Method(i).Type) {
			continue
		}
		t.Errorf("cluster read %s returns a list but is not declared in entities.All", name)
	}
}

// TestConstructionFailsWhenADeclaredEntityLacksBinding proves a newly
// declared entity cannot silently skip the adapter: a binding table
// missing any declared entity panics at construction, which every
// adapter test (and process start) surfaces.
func TestConstructionFailsWhenADeclaredEntityLacksBinding(t *testing.T) {
	partial := make(map[entities.Entity]providerFetch, len(entities.All)-1)
	for _, entity := range entities.All[1:] {
		partial[entity] = providerBindings[entity]
	}

	defer func() {
		if recover() == nil {
			t.Fatal("expected construction to panic when a declared entity lacks a binding")
		}
	}()
	bindEntities(partial)
}

// isClusterReadMethod matches the scoped-read shape: takes an FSID
// string and returns a slice.
func isClusterReadMethod(typ reflect.Type) bool {
	return typ.NumIn() == 3 && typ.In(2) == reflect.TypeOf("") &&
		typ.NumOut() == 2 && typ.Out(0).Kind() == reflect.Slice
}

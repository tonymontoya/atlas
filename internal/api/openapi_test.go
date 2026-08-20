package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/tonymontoya/ceph-atlas/internal/app"
	"github.com/tonymontoya/ceph-atlas/internal/config"
)

func TestOpenAPISpecMatchesRegisteredRoutes(t *testing.T) {
	specPath := locateOpenAPISpec(t)
	data, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("read OpenAPI spec: %v", err)
	}

	var spec struct {
		OpenAPI string `yaml:"openapi"`
		Info    struct {
			Version string `yaml:"version"`
		} `yaml:"info"`
		Paths map[string]map[string]struct {
			OperationID string `yaml:"operationId"`
		} `yaml:"paths"`
	}
	if err := yaml.Unmarshal(data, &spec); err != nil {
		t.Fatalf("parse OpenAPI spec: %v", err)
	}
	if spec.OpenAPI != "3.1.0" {
		t.Fatalf("openapi version = %q, want 3.1.0", spec.OpenAPI)
	}
	if spec.Info.Version == "" {
		t.Fatal("info.version is empty")
	}

	specRoutes := make(map[string]bool)
	for path, operations := range spec.Paths {
		for method, operation := range operations {
			switch method {
			case "get", "post", "put", "patch", "delete":
			default:
				continue
			}
			if operation.OperationID == "" {
				t.Fatalf("path %s has a %s operation without operationId", path, method)
			}
			specRoutes[strings.ToUpper(method)+" "+path] = true
		}
	}

	server := NewServer(app.New(config.Config{FakeScenario: "reef-healthy-baremetal"}))
	for _, r := range server.routes() {
		if !specRoutes[r.method+" "+r.pattern] {
			t.Fatalf("route %q is registered in the server but missing from the OpenAPI spec", r.method+" "+r.pattern)
		}
		delete(specRoutes, r.method+" "+r.pattern)
	}
	for key := range specRoutes {
		t.Fatalf("path %q is documented in the OpenAPI spec but not registered in the server", key)
	}
}

func locateOpenAPISpec(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	for {
		candidate := filepath.Join(dir, "api", "openapi", "atlas-v1.yaml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("api/openapi/atlas-v1.yaml not found from working directory")
		}
		dir = parent
	}
}

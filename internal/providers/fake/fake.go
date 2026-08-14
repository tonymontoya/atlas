package fake

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tonymontoya/ceph-atlas/internal/fleet"
	"github.com/tonymontoya/ceph-atlas/internal/inventory"
	"github.com/tonymontoya/ceph-atlas/internal/providers"
)

type Provider struct {
	fixtureRoot string
	scenario    string
}

func New(fixtureRoot, scenario string) *Provider {
	return &Provider{fixtureRoot: fixtureRoot, scenario: scenario}
}

func DefaultFixtureRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return "dev/fixtures"
	}
	for {
		candidate := filepath.Join(dir, "dev", "fixtures")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "dev/fixtures"
		}
		dir = parent
	}
}

func (p *Provider) ClusterIdentity(ctx context.Context) (fleet.ClusterIdentity, error) {
	var result fleet.ClusterIdentity
	if err := p.load(ctx, "cluster_identity.json", &result); err != nil {
		return fleet.ClusterIdentity{}, err
	}
	return result, nil
}

func (p *Provider) Health(ctx context.Context) (inventory.Health, error) {
	var result inventory.Health
	if err := p.load(ctx, "health.json", &result); err != nil {
		return inventory.Health{}, err
	}
	return result, nil
}

func (p *Provider) OSDs(ctx context.Context) ([]inventory.OSD, error) {
	var result []inventory.OSD
	if err := p.load(ctx, "osds.json", &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (p *Provider) load(ctx context.Context, name string, target any) error {
	select {
	case <-ctx.Done():
		return providers.ProviderError{Class: providers.ErrorTimeout, Message: ctx.Err().Error()}
	default:
	}

	path := filepath.Join(p.fixtureRoot, "ceph", p.scenario, name)
	data, err := os.ReadFile(path)
	if err != nil {
		return providers.ProviderError{Class: providers.ErrorUnavailable, Message: fmt.Sprintf("read fixture %s: %v", path, err)}
	}
	var envelope struct {
		Error *struct {
			Class   string `json:"class"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &envelope); err == nil && envelope.Error != nil {
		class, ok := providers.LookupErrorClass(envelope.Error.Class)
		if !ok {
			return providers.ProviderError{Class: providers.ErrorMalformedResponse, Message: fmt.Sprintf("fixture %s declares unknown error class %q", path, envelope.Error.Class)}
		}
		return providers.ProviderError{Class: class, Message: envelope.Error.Message}
	}
	if err := json.Unmarshal(data, target); err != nil {
		return providers.ProviderError{Class: providers.ErrorMalformedResponse, Message: fmt.Sprintf("parse fixture %s: %v", path, err)}
	}
	return nil
}

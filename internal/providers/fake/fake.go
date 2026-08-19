package fake

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tonymontoya/ceph-atlas/internal/apperr"
	"github.com/tonymontoya/ceph-atlas/internal/fleet"
	"github.com/tonymontoya/ceph-atlas/internal/inventory"
	"github.com/tonymontoya/ceph-atlas/internal/observability"
)

type Provider struct {
	fixtureRoot string
	scenario    string
}

func New(fixtureRoot, scenario string) *Provider {
	return &Provider{fixtureRoot: fixtureRoot, scenario: scenario}
}

type Observability struct {
	fixtureRoot string
	scenario    string
}

func NewObservability(fixtureRoot, scenario string) *Observability {
	return &Observability{fixtureRoot: fixtureRoot, scenario: scenario}
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

func (p *Provider) Hosts(ctx context.Context) ([]inventory.Host, error) {
	var result []inventory.Host
	if err := p.load(ctx, "hosts.json", &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (p *Provider) HostDevices(ctx context.Context, host string) ([]inventory.StorageDevice, error) {
	hosts, err := p.Hosts(ctx)
	if err != nil {
		return nil, err
	}
	known := false
	for _, candidate := range hosts {
		if candidate.Name == host {
			known = true
			break
		}
	}
	if !known {
		return nil, apperr.Error{Class: apperr.NotFound, Message: fmt.Sprintf("host %q not found in scenario %q", host, p.scenario)}
	}
	var all []inventory.StorageDevice
	if err := p.load(ctx, "devices.json", &all); err != nil {
		return nil, err
	}
	devices := make([]inventory.StorageDevice, 0, len(all))
	for _, device := range all {
		if device.Host == host {
			devices = append(devices, device)
		}
	}
	return devices, nil
}

func (p *Provider) Daemons(ctx context.Context) ([]inventory.Daemon, error) {
	var result []inventory.Daemon
	if err := p.load(ctx, "daemons.json", &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (p *Provider) Pools(ctx context.Context) ([]inventory.Pool, error) {
	var result []inventory.Pool
	if err := p.load(ctx, "pools.json", &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (p *Observability) CurrentAlerts(ctx context.Context) ([]observability.Alert, error) {
	var result []observability.Alert
	if err := p.load(ctx, "alerts.json", &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (p *Observability) load(ctx context.Context, name string, target any) error {
	return loadFixture(ctx, p.fixtureRoot, "prometheus", p.scenario, name, target)
}

func (p *Provider) load(ctx context.Context, name string, target any) error {
	return loadFixture(ctx, p.fixtureRoot, "ceph", p.scenario, name, target)
}

func loadFixture(ctx context.Context, fixtureRoot, family, scenario, name string, target any) error {
	select {
	case <-ctx.Done():
		return apperr.Error{Class: apperr.Timeout, Message: ctx.Err().Error()}
	default:
	}

	path := filepath.Join(fixtureRoot, family, scenario, name)
	data, err := os.ReadFile(path)
	if err != nil {
		return apperr.Error{Class: apperr.Unavailable, Message: fmt.Sprintf("read fixture %s: %v", path, err)}
	}
	var envelope struct {
		Error *struct {
			Class   string `json:"class"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &envelope); err == nil && envelope.Error != nil {
		class, ok := apperr.LookupClass(envelope.Error.Class)
		if !ok {
			return apperr.Error{Class: apperr.MalformedResponse, Message: fmt.Sprintf("fixture %s declares unknown error class %q", path, envelope.Error.Class)}
		}
		return apperr.Error{Class: class, Message: envelope.Error.Message}
	}
	if err := json.Unmarshal(data, target); err != nil {
		return apperr.Error{Class: apperr.MalformedResponse, Message: fmt.Sprintf("parse fixture %s: %v", path, err)}
	}
	return nil
}

package providers

import (
	"context"

	"github.com/tonymontoya/ceph-atlas/internal/fleet"
	"github.com/tonymontoya/ceph-atlas/internal/inventory"
	"github.com/tonymontoya/ceph-atlas/internal/observability"
)

type CephReadProvider interface {
	ClusterIdentity(ctx context.Context) (fleet.ClusterIdentity, error)
	Health(ctx context.Context) (inventory.Health, error)
	OSDs(ctx context.Context) ([]inventory.OSD, error)
	Hosts(ctx context.Context) ([]inventory.Host, error)
	HostDevices(ctx context.Context, host string) ([]inventory.StorageDevice, error)
	Daemons(ctx context.Context) ([]inventory.Daemon, error)
	Pools(ctx context.Context) ([]inventory.Pool, error)
}

type ObservabilityProvider interface {
	CurrentAlerts(ctx context.Context) ([]observability.Alert, error)
}

func AllHostDevices(ctx context.Context, provider CephReadProvider, hosts []inventory.Host) ([]inventory.StorageDevice, error) {
	devices := make([]inventory.StorageDevice, 0, len(hosts))
	for _, host := range hosts {
		hostDevices, err := provider.HostDevices(ctx, host.Name)
		if err != nil {
			return nil, err
		}
		devices = append(devices, hostDevices...)
	}
	return devices, nil
}

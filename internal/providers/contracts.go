package providers

import (
	"context"

	"github.com/tonymontoya/ceph-atlas/internal/fleet"
	"github.com/tonymontoya/ceph-atlas/internal/inventory"
)

type ErrorClass string

const (
	ErrorUnavailable        ErrorClass = "Unavailable"
	ErrorUnauthorized       ErrorClass = "Unauthorized"
	ErrorUnsupported        ErrorClass = "Unsupported"
	ErrorVersionUnsupported ErrorClass = "VersionUnsupported"
	ErrorNotFound           ErrorClass = "NotFound"
	ErrorConflict           ErrorClass = "Conflict"
	ErrorUnsafe             ErrorClass = "Unsafe"
	ErrorPartial            ErrorClass = "Partial"
	ErrorMalformedResponse  ErrorClass = "MalformedResponse"
	ErrorTimeout            ErrorClass = "Timeout"
)

type ProviderError struct {
	Class   ErrorClass
	Message string
}

func (e ProviderError) Error() string {
	if e.Message == "" {
		return string(e.Class)
	}
	return string(e.Class) + ": " + e.Message
}

func LookupErrorClass(name string) (ErrorClass, bool) {
	class := ErrorClass(name)
	switch class {
	case ErrorUnavailable,
		ErrorUnauthorized,
		ErrorUnsupported,
		ErrorVersionUnsupported,
		ErrorNotFound,
		ErrorConflict,
		ErrorUnsafe,
		ErrorPartial,
		ErrorMalformedResponse,
		ErrorTimeout:
		return class, true
	}
	return "", false
}

type CephReadProvider interface {
	ClusterIdentity(ctx context.Context) (fleet.ClusterIdentity, error)
	Health(ctx context.Context) (inventory.Health, error)
	OSDs(ctx context.Context) ([]inventory.OSD, error)
	Hosts(ctx context.Context) ([]inventory.Host, error)
	HostDevices(ctx context.Context, host string) ([]inventory.StorageDevice, error)
	Daemons(ctx context.Context) ([]inventory.Daemon, error)
	Pools(ctx context.Context) ([]inventory.Pool, error)
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

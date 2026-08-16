package ceph

import (
	"context"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/tonymontoya/ceph-atlas/internal/fleet"
	"github.com/tonymontoya/ceph-atlas/internal/inventory"
	"github.com/tonymontoya/ceph-atlas/internal/providers"
)

// GET /api/health/get_cluster_fsid answers with a bare JSON string (Reef
// health controller returns mgr config fsid directly); GET /api/summary
// answers with an object whose version field is the mgr version. The
// Dashboard exposes no cluster name, so the name comes from configuration.
func (p *Provider) ClusterIdentity(ctx context.Context) (fleet.ClusterIdentity, error) {
	var fsid string
	if err := p.getJSON(ctx, "/api/health/get_cluster_fsid", nil, &fsid); err != nil {
		return fleet.ClusterIdentity{}, err
	}
	var summary struct {
		Version string `json:"version"`
	}
	if err := p.getJSON(ctx, "/api/summary", nil, &summary); err != nil {
		return fleet.ClusterIdentity{}, err
	}
	return fleet.ClusterIdentity{
		FSID:        fsid,
		Name:        p.clusterName,
		CephVersion: summary.Version,
		Type:        fleet.ClusterTypeBareMetal,
	}, nil
}

// GET /api/health/full nests ceph health under health; the Dashboard
// transforms the checks map into a list, stamping each entry's check name
// into type, and keeps the raw ceph summary list.
func (p *Provider) Health(ctx context.Context) (inventory.Health, error) {
	var full struct {
		Health struct {
			Status  string `json:"status"`
			Summary []struct {
				Severity string `json:"severity"`
				Summary  string `json:"summary"`
			} `json:"summary"`
			Checks []struct {
				Type     string `json:"type"`
				Severity string `json:"severity"`
				Summary  string `json:"summary"`
			} `json:"checks"`
		} `json:"health"`
	}
	if err := p.getJSON(ctx, "/api/health/full", nil, &full); err != nil {
		return inventory.Health{}, err
	}
	health := full.Health
	switch health.Status {
	case string(inventory.HealthOK), string(inventory.HealthWarn), string(inventory.HealthErr):
	default:
		return inventory.Health{}, providerErr(providers.ErrorMalformedResponse, "unknown health status %q", health.Status)
	}
	checks := make([]inventory.HealthCheck, 0, len(health.Checks))
	for _, check := range health.Checks {
		checks = append(checks, inventory.HealthCheck{
			Name:     check.Type,
			Severity: check.Severity,
			Summary:  check.Summary,
		})
	}
	summaryTexts := make([]string, 0, len(health.Summary))
	for _, entry := range health.Summary {
		if entry.Summary != "" {
			summaryTexts = append(summaryTexts, entry.Summary)
		}
	}
	summary := ""
	if len(summaryTexts) > 0 {
		summary = strings.Join(summaryTexts, "; ")
	}
	return inventory.Health{
		Status:  inventory.HealthStatus(health.Status),
		Summary: summary,
		Checks:  checks,
	}, nil
}

// GET /api/osd items carry up and in as 0/1 integers from the OSD map and
// host as the OSD's CRUSH host node; the Dashboard list endpoint attaches
// no device identity, so Device stays empty here.
func (p *Provider) OSDs(ctx context.Context) ([]inventory.OSD, error) {
	items, err := getPaged[struct {
		ID   int `json:"id"`
		Up   int `json:"up"`
		In   int `json:"in"`
		Host struct {
			Name string `json:"name"`
		} `json:"host"`
	}](ctx, p, "/api/osd", nil)
	if err != nil {
		return nil, err
	}
	osds := make([]inventory.OSD, 0, len(items))
	for _, item := range items {
		osds = append(osds, inventory.OSD{
			ID:   item.ID,
			Host: item.Host.Name,
			Up:   item.Up != 0,
			In:   item.In != 0,
		})
	}
	return osds, nil
}

// GET /api/host is paginated (Reef defaults to five per page); items carry
// hostname and an addr that is empty when the orchestrator has none.
func (p *Provider) Hosts(ctx context.Context) ([]inventory.Host, error) {
	items, err := getPaged[struct {
		Hostname string `json:"hostname"`
		Addr     string `json:"addr"`
	}](ctx, p, "/api/host", nil)
	if err != nil {
		return nil, err
	}
	hosts := make([]inventory.Host, 0, len(items))
	for _, item := range items {
		hosts = append(hosts, inventory.Host{
			Name:    item.Hostname,
			Address: item.Addr,
		})
	}
	return hosts, nil
}

// HostDevices probes GET /api/host/{hostname} first so an unknown host
// surfaces as the contract's NotFound (the inventory endpoint answers 200
// with an empty object for unknown hosts instead of 404). GET
// /api/host/{hostname}/inventory requires an orchestrator; device_id is
// the udev ID, osd_ids backs devices provisioned directly while lvm-backed
// OSDs surface through the lvs entries.
func (p *Provider) HostDevices(ctx context.Context, host string) ([]inventory.StorageDevice, error) {
	err := p.getJSON(ctx, hostEndpoint(host), nil, &struct{}{})
	if err != nil {
		return nil, err
	}
	var inventoryResponse struct {
		Devices []struct {
			Path              string `json:"path"`
			DeviceID          string `json:"device_id"`
			HumanReadableType string `json:"human_readable_type"`
			OSDIds            []int  `json:"osd_ids"`
			LVs               []struct {
				OSDID string `json:"osd_id"`
			} `json:"lvs"`
		} `json:"devices"`
	}
	if err := p.getJSON(ctx, hostEndpoint(host)+"/inventory", nil, &inventoryResponse); err != nil {
		return nil, err
	}
	devices := make([]inventory.StorageDevice, 0, len(inventoryResponse.Devices))
	for _, device := range inventoryResponse.Devices {
		var osdID *int
		if len(device.OSDIds) > 0 {
			id := device.OSDIds[0]
			osdID = &id
		} else if len(device.LVs) > 0 {
			if id, err := strconv.Atoi(device.LVs[0].OSDID); err == nil {
				osdID = &id
			}
		}
		devices = append(devices, inventory.StorageDevice{
			Host:   host,
			Serial: device.DeviceID,
			Type:   device.HumanReadableType,
			Path:   device.Path,
			OSDID:  osdID,
		})
	}
	return devices, nil
}

// GET /api/daemon items are orchestrator DaemonDescription dicts: status
// is the int enum -2 unknown, -1 error, 0 stopped, 1 running, 2 starting.
func (p *Provider) Daemons(ctx context.Context) ([]inventory.Daemon, error) {
	items, err := getPaged[struct {
		DaemonType string `json:"daemon_type"`
		DaemonID   string `json:"daemon_id"`
		DaemonName string `json:"daemon_name"`
		Hostname   string `json:"hostname"`
		Status     int    `json:"status"`
		Version    string `json:"version"`
	}](ctx, p, "/api/daemon", nil)
	if err != nil {
		return nil, err
	}
	daemons := make([]inventory.Daemon, 0, len(items))
	for _, item := range items {
		name := item.DaemonName
		if name == "" {
			name = item.DaemonType + "." + item.DaemonID
		}
		daemons = append(daemons, inventory.Daemon{
			Type:    item.DaemonType,
			Name:    name,
			Host:    item.Hostname,
			Status:  daemonStatus(item.Status),
			Version: item.Version,
		})
	}
	return daemons, nil
}

// GET /api/pool items name pools pool/pool_name with string types
// (replicated or erasure) and integer size/min_size.
func (p *Provider) Pools(ctx context.Context) ([]inventory.Pool, error) {
	items, err := getPaged[struct {
		Pool     int    `json:"pool"`
		PoolName string `json:"pool_name"`
		Type     string `json:"type"`
		Size     *int   `json:"size"`
		MinSize  *int   `json:"min_size"`
	}](ctx, p, "/api/pool", nil)
	if err != nil {
		return nil, err
	}
	pools := make([]inventory.Pool, 0, len(items))
	for _, item := range items {
		pools = append(pools, inventory.Pool{
			ID:      item.Pool,
			Name:    item.PoolName,
			Type:    item.Type,
			Size:    item.Size,
			MinSize: item.MinSize,
		})
	}
	return pools, nil
}

func hostEndpoint(host string) string {
	return path.Join("/api/host", url.PathEscape(host))
}

func daemonStatus(status int) string {
	switch status {
	case 1:
		return "running"
	case 0:
		return "stopped"
	case -1:
		return "error"
	case 2:
		return "starting"
	default:
		return "unknown"
	}
}

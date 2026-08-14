package inventory

type HealthStatus string

const (
	HealthOK   HealthStatus = "HEALTH_OK"
	HealthWarn HealthStatus = "HEALTH_WARN"
	HealthErr  HealthStatus = "HEALTH_ERR"
)

type Health struct {
	Status  HealthStatus  `json:"status"`
	Summary string        `json:"summary"`
	Checks  []HealthCheck `json:"checks"`
}

type HealthCheck struct {
	Name     string `json:"name"`
	Severity string `json:"severity"`
	Summary  string `json:"summary"`
}

type OSD struct {
	ID     int    `json:"id"`
	Host   string `json:"host"`
	Up     bool   `json:"up"`
	In     bool   `json:"in"`
	Device string `json:"device,omitempty"`
}

type Host struct {
	Name    string `json:"name"`
	Address string `json:"address,omitempty"`
}

type StorageDevice struct {
	Host   string `json:"host"`
	Serial string `json:"serial"`
	Type   string `json:"type,omitempty"`
	Path   string `json:"path,omitempty"`
	Health string `json:"health,omitempty"`
	OSDID  *int   `json:"osdId,omitempty"`
}

type Daemon struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Host    string `json:"host"`
	Status  string `json:"status"`
	Version string `json:"version,omitempty"`
}

type Pool struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Size    *int   `json:"size,omitempty"`
	MinSize *int   `json:"minSize,omitempty"`
}

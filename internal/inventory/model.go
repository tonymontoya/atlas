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

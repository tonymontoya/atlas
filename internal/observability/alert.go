package observability

import "time"

type AlertState string

const (
	AlertStateFiring   AlertState = "firing"
	AlertStatePending  AlertState = "pending"
	AlertStateResolved AlertState = "resolved"
)

type Alert struct {
	Name        string            `json:"name"`
	Severity    string            `json:"severity"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	StartedAt   time.Time         `json:"startedAt"`
	State       AlertState        `json:"state"`
	Source      string            `json:"source"`
}

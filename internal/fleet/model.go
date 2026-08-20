package fleet

import "time"

type ClusterType string

const (
	ClusterTypeBareMetal ClusterType = "bare-metal"
	ClusterTypeRook      ClusterType = "rook"
)

type ClusterIdentity struct {
	FSID        string      `json:"fsid"`
	Name        string      `json:"name"`
	CephVersion string      `json:"cephVersion"`
	Type        ClusterType `json:"type"`
}

// ClusterRegistration is the Operator-created record of a Cluster (ADR-0025).
// FSID is nil until Enrollment binds it; CephVersion is nil until the first
// observation. Deregistration keeps the row and its history.
type ClusterRegistration struct {
	ID             int64       `json:"id"`
	FSID           *string     `json:"fsid"`
	Name           string      `json:"name"`
	CephVersion    *string     `json:"cephVersion"`
	Type           ClusterType `json:"clusterType"`
	RegisteredAt   *time.Time  `json:"registeredAt"`
	RegisteredBy   string      `json:"registeredBy"`
	DeregisteredAt *time.Time  `json:"deregisteredAt"`
}

// EnrollmentCredential is the one-time credential shown exactly once in the
// registration response; Atlas persists only its hash (ADR-0026).
type EnrollmentCredential struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
}

package fleet

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

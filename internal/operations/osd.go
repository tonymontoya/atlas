package operations

import "errors"

// DestroyOSD destroys an OSD on the target cluster. Mutating: it executes
// only under the approval context the request envelope carries.
type DestroyOSD struct {
	ClusterFSID string `json:"clusterFsid"`
	OSDID       int64  `json:"osdId"`
}

func (DestroyOSD) OperationType() string {
	return "DestroyOSD"
}

func (op DestroyOSD) Validate() error {
	return validateOSDTarget("DestroyOSD", op.ClusterFSID, op.OSDID)
}

// VerifyOSD verifies an OSD is healthy after replacement work.
type VerifyOSD struct {
	ClusterFSID string `json:"clusterFsid"`
	OSDID       int64  `json:"osdId"`
}

func (VerifyOSD) OperationType() string {
	return "VerifyOSD"
}

func (op VerifyOSD) Validate() error {
	return validateOSDTarget("VerifyOSD", op.ClusterFSID, op.OSDID)
}

func validateOSDTarget(operationType, clusterFSID string, osdID int64) error {
	if clusterFSID == "" {
		return errors.New(operationType + ": clusterFsid is required")
	}
	if osdID < 0 {
		return errors.New(operationType + ": osdId must not be negative")
	}
	return nil
}

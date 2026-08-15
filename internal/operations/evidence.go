package operations

import "errors"

// CollectHostEvidence collects host-local evidence through an
// authenticated Atlas Agent. Read-only, but privileged.
type CollectHostEvidence struct {
	Host        string `json:"host"`
	RequestType string `json:"requestType"`
}

func (CollectHostEvidence) OperationType() string {
	return "CollectHostEvidence"
}

func (op CollectHostEvidence) Validate() error {
	if op.Host == "" {
		return errors.New("host is required")
	}
	if op.RequestType == "" {
		return errors.New("requestType is required")
	}
	return nil
}

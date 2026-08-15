package workflows

import (
	"github.com/tonymontoya/ceph-atlas/internal/operations"
)

// ReplaceOSD returns the Replace OSD Workflow definition (ADR-0017): the
// MVP tracer-bullet procedure for replacing a failed Storage Device behind
// an OSD. Ordered steps: collect host evidence, pause at the single
// Approval Gate before any mutation (ADR-0020), destroy the OSD, pause for
// the human hardware-replacement Task (ADR-0019), then verify the OSD.
//
// Read-only Jobs retry up to three attempts; DestroyOSD does not
// auto-retry — a failed destruction is surfaced for the Operator instead
// of re-executed (ADR-0019 idempotency posture).
func ReplaceOSD() Definition {
	return Definition{
		ID:      "replace-osd",
		Version: 1,
		Steps: []Step{
			JobStep{
				ID:            "collect-evidence",
				OperationType: operations.CollectHostEvidence{}.OperationType(),
				Retry:         RetryPolicy{MaxAttempts: 3},
			},
			ApprovalGate{ID: "approve-destroy"},
			JobStep{
				ID:            "destroy-osd",
				OperationType: operations.DestroyOSD{}.OperationType(),
				Retry:         RetryPolicy{MaxAttempts: 1},
			},
			TaskStep{
				ID:      "replace-device",
				Summary: "Replace the failed Storage Device",
			},
			JobStep{
				ID:            "verify-osd",
				OperationType: operations.VerifyOSD{}.OperationType(),
				Retry:         RetryPolicy{MaxAttempts: 3},
			},
		},
	}
}

// DefaultRegistry returns a code registry with the Workflow definitions
// this Atlas build ships. Adding a Workflow means adding a Definition and
// registering it here (ADR-0017).
func DefaultRegistry(ops *operations.Registry) (*CodeRegistry, error) {
	return NewCodeRegistry(ops, ReplaceOSD())
}

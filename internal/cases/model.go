package cases

import "time"

type CaseStatus string

const (
	CaseStatusDetected CaseStatus = "detected"
	CaseStatusTriaged  CaseStatus = "triaged"
	CaseStatusClosed   CaseStatus = "closed"
)

type CaseSeverity string

const (
	CaseSeverityInfo     CaseSeverity = "info"
	CaseSeverityLow      CaseSeverity = "low"
	CaseSeverityMedium   CaseSeverity = "medium"
	CaseSeverityHigh     CaseSeverity = "high"
	CaseSeverityCritical CaseSeverity = "critical"
)

type CaseSource string

const (
	CaseSourceManual     CaseSource = "manual"
	CaseSourcePrometheus CaseSource = "prometheus"
	CaseSourceCeph       CaseSource = "ceph"
	CaseSourceRook       CaseSource = "rook"
	CaseSourceAtlas      CaseSource = "atlas"
)

type Case struct {
	ID                  int64          `json:"id"`
	Title               string         `json:"title"`
	Summary             string         `json:"summary"`
	Status              CaseStatus     `json:"status"`
	Severity            CaseSeverity   `json:"severity"`
	Source              CaseSource     `json:"source"`
	ClusterFSID         string         `json:"clusterFsid,omitempty"`
	Assignee            string         `json:"assignee,omitempty"`
	AssigneeDisplayName string         `json:"assigneeDisplayName,omitempty"`
	CreatedAt           time.Time      `json:"createdAt"`
	UpdatedAt           time.Time      `json:"updatedAt"`
	ClosedAt            *time.Time     `json:"closedAt,omitempty"`
	DetectedBy          *DetectionLink `json:"detectedBy,omitempty"`
}

type DetectionLink struct {
	Source      string    `json:"source"`
	AlertName   string    `json:"alertName"`
	Signal      string    `json:"signal,omitempty"`
	FirstSeenAt time.Time `json:"firstSeenAt"`
	LastSeenAt  time.Time `json:"lastSeenAt"`
}

type TimelineEventType string

const (
	TimelineEventCaseDetected         TimelineEventType = "case_detected"
	TimelineEventCaseTriaged          TimelineEventType = "case_triaged"
	TimelineEventCaseStatusChanged    TimelineEventType = "case_status_changed"
	TimelineEventCaseNoteAdded        TimelineEventType = "case_note_added"
	TimelineEventCaseAssigned         TimelineEventType = "case_assigned"
	TimelineEventWorkflowAttached     TimelineEventType = "workflow_attached"
	TimelineEventWorkflowStateChanged TimelineEventType = "workflow_state_changed"
)

type TimelineActorType string

const (
	TimelineActorSystem     TimelineActorType = "system"
	TimelineActorUser       TimelineActorType = "user"
	TimelineActorAtlasAgent TimelineActorType = "atlas_agent"
	TimelineActorProvider   TimelineActorType = "provider"
)

type TimelineActor struct {
	Type        TimelineActorType `json:"type"`
	ID          string            `json:"id,omitempty"`
	DisplayName string            `json:"displayName"`
}

type TimelineEvent struct {
	ID         int64             `json:"id"`
	CaseID     int64             `json:"caseId"`
	Type       TimelineEventType `json:"type"`
	Message    string            `json:"message"`
	OccurredAt time.Time         `json:"occurredAt"`
	Actor      TimelineActor     `json:"actor"`
	Payload    map[string]any    `json:"payload"`
}

type CaseNote struct {
	ID                int64     `json:"id"`
	CaseID            int64     `json:"caseId"`
	AuthorID          string    `json:"authorId"`
	AuthorDisplayName string    `json:"authorDisplayName"`
	Body              string    `json:"body"`
	CreatedAt         time.Time `json:"createdAt"`
}

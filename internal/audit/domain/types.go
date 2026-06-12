package domain

import "time"

type Status string

const (
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityHigh     Severity = "high"
	SeverityMedium   Severity = "medium"
	SeverityLow      Severity = "low"
)

type Finding struct {
	Severity   Severity `json:"severity"`
	Category   string   `json:"category"`
	Resource   string   `json:"resource"`
	Risk       string   `json:"risk"`
	Impact     string   `json:"impact"`
	Suggestion string   `json:"suggestion"`
}

type Summary struct {
	Total    int `json:"total"`
	Critical int `json:"critical"`
	High     int `json:"high"`
	Medium   int `json:"medium"`
	Low      int `json:"low"`
}

type Result struct {
	Findings   []Finding `json:"findings"`
	Summary    Summary   `json:"summary"`
	Markdown   string    `json:"markdown,omitempty"`
	DurationMs int64     `json:"duration_ms,omitempty"`
}

type Audit struct {
	ID                 string    `json:"id"`
	ClusterFingerprint string    `json:"cluster_fingerprint"`
	ClusterDisplay     string    `json:"cluster_display"`
	Status             Status    `json:"status"`
	Result             *Result   `json:"result,omitempty"`
	ErrorMessage       string    `json:"error_message,omitempty"`
	DurationMs         int64     `json:"duration_ms,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
}

type EventRecord struct {
	ID          int64  `json:"-"`
	AuditID     string `json:"audit_id"`
	SeqNum      int    `json:"seq_num"`
	EventType   string `json:"event_type"`
	Message     string `json:"message,omitempty"`
	Summary     string `json:"summary,omitempty"`
	Detail      string `json:"detail,omitempty"`
	PayloadKind string `json:"payload_kind,omitempty"`
	PayloadJSON string `json:"payload_json,omitempty"`
	ElapsedMs   int    `json:"elapsed_ms,omitempty"`
	CreatedAt   int64  `json:"created_at"`
}

type EventAppend struct {
	EventType   string
	Message     string
	Summary     string
	Detail      string
	PayloadKind string
	PayloadJSON string
	ElapsedMs   int
}

type Target struct {
	Cluster        string `json:"cluster"`
	ClusterDisplay string `json:"cluster_display"`
}

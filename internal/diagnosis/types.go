package diagnosis

import "time"

type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
)

type FixAction struct {
	Type        string `json:"type"`        // "command" | "guidance"
	Description string `json:"description"`
	Command     string `json:"command,omitempty"`
	Risk        string `json:"risk,omitempty"`
}

type Evidence struct {
	Num  int    `json:"num"`
	Text string `json:"text"`
}

type StreamEvent struct {
	Type    string `json:"type"`
	Message string `json:"message,omitempty"`
	Detail  string `json:"detail,omitempty"`
}

type Diagnosis struct {
	ID                 string       `json:"id"`
	ClusterFingerprint string       `json:"cluster_fingerprint"`
	ClusterDisplay     string       `json:"cluster_display"`
	Disconnected       bool         `json:"disconnected"`
	Namespace          string       `json:"namespace"`
	Pod                string       `json:"pod"`
	PodUID             string       `json:"pod_uid,omitempty"`
	SymptomHash        string       `json:"symptom_hash,omitempty"`

	Status     Status      `json:"status"`
	RootCause  string      `json:"root_cause,omitempty"`
	Confidence string      `json:"confidence,omitempty"`
	Evidence   []Evidence  `json:"evidence,omitempty"`
	FixActions []FixAction `json:"fix_actions,omitempty"`
	Impact     string      `json:"impact,omitempty"`
	DurationMs int64       `json:"duration_ms,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	Resolved  int       `json:"resolved"`
}

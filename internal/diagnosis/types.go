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
	Type        string `json:"type"` // "command" | "guidance"
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
	ID                 string `json:"id"`
	ClusterFingerprint string `json:"cluster_fingerprint"`
	ClusterDisplay     string `json:"cluster_display"`
	Disconnected       bool   `json:"disconnected"`
	Namespace          string `json:"namespace"`
	Pod                string `json:"pod"`
	PodUID             string `json:"pod_uid,omitempty"`
	SymptomHash        string `json:"symptom_hash,omitempty"`

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

// EventRecord is a single diagnosis event stored in diagnosis_events table.
type EventRecord struct {
	ID          int64  `json:"-"`
	DiagnosisID string `json:"diagnosis_id"`
	SeqNum      int    `json:"seq_num"`
	EventType   string `json:"event_type"`
	Message     string `json:"message,omitempty"`
	Detail      string `json:"detail,omitempty"`
	TokenIn     int    `json:"token_in,omitempty"`
	TokenOut    int    `json:"token_out,omitempty"`
	ElapsedMs   int    `json:"elapsed_ms,omitempty"`
	CreatedAt   int64  `json:"created_at"`
}

// DiagnosisTarget identifies the resource being diagnosed.
type DiagnosisTarget struct {
	Cluster        string `json:"cluster"`
	ClusterDisplay string `json:"cluster_display"`
	Namespace      string `json:"namespace"`
	Pod            string `json:"pod"`
}

// DiagnosisResult is the final output of a completed diagnosis.
type DiagnosisResult struct {
	RootCause  string      `json:"root_cause"`
	Confidence string      `json:"confidence,omitempty"`
	Evidence   []Evidence  `json:"evidence,omitempty"`
	FixActions []FixAction `json:"fix_actions,omitempty"`
	Impact     string      `json:"impact,omitempty"`
	DurationMs int64       `json:"duration_ms,omitempty"`
}

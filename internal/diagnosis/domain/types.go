package domain

import "time"

type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

type Verdict string

const (
	VerdictConfirmed    Verdict = "confirmed"
	VerdictLikely       Verdict = "likely"
	VerdictInconclusive Verdict = "inconclusive"
)

type FixAction struct {
	Priority    string `json:"priority,omitempty"`
	Type        string `json:"type,omitempty"`
	Description string `json:"description"`
	Command     string `json:"command,omitempty"`
	Risk        string `json:"risk,omitempty"`
}

type Evidence struct {
	ID         string `json:"id"`
	Source     string `json:"source,omitempty"`
	Signal     string `json:"signal,omitempty"`
	Strength   string `json:"strength,omitempty"`
	Summary    string `json:"summary"`
	Detail     string `json:"detail,omitempty"`
	RawExcerpt string `json:"raw_excerpt,omitempty"`
	// Legacy numbered evidence for older clients.
	Num  int    `json:"num,omitempty"`
	Text string `json:"text,omitempty"`
}

type Hypothesis struct {
	ID                 string   `json:"id"`
	Category           string   `json:"category"`
	Title              string   `json:"title"`
	Status             string   `json:"status"`
	ConfidenceDelta    float64  `json:"confidence_delta,omitempty"`
	SupportingEvidence []string `json:"supporting_evidence,omitempty"`
	RefutingEvidence   []string `json:"refuting_evidence,omitempty"`
	Rationale          string   `json:"rationale,omitempty"`
}

type RootCause struct {
	Category        string  `json:"category"`
	Title           string  `json:"title"`
	ConfidenceScore float64 `json:"confidence_score,omitempty"`
	ConfidenceLabel string  `json:"confidence_label,omitempty"`
	Summary         string  `json:"summary"`
}

type Impact struct {
	Severity    string `json:"severity"`
	Description string `json:"description"`
}

type Diagnosis struct {
	ID                 string  `json:"id"`
	ClusterFingerprint string  `json:"cluster_fingerprint"`
	ClusterDisplay     string  `json:"cluster_display"`
	Namespace          string  `json:"namespace"`
	Pod                string  `json:"pod"`
	Status             Status  `json:"status"`
	Report             *Result `json:"report,omitempty"`
	// Legacy flat fields populated from report for list views.
	RootCause  string      `json:"root_cause,omitempty"`
	Confidence string      `json:"confidence,omitempty"`
	Evidence   []Evidence  `json:"evidence,omitempty"`
	FixActions []FixAction `json:"fix_actions,omitempty"`
	Impact     string      `json:"impact,omitempty"`
	DurationMs int64       `json:"duration_ms,omitempty"`
	CreatedAt  time.Time   `json:"created_at"`
}

type EventRecord struct {
	ID          int64  `json:"-"`
	DiagnosisID string `json:"diagnosis_id"`
	SeqNum      int    `json:"seq_num"`
	EventType   string `json:"event_type"`
	Message     string `json:"message,omitempty"`
	Summary     string `json:"summary,omitempty"`
	Detail      string `json:"detail,omitempty"`
	PayloadKind string `json:"payload_kind,omitempty"`
	PayloadJSON string `json:"payload_json,omitempty"`
	TokenIn     int    `json:"token_in,omitempty"`
	TokenOut    int    `json:"token_out,omitempty"`
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
	TokenIn     int
	TokenOut    int
	ElapsedMs   int
}

type Target struct {
	Cluster        string `json:"cluster"`
	ClusterDisplay string `json:"cluster_display"`
	Namespace      string `json:"namespace"`
	Pod            string `json:"pod"`
}

type EnrichmentInfo struct {
	Status        string   `json:"status"`
	DegradedSteps []string `json:"degraded_steps,omitempty"`
	Message       string   `json:"message,omitempty"`
}

type Result struct {
	Verdict     Verdict        `json:"verdict"`
	RootCause   RootCause      `json:"root_cause"`
	Evidence    []Evidence     `json:"evidence,omitempty"`
	Hypotheses  []Hypothesis   `json:"hypotheses,omitempty"`
	Actions     []FixAction    `json:"actions,omitempty"`
	Impact      Impact         `json:"impact,omitempty"`
	Limitations []string       `json:"limitations,omitempty"`
	Enrichment  EnrichmentInfo `json:"enrichment"`
	Markdown    string         `json:"markdown,omitempty"`
	DurationMs  int64          `json:"duration_ms,omitempty"`
}

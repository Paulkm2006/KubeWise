package report

import "time"

type DiagnosisReport struct {
	Target      Target         `json:"target"`
	GeneratedAt time.Time      `json:"generated_at"`
	Summary     string         `json:"summary"`
	Verdict     Verdict        `json:"verdict"`
	RootCause   RootCause      `json:"root_cause"`
	Evidence    []Evidence     `json:"evidence"`
	Hypotheses  []Hypothesis   `json:"hypotheses"`
	Actions     []Action       `json:"actions"`
	Impact      Impact         `json:"impact"`
	Limitations []string       `json:"limitations,omitempty"`
	Enrichment  EnrichmentInfo `json:"enrichment"`
}

type Target struct {
	Cluster   string `json:"cluster"`
	Namespace string `json:"namespace"`
	Pod       string `json:"pod"`
}

type Evidence struct {
	ID         string            `json:"id"`
	Source     string            `json:"source"`
	Signal     string            `json:"signal,omitempty"`
	Strength   string            `json:"strength"`
	Summary    string            `json:"summary"`
	Detail     string            `json:"detail,omitempty"`
	RawExcerpt string            `json:"raw_excerpt,omitempty"`
	Refs       []string          `json:"refs,omitempty"`
	Attrs      map[string]string `json:"attrs,omitempty"`
}

type Hypothesis struct {
	ID                 string   `json:"id"`
	Category           string   `json:"category"`
	Title              string   `json:"title"`
	Status             string   `json:"status"`
	ConfidenceDelta    float64  `json:"confidence_delta,omitempty"`
	SupportingEvidence []string `json:"supporting_evidence"`
	RefutingEvidence   []string `json:"refuting_evidence,omitempty"`
	Rationale          string   `json:"rationale"`
}

type RootCause struct {
	Category        string   `json:"category"`
	Title           string   `json:"title"`
	ConfidenceScore float64  `json:"confidence_score"`
	ConfidenceLabel string   `json:"confidence_label"`
	Summary         string   `json:"summary"`
	EvidenceIDs     []string `json:"evidence_ids,omitempty"`
}

type Action struct {
	Priority    string `json:"priority"`
	Description string `json:"description"`
	Command     string `json:"command,omitempty"`
	Risk        string `json:"risk,omitempty"`
}

type Impact struct {
	Severity    string `json:"severity"`
	Description string `json:"description"`
}

type Verdict string

const (
	VerdictConfirmed    Verdict = "confirmed"
	VerdictLikely       Verdict = "likely"
	VerdictInconclusive Verdict = "inconclusive"
)

const (
	EnrichmentFull     = "full"
	EnrichmentDegraded = "degraded"
)

type EnrichmentInfo struct {
	Status        string   `json:"status"`
	DegradedSteps []string `json:"degraded_steps,omitempty"`
	Message       string   `json:"message,omitempty"`
}

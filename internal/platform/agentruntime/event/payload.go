package event

// Payload carries optional typed data on shared stream events.
type Payload struct {
	Kind string `json:"kind"`
	Data any    `json:"data,omitempty"`
}

const (
	PayloadKindTarget   = "target"
	PayloadKindError    = "error"
	PayloadKindMarkdown = "markdown"

	PayloadKindDiagnosisStage        = "diagnosis_stage"
	PayloadKindDiagnosisObservations = "diagnosis_observations"
	PayloadKindDiagnosisEvidence     = "diagnosis_evidence"
	PayloadKindDiagnosisHypothesis   = "diagnosis_hypothesis"
	PayloadKindDiagnosisVerification = "diagnosis_verification"
	PayloadKindDiagnosisReport       = "diagnosis_report"
	PayloadKindDiagnosisLLMStep      = "diagnosis_llm_step"
	PayloadKindDiagnosisEnrichment   = "diagnosis_enrichment"

	// Legacy aliases used during migration.
	PayloadKindEvidence     = PayloadKindDiagnosisEvidence
	PayloadKindHypothesis   = PayloadKindDiagnosisHypothesis
	PayloadKindVerification = PayloadKindDiagnosisVerification
	PayloadKindReport       = PayloadKindDiagnosisReport
)

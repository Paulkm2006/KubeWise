package runtime

type Phase int

const (
	PhaseIntake Phase = iota
	PhaseCollect
	PhaseEvidence
	PhaseHypothesis
	PhaseVerify
	PhaseReport
	PhaseDone
	PhaseFailed
)

func (p Phase) String() string {
	switch p {
	case PhaseIntake:
		return "diagnosis.intake"
	case PhaseCollect:
		return "diagnosis.collect"
	case PhaseEvidence:
		return "diagnosis.evidence"
	case PhaseHypothesis:
		return "diagnosis.hypothesis"
	case PhaseVerify:
		return "diagnosis.verify"
	case PhaseReport:
		return "diagnosis.report"
	case PhaseDone:
		return "diagnosis.done"
	case PhaseFailed:
		return "diagnosis.failed"
	default:
		return "diagnosis.unknown"
	}
}

func (p Phase) Stage() string {
	switch p {
	case PhaseIntake:
		return "intake"
	case PhaseCollect:
		return "collect"
	case PhaseEvidence, PhaseHypothesis:
		return "analyze"
	case PhaseVerify:
		return "verify"
	case PhaseReport:
		return "report"
	case PhaseDone:
		return "done"
	case PhaseFailed:
		return "failed"
	default:
		return "unknown"
	}
}

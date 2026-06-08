package state

// Phase is the deploy pipeline state machine step.
type Phase int

const (
	PhaseExtractApp Phase = iota
	PhaseResolveChart
	PhaseFetchDefaults
	PhaseGenerateValues
	PhaseReviewPlan
	PhasePreflight
	PhaseDeploy
	PhaseRecover
	PhaseVerify
	PhaseDone
	PhaseFailed
)

func (p Phase) String() string {
	switch p {
	case PhaseExtractApp:
		return "extract_app"
	case PhaseResolveChart:
		return "resolve_chart"
	case PhaseFetchDefaults:
		return "fetch_defaults"
	case PhaseGenerateValues:
		return "generate_values"
	case PhaseReviewPlan:
		return "review_plan"
	case PhasePreflight:
		return "preflight"
	case PhaseDeploy:
		return "deploy"
	case PhaseRecover:
		return "recover"
	case PhaseVerify:
		return "verify"
	case PhaseDone:
		return "done"
	case PhaseFailed:
		return "failed"
	default:
		return "unknown"
	}
}

func (p Phase) Terminal() bool {
	return p == PhaseDone || p == PhaseFailed
}

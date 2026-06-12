package nodes

import (
	"github.com/kubewise/kubewise/internal/platform/agentruntime/subagent/deploy/state"
)

// RunFunc executes one pipeline phase and mutates state (including Phase).
type RunFunc func(st *state.State) error

// Dispatch returns the node for the current phase.
func Dispatch(phase state.Phase) (RunFunc, bool) {
	switch phase {
	case state.PhaseExtractApp:
		return ExtractApp, true
	case state.PhaseResolveChart:
		return ResolveChart, true
	case state.PhaseFetchDefaults:
		return FetchDefaults, true
	case state.PhaseGenerateValues:
		return GenerateValues, true
	case state.PhaseReviewPlan:
		return ReviewPlan, true
	case state.PhasePreflight:
		return Preflight, true
	case state.PhaseDeploy:
		return DeployRelease, true
	case state.PhaseRecover:
		return RecoverDeployment, true
	case state.PhaseVerify:
		return VerifyRelease, true
	default:
		return nil, false
	}
}

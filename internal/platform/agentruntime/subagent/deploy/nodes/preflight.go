package nodes

import (
	"context"

	"go.uber.org/zap"

	"github.com/kubewise/kubewise/internal/agent/event"
	"github.com/kubewise/kubewise/internal/agent/subagent/deploy/core/plan"
	"github.com/kubewise/kubewise/internal/agent/subagent/deploy/state"
)

const toolPreflight = "helm preflight"

// Preflight validates the rendered chart before install.
func Preflight(st *state.State) error {
	st.Emit(event.Phase{QueryID: st.QueryID, Phase: "Helm 预检"})
	st.LogInfo("running helm preflight",
		zap.String("release", st.Plan.ReleaseName),
		zap.String("namespace", st.Plan.Namespace),
		zap.Bool("upgrade", st.Plan.IsUpgrade),
	)
	if err := st.RunTool(st.Ctx, toolPreflight, 5, func(ctx context.Context) error {
		return plan.RunHelmPreflight(ctx, st.Helm, st.Plan)
	}); err != nil {
		st.LogError("helm preflight failed", zap.Error(err))
		st.RecoveryErr = err
		st.Next(state.PhaseRecover)
		return nil
	}
	st.LogInfo("helm preflight passed")
	st.RecoveryErr = nil
	if st.RecoveryPendingReview {
		st.RecoveryPendingReview = false
		st.Next(state.PhaseReviewPlan)
		return nil
	}
	st.Next(state.PhaseDeploy)
	return nil
}

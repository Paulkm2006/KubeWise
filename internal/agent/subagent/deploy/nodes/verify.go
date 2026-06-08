package nodes

import (
	"context"

	"go.uber.org/zap"

	"github.com/kubewise/kubewise/internal/agent/subagent/deploy/core/report"
	"github.com/kubewise/kubewise/internal/agent/subagent/deploy/state"
	"github.com/kubewise/kubewise/internal/agent/event"
)

const toolVerifyDeploy = "verify deployment"

// VerifyRelease emits verification phase and builds the success report.
func VerifyRelease(st *state.State) error {
	st.Emit(event.Phase{QueryID: st.QueryID, Phase: "验证部署状态"})
	reportText, err := state.RunToolWithResult(st, st.Ctx, toolVerifyDeploy, 7, func(ctx context.Context) (string, error) {
		if st.BuildReport != nil {
			return st.BuildReport(ctx, st.Release, st.Chart, st.Plan.Namespace, st.ReleaseName), nil
		}
		return report.SuccessMessage(ctx, st.Release, st.Chart, st.Plan.Namespace, st.ReleaseName, st.K8s, st.Log), nil
	})
	if err != nil {
		return err
	}
	st.LogInfo("deploy succeeded", zap.String("release", st.Release.Name), zap.String("namespace", st.Release.Namespace))
	st.Done(reportText)
	return nil
}

package nodes

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/kubewise/kubewise/internal/agent/event"
	"github.com/kubewise/kubewise/internal/agent/subagent/deploy/core/recovery"
	"github.com/kubewise/kubewise/internal/agent/subagent/deploy/state"
	"github.com/kubewise/kubewise/internal/utils/helm"
)

const toolInstallUpgrade = "helm install/upgrade"

// DeployRelease runs helm install/upgrade and returns deployment failures as advice.
func DeployRelease(st *state.State) error {
	st.Emit(event.Phase{QueryID: st.QueryID, Phase: "执行部署"})
	st.LogInfo("helm install/upgrade starting",
		zap.String("release", st.ReleaseName),
		zap.String("namespace", st.Plan.Namespace),
	)

	rel, err := state.RunToolWithResult(st, st.Ctx, toolInstallUpgrade, 6, func(ctx context.Context) (*helm.Release, error) {
		return st.Helm.InstallOrUpgrade(ctx, helm.InstallOptions{
			ChartOptions: helm.ChartOptions{
				ReleaseName: st.ReleaseName,
				RepoName:    st.Chart.RepoName,
				ChartName:   st.Chart.ChartName,
				RepoURL:     st.Chart.RepoURL,
				Namespace:   st.Plan.Namespace,
				Values:      st.FinalValues,
			},
			CreateNS: true,
			Wait:     true,
			Timeout:  5 * time.Minute,
		})
	})
	if err != nil {
		st.LogError("helm install/upgrade failed", zap.Error(err))
		st.DeployErr = err
		if triage := recovery.ClassifyError(err); triage.Deterministic {
			st.LogInfo("recovery classified deterministic error",
				zap.String("class", string(triage.Class)),
				zap.String("reason", triage.Reason),
			)
			st.Done(triage.Report)
			return nil
		}
		st.Done(buildDeployFailureAdvice(err))
		return nil
	}

	st.Release = rel
	st.LogInfo("helm install/upgrade succeeded", zap.String("status", rel.Status))
	st.Next(state.PhaseVerify)
	return nil
}

func buildDeployFailureAdvice(err error) string {
	return fmt.Sprintf(`部署执行失败：%s

Helm values 已通过预检，问题更可能发生在集群运行期，例如镜像拉取、调度、权限、资源配额、Pod 启动探针或依赖服务状态。

当前 deploy agent 暂不自动处理这类运行期故障。建议使用 troubleshooting agent 继续诊断，或手动检查 release 状态、Pod 事件和相关日志。`, err)
}

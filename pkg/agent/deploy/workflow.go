package deploy

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/kubewise/kubewise/pkg/agent/deploy/core/report"
	"github.com/kubewise/kubewise/pkg/agent/deploy/nodes"
	"github.com/kubewise/kubewise/pkg/agent/deploy/plan"
	"github.com/kubewise/kubewise/pkg/agent/deploy/recovery"
	helmtools "github.com/kubewise/kubewise/pkg/agent/deploy/workflow/helm"
	"github.com/kubewise/kubewise/pkg/helm"
	"github.com/kubewise/kubewise/pkg/tui/events"
	"github.com/kubewise/kubewise/pkg/types"
)

func (a *Agent) runDeployPipeline(ctx context.Context, query string, entities types.Entities) (string, error) {
	st := &State{Query: query, Entities: entities}
	wf := a.workflowRunner()

	st.AppName = nodes.AppName(st.Entities, query)
	if st.AppName == "" {
		a.logWarn("could not extract app name from query", zap.String("query", query))
		return "", fmt.Errorf("无法从查询中提取应用名称，请明确指定要部署的应用")
	}

	st.ReleaseName = plan.SanitizeReleaseName(st.AppName)

	a.logInfo("deploy pipeline started",
		zap.String("app", st.AppName),
		zap.String("release", st.ReleaseName),
		zap.String("query", query),
	)

	a.emit(events.PhaseEvent{QueryID: a.queryID, Phase: fmt.Sprintf("搜索 Chart: %s", st.AppName)})

	var sel nodes.SelectChartFn
	if a.selectionHandler != nil {
		sel = a.selectionHandler.SelectChart
	}
	chosen, err := nodes.ResolveChart(ctx, st.AppName, sel, a.logger())
	if err != nil {
		a.logError("chart resolution failed", zap.String("app", st.AppName), zap.Error(err))
		return "", err
	}
	if chosen == nil {
		a.logInfo("chart selection cancelled", zap.String("app", st.AppName))
		return "部署已取消", nil
	}
	st.Chart = chosen

	a.logInfo("chart resolved",
		zap.String("app", st.AppName),
		zap.String("repo", st.Chart.RepoName),
		zap.String("chart", st.Chart.ChartName),
		zap.String("source", st.Chart.Source),
		zap.String("default_namespace", st.Chart.DefaultNamespace),
	)

	a.emit(events.PhaseEvent{QueryID: a.queryID, Phase: "获取 Chart 默认配置"})
	if err := nodes.AddHelmRepository(ctx, wf, a.helmClient, st.Chart.RepoName, st.Chart.RepoURL); err != nil {
		a.logError("helm repo add failed", zap.String("repo", st.Chart.RepoName), zap.Error(err))
		return "", err
	}
	a.logDebug("helm repo ready", zap.String("repo", st.Chart.RepoName))

	st.DefaultValues, err = nodes.HelmDefaultValues(ctx, wf, a.helmClient, st.Chart.RepoName, st.Chart.RepoURL, st.Chart.ChartName)
	if err != nil {
		a.logError("fetch default values failed", zap.String("chart", st.Chart.ChartName), zap.Error(err))
		return "", err
	}
	a.logDebug("default values fetched",
		zap.String("chart", st.Chart.ChartName),
		zap.Int("lines", countLines(st.DefaultValues)),
	)

	a.emit(events.PhaseEvent{QueryID: a.queryID, Phase: "生成配置建议"})
	genResult, err := nodes.GenerateValuesOverrides(ctx, wf, a.llmClient, query, st.Chart, st.DefaultValues)
	if err != nil {
		a.logError("values generation failed", zap.Error(err))
		return "", fmt.Errorf("生成 values 失败: %w", err)
	}
	st.GenResult = genResult

	a.logInfo("values generated",
		zap.String("namespace", genResult.Namespace),
		zap.String("risk_level", genResult.RiskLevel),
		zap.Int("override_lines", countLines(genResult.Values)),
	)

	nodes.EmitValuesGenerationNotes(a.emit, a.queryID, genResult)

	st.Plan = nodes.BuildDeployPlan(st.AppName, st.ReleaseName, st.Chart, st.DefaultValues, genResult)
	validateFn := a.makeValidatePlanFn()
	if err := validateFn(ctx, &st.Plan, "initial"); err != nil {
		return "", err
	}

	a.emit(events.PhaseEvent{QueryID: a.queryID, Phase: "等待用户确认"})
	a.logInfo("awaiting user confirmation",
		zap.String("namespace", st.Plan.Namespace),
		zap.String("release", st.Plan.ReleaseName),
		zap.Bool("upgrade", st.Plan.IsUpgrade),
		zap.Int("warnings", len(st.Plan.Warnings)),
	)

	st.FinalValues, st.CorrectionHistory, err = nodes.ReviewAndConfirm(
		ctx, wf, a.llmClient, a.helmClient,
		a.emit, a.queryID, query, st.Chart, st.DefaultValues, &st.Plan,
		a.confirmDeploy, validateFn, a.logger(), countLines,
	)
	if err != nil {
		return "", err
	}
	if st.FinalValues == "" {
		return "部署已取消", nil
	}

	a.emit(events.PhaseEvent{QueryID: a.queryID, Phase: "Helm 预检"})
	a.logInfo("running helm preflight",
		zap.String("release", st.Plan.ReleaseName),
		zap.String("namespace", st.Plan.Namespace),
		zap.Bool("upgrade", st.Plan.IsUpgrade),
	)
	if err := nodes.HelmPreflight(ctx, wf, a.helmClient, st.Plan, 5); err != nil {
		a.logError("helm preflight failed", zap.Error(err))
		return "", err
	}
	a.logInfo("helm preflight passed")

	a.emit(events.PhaseEvent{QueryID: a.queryID, Phase: "执行部署"})
	a.logInfo("helm install/upgrade starting",
		zap.String("release", st.ReleaseName),
		zap.String("namespace", st.Plan.Namespace),
	)
	rel, err := nodes.HelmInstallOrUpgrade(ctx, wf, a.helmClient, helmtools.InstallInput{
		ReleaseName: st.ReleaseName,
		Chart:       st.Chart,
		Namespace:   st.Plan.Namespace,
		Values:      st.FinalValues,
	}, 6)
	if err != nil {
		a.logError("helm install/upgrade failed", zap.Error(err))
		if triage := recovery.ClassifyError(err); triage.Deterministic {
			a.logInfo("recovery classified deterministic error",
				zap.String("class", string(triage.Class)),
				zap.String("reason", triage.Reason),
			)
			return triage.Report, nil
		}
		return a.runRecovery(ctx, err, query, st.CorrectionHistory, st.Chart, st.DefaultValues, st.FinalValues, st.Plan.Namespace, st.AppName)
	}

	st.Release = rel
	a.logInfo("helm install/upgrade succeeded", zap.String("status", rel.Status))
	reportText, err := nodes.VerifyDeployedRelease(ctx, wf, a.queryID, a.emit, rel, st.Chart, st.Plan.Namespace, st.ReleaseName, func(ctx context.Context) (string, error) {
		return report.SuccessMessage(ctx, rel, st.Chart, st.Plan.Namespace, st.ReleaseName, a.k8sClient, a.logger()), nil
	})
	if err != nil {
		return "", err
	}
	a.logInfo("deploy succeeded", zap.String("release", rel.Name), zap.String("namespace", rel.Namespace))
	return reportText, nil
}

func (a *Agent) makeValidatePlanFn() func(context.Context, *plan.DeployPlan, string) error {
	return func(ctx context.Context, p *plan.DeployPlan, stage string) error {
		return nodes.ValidatePlan(ctx, a.helmClient, p, stage, a.logPlanValidation, func(p *plan.DeployPlan, existing *helm.Release) {
			a.logDebug("existing release in target namespace",
				zap.String("release", p.ReleaseName),
				zap.String("namespace", p.Namespace),
				zap.String("status", existing.Status),
			)
		})
	}
}

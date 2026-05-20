// pkg/agent/deploy/agent.go
package deploy

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kubewise/kubewise/pkg/agent/deploy/plan"
	"github.com/kubewise/kubewise/pkg/agent/deploy/recovery"
	"github.com/kubewise/kubewise/pkg/agent/deploy/values"
	"github.com/kubewise/kubewise/pkg/agent/deploy/workflow"
	helmtools "github.com/kubewise/kubewise/pkg/agent/deploy/workflow/helm"
	"github.com/kubewise/kubewise/pkg/catalog"
	"github.com/kubewise/kubewise/pkg/helm"
	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/llm"
	"github.com/kubewise/kubewise/pkg/tool"
	"github.com/kubewise/kubewise/pkg/tui/events"
	"github.com/kubewise/kubewise/pkg/types"

	_ "github.com/kubewise/kubewise/pkg/tools/v1/query"
	_ "github.com/kubewise/kubewise/pkg/tools/v1/troubleshooting"
)

const toolUserConfirm = "user confirm"

// helmClient is the minimal helm.Client interface for deploy and tests.
type helmClient interface {
	helmtools.Client
	Status(ctx context.Context, releaseName, namespace string) (*helm.Release, error)
	ListReleases(ctx context.Context) ([]helm.Release, error)
}

// llmClient is the minimal llm.Client interface for tests.
type llmClient interface {
	values.LLMClient
	recovery.LLMClient
}

// DeployConfirmationHandler presents a deploy plan and waits for user decision.
type DeployConfirmationHandler interface {
	ConfirmDeploy(ctx context.Context, plan events.DeployPlan) (events.DeployDecision, error)
}

// ChartSelectionHandler presents chart candidates for user selection.
type ChartSelectionHandler interface {
	SelectChart(ctx context.Context, appName string, candidates []catalog.ChartInfo) (*catalog.ChartInfo, error)
}

// Agent orchestrates the deploy pipeline.
type Agent struct {
	llmClient        llmClient
	helmClient       helmClient
	confirmHandler   DeployConfirmationHandler
	selectionHandler ChartSelectionHandler
	eventCh          chan<- events.TUIEvent
	queryID          string
	log              *zap.Logger
	toolRegistry     *tool.Registry
	k8sClient        *k8s.Client
}

// SetLogger injects a logger for debug output.
func (a *Agent) SetLogger(l *zap.Logger) { a.log = l }

func (a *Agent) logger() *zap.Logger {
	if a.log == nil {
		return zap.NewNop()
	}
	return a.log
}

// Option configures the Agent.
type Option func(*Agent)

// WithConfirmHandler sets a custom confirmation handler.
func WithConfirmHandler(h DeployConfirmationHandler) Option {
	return func(a *Agent) { a.confirmHandler = h }
}

// WithSelectionHandler sets a custom chart selection handler.
func WithSelectionHandler(h ChartSelectionHandler) Option {
	return func(a *Agent) { a.selectionHandler = h }
}

// WithEventChannel sets the TUI event channel.
func WithEventChannel(ch chan<- events.TUIEvent, queryID string) Option {
	return func(a *Agent) {
		a.eventCh = ch
		a.queryID = queryID
	}
}

// New creates a Deploy Agent.
func New(llmClient *llm.Client, helmClient *helm.Client, k8sClient *k8s.Client, opts ...Option) *Agent {
	toolDep := tool.ToolDependency{K8sClient: k8sClient}
	registry, err := tool.LoadGlobalRegistryByCategory(toolDep, "")
	if err != nil {
		registry, _ = tool.LoadGlobalRegistryByCategory(tool.ToolDependency{}, "none")
	}

	a := &Agent{
		llmClient:    llmClient,
		helmClient:   helmClient,
		toolRegistry: registry,
		k8sClient:    k8sClient,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

func (a *Agent) workflowRunner() *workflow.Runner {
	return &workflow.Runner{QueryID: a.queryID, Emit: a}
}

// HandleQuery runs the deploy pipeline: resolve → values → validate → review → preflight → apply → verify.
func (a *Agent) HandleQuery(ctx context.Context, query string, entities types.Entities) (string, error) {
	a.emit(events.AgentStartEvent{AgentName: "Deploy Agent", QueryID: a.queryID})
	startTime := time.Now()
	defer func() {
		a.logInfo("deploy pipeline finished", zap.Duration("elapsed", time.Since(startTime)))
		a.emit(events.AgentDoneEvent{QueryID: a.queryID, Duration: time.Since(startTime)})
	}()

	appName := a.extractAppName(entities, query)
	if appName == "" {
		a.logWarn("could not extract app name from query", zap.String("query", query))
		return "", fmt.Errorf("无法从查询中提取应用名称，请明确指定要部署的应用")
	}

	releaseName := plan.SanitizeReleaseName(appName)
	wf := a.workflowRunner()

	a.logInfo("deploy pipeline started",
		zap.String("app", appName),
		zap.String("release", releaseName),
		zap.String("query", query),
	)

	a.emit(events.PhaseEvent{QueryID: a.queryID, Phase: fmt.Sprintf("搜索 Chart: %s", appName)})
	chartInfo, err := a.resolveChartFromArtifactHub(ctx, appName)
	if err != nil {
		a.logError("chart resolution failed", zap.String("app", appName), zap.Error(err))
		return "", err
	}
	if chartInfo == nil {
		a.logInfo("chart selection cancelled", zap.String("app", appName))
		return "部署已取消", nil
	}
	a.logInfo("chart resolved",
		zap.String("app", appName),
		zap.String("repo", chartInfo.RepoName),
		zap.String("chart", chartInfo.ChartName),
		zap.String("source", chartInfo.Source),
		zap.String("default_namespace", chartInfo.DefaultNamespace),
	)

	a.emit(events.PhaseEvent{QueryID: a.queryID, Phase: "获取 Chart 默认配置"})
	if err := helmtools.RepoAdd(ctx, wf, a.helmClient, helmtools.RepoAddInput{
		RepoName: chartInfo.RepoName,
		RepoURL:  chartInfo.RepoURL,
	}); err != nil {
		a.logError("helm repo add failed", zap.String("repo", chartInfo.RepoName), zap.Error(err))
		return "", err
	}
	a.logDebug("helm repo ready", zap.String("repo", chartInfo.RepoName))

	defaultValues, err := helmtools.ShowValues(ctx, wf, a.helmClient, helmtools.ShowValuesInput{
		RepoName:  chartInfo.RepoName,
		RepoURL:   chartInfo.RepoURL,
		ChartName: chartInfo.ChartName,
	})
	if err != nil {
		a.logError("fetch default values failed", zap.String("chart", chartInfo.ChartName), zap.Error(err))
		return "", err
	}
	a.logDebug("default values fetched",
		zap.String("chart", chartInfo.ChartName),
		zap.Int("lines", countLines(defaultValues)),
	)

	a.emit(events.PhaseEvent{QueryID: a.queryID, Phase: "生成配置建议"})
	genResult, err := values.Generate(ctx, wf, a.llmClient, values.GenerateInput{
		Query:         query,
		Chart:         chartInfo,
		DefaultValues: defaultValues,
	})
	if err != nil {
		a.logError("values generation failed", zap.Error(err))
		return "", fmt.Errorf("生成 values 失败: %w", err)
	}
	a.logInfo("values generated",
		zap.String("namespace", genResult.Namespace),
		zap.String("risk_level", genResult.RiskLevel),
		zap.Int("override_lines", countLines(genResult.Values)),
	)

	if genResult.Explanation != "" {
		a.emit(events.RenderTextEvent{QueryID: a.queryID, Text: genResult.Explanation})
		if genResult.RiskLevel == "high" {
			a.emit(events.RenderTextEvent{QueryID: a.queryID, Text: "⚠️ 配置风险等级: high，请仔细确认"})
		}
	}

	deployPlan := a.buildPlan(appName, releaseName, chartInfo, defaultValues, genResult)
	if err := a.validatePlan(ctx, &deployPlan, "initial"); err != nil {
		return "", err
	}

	a.emit(events.PhaseEvent{QueryID: a.queryID, Phase: "等待用户确认"})
	a.logInfo("awaiting user confirmation",
		zap.String("namespace", deployPlan.Namespace),
		zap.String("release", deployPlan.ReleaseName),
		zap.Bool("upgrade", deployPlan.IsUpgrade),
		zap.Int("warnings", len(deployPlan.Warnings)),
	)

	finalValues, correctionHistory, err := a.reviewAndConfirm(ctx, wf, query, chartInfo, defaultValues, &deployPlan)
	if err != nil {
		return "", err
	}
	if finalValues == "" {
		return "部署已取消", nil
	}

	a.emit(events.PhaseEvent{QueryID: a.queryID, Phase: "Helm 预检"})
	a.logInfo("running helm preflight",
		zap.String("release", deployPlan.ReleaseName),
		zap.String("namespace", deployPlan.Namespace),
		zap.Bool("upgrade", deployPlan.IsUpgrade),
	)
	if err := helmtools.Preflight(ctx, wf, a.helmClient, deployPlan, 5); err != nil {
		a.logError("helm preflight failed", zap.Error(err))
		return "", err
	}
	a.logInfo("helm preflight passed")

	a.emit(events.PhaseEvent{QueryID: a.queryID, Phase: "执行部署"})
	a.logInfo("helm install/upgrade starting",
		zap.String("release", releaseName),
		zap.String("namespace", deployPlan.Namespace),
	)
	rel, err := helmtools.InstallUpgrade(ctx, wf, a.helmClient, helmtools.InstallInput{
		ReleaseName: releaseName,
		Chart:       chartInfo,
		Namespace:   deployPlan.Namespace,
		Values:      finalValues,
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
		return a.runRecovery(ctx, err, query, correctionHistory, chartInfo, defaultValues, finalValues, deployPlan.Namespace, appName)
	}

	a.logInfo("helm install/upgrade succeeded", zap.String("status", rel.Status))
	return a.verifyAndReport(ctx, wf, rel, chartInfo, deployPlan.Namespace, releaseName)
}

func (a *Agent) buildPlan(appName, releaseName string, chart *catalog.ChartInfo, defaultValues string, gen *values.Result) plan.DeployPlan {
	customValues := plan.ApplyCRDValues(chart, defaultValues, gen.Values)
	p := plan.NewDeployPlan(appName, chart, defaultValues, customValues, gen.Namespace, false)
	p.ReleaseName = releaseName
	p.Warnings = append(p.Warnings, chartSelectionWarnings(appName, chart)...)
	return p
}

func (a *Agent) validatePlan(ctx context.Context, p *plan.DeployPlan, stage string) error {
	existingInTarget, _ := a.helmClient.Status(ctx, p.ReleaseName, p.Namespace)
	p.IsUpgrade = existingInTarget != nil
	if existingInTarget != nil {
		a.logDebug("existing release in target namespace",
			zap.String("release", p.ReleaseName),
			zap.String("namespace", p.Namespace),
			zap.String("status", existingInTarget.Status),
		)
	}

	validation := plan.ValidateDeployPlan(*p)
	validation.Merge(plan.CheckHelmReleaseConflicts(ctx, a.helmClient, *p))
	p.Warnings = append(p.Warnings, validation.Warnings...)
	a.logPlanValidation(stage, *p, validation)
	if validation.HasBlockingErrors() {
		return fmt.Errorf("部署计划校验失败: %s", strings.Join(validation.Errors, "; "))
	}
	return nil
}

func (a *Agent) reviewAndConfirm(
	ctx context.Context,
	wf *workflow.Runner,
	query string,
	chart *catalog.ChartInfo,
	defaultValues string,
	p *plan.DeployPlan,
) (finalValues string, correctionHistory []string, err error) {
	for {
		var decision events.DeployDecision
		confirmErr := wf.Run(ctx, workflow.Meta{Name: toolUserConfirm, Step: 4}, func(ctx context.Context) error {
			var e error
			decision, e = a.confirmDeploy(ctx, p.ToEventPlan())
			return e
		})
		if confirmErr != nil {
			return "", correctionHistory, fmt.Errorf("确认部署失败: %w", confirmErr)
		}

		if decision.Action == "cancel" {
			a.logInfo("deploy cancelled by user")
			return "", correctionHistory, nil
		}

		if decision.Correction == "" {
			p.CustomValues = decision.Values
			a.logInfo("user confirmed deploy plan", zap.Int("values_lines", countLines(decision.Values)))
			return decision.Values, correctionHistory, nil
		}

		a.logInfo("user requested values correction", zap.String("correction", decision.Correction))
		correctionHistory = append(correctionHistory, decision.Correction)
		regenResult, err := values.Regenerate(ctx, wf, a.llmClient, values.RegenerateInput{
			Query:         query,
			Chart:         chart,
			DefaultValues: defaultValues,
			CurrentValues: decision.Values,
			Correction:    decision.Correction,
		}, 5)
		if err != nil {
			a.logError("values regeneration failed", zap.Error(err))
			return "", correctionHistory, fmt.Errorf("重新生成 values 失败: %w", err)
		}
		a.logInfo("values regenerated",
			zap.String("namespace", regenResult.Namespace),
			zap.Int("override_lines", countLines(regenResult.Values)),
		)

		if regenResult.Explanation != "" {
			a.emit(events.RenderTextEvent{QueryID: a.queryID, Text: regenResult.Explanation})
		}

		p.CustomValues = plan.ApplyCRDValues(chart, defaultValues, regenResult.Values)
		if regenResult.Namespace != p.Namespace {
			p.Namespace = plan.SanitizeNamespace(regenResult.Namespace)
			if err := plan.ValidateNamespace(p.Namespace); err != nil {
				return "", correctionHistory, err
			}
			existingInTarget, _ := a.helmClient.Status(ctx, p.ReleaseName, p.Namespace)
			p.IsUpgrade = existingInTarget != nil
		}

		if err := a.validatePlan(ctx, p, "after_correction"); err != nil {
			return "", correctionHistory, err
		}
	}
}

func (a *Agent) runRecovery(
	ctx context.Context,
	deployErr error,
	query string,
	correctionHistory []string,
	chart *catalog.ChartInfo,
	defaultValues, finalValues, namespace, appName string,
) (string, error) {
	a.logInfo("entering recovery loop", zap.String("app", appName))
	runner := &recovery.Runner{
		QueryID:      a.queryID,
		LLM:          a.llmClient,
		Helm:         a.helmClient,
		Tools:        a.toolRegistry,
		Workflow:     a.workflowRunner(),
		K8s:          a.k8sClient,
		Confirm:      a.confirmDeploy,
		BuildReport:  a.buildReport,
		EmitPhase:    a.emit,
		EmitCritical: a.emitCritical,
		Log:          &recoveryLogger{agent: a},
	}
	result, recErr := runner.Run(ctx, recovery.RunInput{
		DeployErr:         deployErr,
		Query:             query,
		CorrectionHistory: correctionHistory,
		Chart:             chart,
		DefaultValues:     defaultValues,
		CurrentValues:     finalValues,
		TargetNS:          namespace,
		AppName:           appName,
	})
	if recErr != nil {
		a.logError("recovery loop error", zap.Error(recErr))
		return "", fmt.Errorf("诊断修复过程出错: %w", recErr)
	}
	a.logInfo("recovery loop finished",
		zap.Int("action", int(result.Action)),
		zap.String("reason", result.Reason),
	)
	return result.Details, nil
}

func (a *Agent) verifyAndReport(
	ctx context.Context,
	wf *workflow.Runner,
	rel *helm.Release,
	chart *catalog.ChartInfo,
	namespace, releaseName string,
) (string, error) {
	a.emit(events.PhaseEvent{QueryID: a.queryID, Phase: "验证部署状态"})
	report, err := workflow.RunWithResult(wf, ctx, workflow.Meta{Name: helmtools.ToolVerifyDeploy, Step: 7}, func(ctx context.Context) (string, error) {
		return a.buildReport(ctx, rel, chart, namespace, releaseName), nil
	})
	if err != nil {
		return "", err
	}
	a.logInfo("deploy succeeded", zap.String("release", rel.Name), zap.String("namespace", rel.Namespace))
	return report, nil
}

func (a *Agent) logPlanValidation(stage string, p plan.DeployPlan, validation plan.ValidationResult) {
	fields := []zap.Field{
		zap.String("stage", stage),
		zap.String("namespace", p.Namespace),
		zap.String("release", p.ReleaseName),
		zap.Int("errors", len(validation.Errors)),
		zap.Int("warnings", len(validation.Warnings)),
	}
	if len(validation.Errors) > 0 {
		fields = append(fields, zap.Strings("error_details", validation.Errors))
	}
	if len(validation.Warnings) > 0 {
		a.logWarn("deploy plan validation warnings", fields...)
		return
	}
	a.logDebug("deploy plan validation ok", fields...)
}

func (a *Agent) extractAppName(entities types.Entities, query string) string {
	if entities.AppName != "" {
		return entities.AppName
	}
	if entities.ResourceName != "" {
		return entities.ResourceName
	}
	return inferAppNameFromQuery(query)
}

func inferAppNameFromQuery(query string) string {
	q := strings.ToLower(strings.TrimSpace(query))
	for _, prefix := range []string{"部署", "安装", "deploy", "install"} {
		if idx := strings.Index(q, prefix); idx >= 0 {
			rest := strings.TrimSpace(q[idx+len(prefix):])
			fields := strings.Fields(rest)
			if len(fields) > 0 {
				return strings.Trim(fields[0], "，,. ")
			}
		}
	}
	return ""
}

func (a *Agent) confirmDeploy(ctx context.Context, p events.DeployPlan) (events.DeployDecision, error) {
	if a.confirmHandler == nil {
		return events.DeployDecision{Action: "execute", Values: p.CustomValues}, nil
	}
	return a.confirmHandler.ConfirmDeploy(ctx, p)
}

// Emit implements workflow.Emitter.
func (a *Agent) Emit(e events.TUIEvent) {
	a.emit(e)
}

func (a *Agent) emit(e events.TUIEvent) {
	if a.eventCh == nil {
		return
	}
	select {
	case a.eventCh <- e:
	default:
	}
}

func (a *Agent) emitCritical(e events.TUIEvent) {
	if a.eventCh == nil {
		return
	}
	a.eventCh <- e
}

func (a *Agent) buildReport(ctx context.Context, rel *helm.Release, chartInfo *catalog.ChartInfo, namespace, releaseName string) string {
	if rel == nil {
		return fmt.Sprintf("✅ %s 部署完成", chartInfo.ChartName)
	}
	ns := rel.Namespace
	if ns == "" {
		ns = namespace
	}
	rn := rel.Name
	if rn == "" {
		rn = releaseName
	}
	verifyNote := a.verifyWorkloadNote(ctx, ns, rn)
	return fmt.Sprintf(`✅ Helm 部署成功

Release:   %s
Namespace: %s
Chart:     %s (%s)
Status:    %s
%s
提示：kubectl get all -n %s`,
		rn,
		ns,
		chartInfo.ChartName,
		rel.Chart,
		rel.Status,
		verifyNote,
		ns,
	)
}

func (a *Agent) verifyWorkloadNote(ctx context.Context, namespace, releaseName string) string {
	if a.k8sClient == nil || namespace == "" {
		return ""
	}
	pods, err := a.k8sClient.ListPods(ctx, namespace)
	if err != nil {
		a.logWarn("post-deploy pod check failed", zap.String("namespace", namespace), zap.Error(err))
		return fmt.Sprintf("\n⚠️ 无法检查命名空间 %s 中的 Pod: %v\n", namespace, err)
	}
	if len(pods) == 0 {
		a.logWarn("helm deployed but no pods in namespace",
			zap.String("namespace", namespace),
			zap.String("release", releaseName),
		)
		return fmt.Sprintf(`
⚠️ 命名空间 %s 内没有 Pod。Helm 状态为 deployed 不代表主应用已运行。
常见原因：选错了 Chart（例如 argocd-apps 不会安装 Argo CD 本体），或 values 为空未启用任何组件。
请检查 Chart 选择，或执行 helm get manifest %s -n %s 查看实际创建的资源。
`, namespace, releaseName, namespace)
	}
	running := 0
	for _, p := range pods {
		switch p.Status.Phase {
		case "Running", "Pending":
			running++
		}
	}
	a.logInfo("post-deploy pod check",
		zap.String("namespace", namespace),
		zap.Int("pods", len(pods)),
		zap.Int("active", running),
	)
	if running == 0 {
		return fmt.Sprintf("\n⚠️ 命名空间 %s 有 %d 个 Pod，但无 Running/Pending 状态，请 kubectl describe pod -n %s\n", namespace, len(pods), namespace)
	}
	return fmt.Sprintf("\nPod: %d 个（%d Running/Pending）\n", len(pods), running)
}

type recoveryLogger struct {
	agent *Agent
}

func (l *recoveryLogger) Info(msg string, fields ...zap.Field)  { l.agent.logInfo(msg, fields...) }
func (l *recoveryLogger) Debug(msg string, fields ...zap.Field) { l.agent.logDebug(msg, fields...) }
func (l *recoveryLogger) Warn(msg string, fields ...zap.Field)  { l.agent.logWarn(msg, fields...) }
func (l *recoveryLogger) Error(msg string, fields ...zap.Field) { l.agent.logError(msg, fields...) }

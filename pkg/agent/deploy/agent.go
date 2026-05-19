// pkg/agent/deploy/agent.go
package deploy

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/kubewise/kubewise/pkg/catalog"
	"github.com/kubewise/kubewise/pkg/helm"
	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/llm"
	"github.com/kubewise/kubewise/pkg/tool"
	"github.com/kubewise/kubewise/pkg/tui/events"
	"github.com/kubewise/kubewise/pkg/types"

	// Load diagnostic tool definitions
	_ "github.com/kubewise/kubewise/pkg/tools/v1/query"
	_ "github.com/kubewise/kubewise/pkg/tools/v1/troubleshooting"
)

// --- internal interfaces (for testability) ---

// helmClient 是 helm.Client 的最小接口抽象，用于 mock 测试。
type helmClient interface {
	AddRepo(ctx context.Context, name, repoURL string) error
	FetchDefaultValues(ctx context.Context, repoName, repoURL, chartName string) (string, error)
	InstallOrUpgrade(ctx context.Context, opts helm.InstallOptions) (*helm.Release, error)
	Status(ctx context.Context, releaseName, namespace string) (*helm.Release, error)
}

// llmClient 是 llm.Client 的最小接口抽象，用于 mock 测试。
type llmClient interface {
	ChatCompletion(ctx context.Context, messages []llm.Message, functions []llm.FunctionDefinition) (*llm.Message, error)
}

// chartSearcher 抽象 ArtifactHub 搜索，用于 mock 测试。
type chartSearcher interface {
	SearchCandidates(ctx context.Context, appName string) ([]catalog.ChartInfo, error)
}

// DeployConfirmationHandler 负责向用户展示部署计划并等待决策。
type DeployConfirmationHandler interface {
	ConfirmDeploy(ctx context.Context, plan events.DeployPlan) (events.DeployDecision, error)
}

// ChartSelectionHandler 负责向用户展示 Chart 候选列表并等待选择。
type ChartSelectionHandler interface {
	SelectChart(ctx context.Context, appName string, candidates []catalog.ChartInfo) (*catalog.ChartInfo, error)
}

// Agent 是 Deploy Agent 的主体，实现六阶段部署流程。
type Agent struct {
	llmClient        llmClient
	helmClient       helmClient
	confirmHandler   DeployConfirmationHandler
	selectionHandler ChartSelectionHandler
	eventCh          chan<- events.TUIEvent
	queryID          string
	log              *zap.Logger
	toolRegistry      *tool.Registry
}

// SetLogger injects a logger for debug output.
func (a *Agent) SetLogger(l *zap.Logger) { a.log = l }

func (a *Agent) logger() *zap.Logger {
	if a.log == nil {
		return zap.NewNop()
	}
	return a.log
}

// Option 是 Agent 的函数式配置选项。
type Option func(*Agent)

// WithConfirmHandler 设置自定义确认处理器。
func WithConfirmHandler(h DeployConfirmationHandler) Option {
	return func(a *Agent) { a.confirmHandler = h }
}

// WithSelectionHandler 设置自定义 Chart 选择处理器。
func WithSelectionHandler(h ChartSelectionHandler) Option {
	return func(a *Agent) { a.selectionHandler = h }
}

// WithEventChannel 设置 TUI 事件通道。
func WithEventChannel(ch chan<- events.TUIEvent, queryID string) Option {
	return func(a *Agent) {
		a.eventCh = ch
		a.queryID = queryID
	}
}

// New 创建 Deploy Agent。
func New(llmClient *llm.Client, helmClient *helm.Client, k8sClient *k8s.Client, opts ...Option) *Agent {
	toolDep := tool.ToolDependency{K8sClient: k8sClient}
	registry, err := tool.LoadGlobalRegistryByCategory(toolDep, "")
	if err != nil {
		// Use empty registry on failure — diagnostics will skip tool calls
		registry, _ = tool.LoadGlobalRegistryByCategory(tool.ToolDependency{}, "none")
	}

	a := &Agent{
		llmClient:    llmClient,
		helmClient:   helmClient,
		toolRegistry: registry,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// HandleQuery 实现六阶段部署流程，每个阶段前后发送 PhaseEvent / ToolCallEvent / ToolDoneEvent
// 用于 TUI progressCard 实时显示进度。
func (a *Agent) HandleQuery(ctx context.Context, query string, entities types.Entities) (string, error) {
	a.emit(events.AgentStartEvent{AgentName: "Deploy Agent", QueryID: a.queryID})
	a.logger().Debug("handling deploy query", zap.String("query", query))
	startTime := time.Now()
	defer func() {
		a.emit(events.AgentDoneEvent{QueryID: a.queryID, Duration: time.Since(startTime)})
	}()

	// Phase 1: 提取应用名称
	appName := a.extractAppName(entities, query)
	if appName == "" {
		a.logger().Error("failed to extract app name from query", zap.String("query", query))
		return "", fmt.Errorf("无法从查询中提取应用名称，请明确指定要部署的应用")
	}

	a.emit(events.PhaseEvent{QueryID: a.queryID, Phase: fmt.Sprintf("搜索 Chart: %s", appName)})

	// Phase 2: 通过 ArtifactHub 搜索 Chart
	chartInfo, err := a.resolveChartFromArtifactHub(ctx, appName)
	if err != nil {
		return "", err
	}
	if chartInfo == nil {
		return "部署已取消", nil
	}

	// Phase 2.5: 检查是否已部署（先用 chart 默认 namespace）
	existingRelease, _ := a.helmClient.Status(ctx, appName, chartInfo.DefaultNamespace)

	// Phase 3: 获取默认 values — step 1 + step 2
	a.emit(events.PhaseEvent{QueryID: a.queryID, Phase: "获取 Chart 默认配置"})

	// Step 1: helm repo add
	a.emit(events.ToolCallEvent{QueryID: a.queryID, ToolName: "helm repo add", Step: 1})
	t0 := time.Now()
	if err := a.helmClient.AddRepo(ctx, chartInfo.RepoName, chartInfo.RepoURL); err != nil {
		a.logger().Error("helm repo add failed", zap.Error(err), zap.String("repo", chartInfo.RepoName))
		return "", fmt.Errorf("添加 Helm 仓库失败: %w", err)
	}
	a.emit(events.ToolDoneEvent{QueryID: a.queryID, ToolName: "helm repo add", Step: 1, Elapsed: time.Since(t0)})

	// Step 2: helm show values
	a.emit(events.ToolCallEvent{QueryID: a.queryID, ToolName: "helm show values", Step: 2})
	t0 = time.Now()
	defaultValues, err := a.helmClient.FetchDefaultValues(ctx, chartInfo.RepoName, chartInfo.RepoURL, chartInfo.ChartName)
	if err != nil {
		a.logger().Error("fetch default values failed", zap.Error(err), zap.String("chart", chartInfo.ChartName))
		return "", fmt.Errorf("获取默认 values 失败: %w", err)
	}
	a.emit(events.ToolDoneEvent{QueryID: a.queryID, ToolName: "helm show values", Step: 2, Elapsed: time.Since(t0)})

	// Phase 4: LLM 生成 override values + namespace — step 3
	a.emit(events.PhaseEvent{QueryID: a.queryID, Phase: "生成配置建议"})

	a.emit(events.ToolCallEvent{QueryID: a.queryID, ToolName: "LLM values generation", Step: 3})
	t0 = time.Now()
	genResult, err := generateValues(ctx, a.llmClient, query, chartInfo, defaultValues)
	if err != nil {
		a.logger().Error("llm values generation failed", zap.Error(err), zap.String("chart", chartInfo.ChartName))
		return "", fmt.Errorf("生成 values 失败: %w", err)
	}
	a.emit(events.ToolDoneEvent{QueryID: a.queryID, ToolName: "LLM values generation", Step: 3, Elapsed: time.Since(t0)})

	targetNS := genResult.namespace
	customValues := genResult.values
	a.logger().Info("llm recommended namespace",
		zap.String("namespace", targetNS),
		zap.String("chart", chartInfo.ChartName),
	)

	// 如果需要安装 CRDs，自动添加
	if chartInfo.InstallCRDs {
		customValues = "installCRDs: true\n" + customValues
	}

	// 如果 LLM 建议的 namespace 与 chart 默认不同，重新检查部署状态
	if targetNS != chartInfo.DefaultNamespace {
		existingRelease, _ = a.helmClient.Status(ctx, appName, targetNS)
	}

	// Phase 5: 人工审查 — 循环直到用户满意按 Y 执行
	a.emit(events.PhaseEvent{QueryID: a.queryID, Phase: "等待用户确认"})

	plan := events.DeployPlan{
		ChartInfo:     chartInfo,
		DefaultValues: defaultValues,
		CustomValues:  customValues,
		ReleaseName:   appName,
		Namespace:     targetNS,
		IsUpgrade:     existingRelease != nil,
	}

	var finalValues string
	var correctionHistory []string
	for {
		a.emit(events.ToolCallEvent{QueryID: a.queryID, ToolName: "user confirm", Step: 4})
		t0 = time.Now()
		decision, err := a.confirmDeploy(ctx, plan)
		if err != nil {
			return "", fmt.Errorf("确认部署失败: %w", err)
		}
		a.emit(events.ToolDoneEvent{QueryID: a.queryID, ToolName: "user confirm", Step: 4, Elapsed: time.Since(t0)})

		if decision.Action == "cancel" {
			return "部署已取消", nil
		}

		// 用户直接按 Y 执行（无修正指令）
		if decision.Correction == "" {
			finalValues = decision.Values
			break
		}

		// 用户按 C 或 E 修改了 values，重新生成
		if decision.Correction != "" {
			correctionHistory = append(correctionHistory, decision.Correction)
		}
		a.emit(events.ToolCallEvent{QueryID: a.queryID, ToolName: "LLM values regeneration", Step: 5})
		t0 = time.Now()
		regenResult, err := regenerateValues(ctx, a.llmClient, query, chartInfo, defaultValues, decision.Values, decision.Correction)
		if err != nil {
			a.logger().Error("llm values regeneration failed", zap.Error(err), zap.String("chart", chartInfo.ChartName))
			return "", fmt.Errorf("重新生成 values 失败: %w", err)
		}
		a.emit(events.ToolDoneEvent{QueryID: a.queryID, ToolName: "LLM values regeneration", Step: 5, Elapsed: time.Since(t0)})

		plan.CustomValues = regenResult.values
		if regenResult.namespace != targetNS {
			a.logger().Info("namespace changed via NL correction",
				zap.String("old", targetNS),
				zap.String("new", regenResult.namespace),
			)
			targetNS = regenResult.namespace
			plan.Namespace = targetNS
		}
	}

	// Phase 6: 执行 helm install/upgrade — 失败时进入自由诊断修复循环
	a.emit(events.PhaseEvent{QueryID: a.queryID, Phase: "执行部署"})

	a.emit(events.ToolCallEvent{QueryID: a.queryID, ToolName: "helm install/upgrade", Step: 6})
	t0 = time.Now()
	rel, err := a.helmClient.InstallOrUpgrade(ctx, helm.InstallOptions{
		ReleaseName: appName,
		RepoName:    chartInfo.RepoName,
		ChartName:   chartInfo.ChartName,
		RepoURL:     chartInfo.RepoURL,
		Namespace:   targetNS,
		Values:      finalValues,
		CreateNS:    true,
		Wait:        true,
		Timeout:     5 * time.Minute,
	})
	a.emit(events.ToolDoneEvent{QueryID: a.queryID, ToolName: "helm install/upgrade", Step: 6, Elapsed: time.Since(t0)})

	if err != nil {
		a.logger().Warn("helm install/upgrade failed, starting recovery loop",
			zap.Error(err),
			zap.String("release", appName),
		)

		result, recErr := a.recoverDeploy(ctx, err, query, correctionHistory, chartInfo, defaultValues, finalValues, targetNS, appName)
		if recErr != nil {
			return "", fmt.Errorf("诊断修复过程出错: %w", recErr)
		}

		// recoverDeploy already emitted Render*Events to TUI via submit_report
		// Return nil error so Router sends StreamDoneEvent (not StreamErrEvent)
		return result.Details, nil
	}

	// Phase 7: 验证部署状态 — step 7
	a.emit(events.PhaseEvent{QueryID: a.queryID, Phase: "验证部署状态"})

	a.emit(events.ToolCallEvent{QueryID: a.queryID, ToolName: "verify deployment", Step: 7})
	t0 = time.Now()
	report := a.buildReport(rel, chartInfo)
	a.emit(events.ToolDoneEvent{QueryID: a.queryID, ToolName: "verify deployment", Step: 7, Elapsed: time.Since(t0)})

	return report, nil
}

// extractAppName 从 entities 或 query 中提取应用名称。
func (a *Agent) extractAppName(entities types.Entities, query string) string {
	if entities.AppName != "" {
		return entities.AppName
	}
	if entities.ResourceName != "" {
		return entities.ResourceName
	}
	return ""
}

// resolveChartFromArtifactHub 通过 ArtifactHub 搜索 Chart，如果找到多个候选则通过 TUI 让用户选择。
func (a *Agent) resolveChartFromArtifactHub(ctx context.Context, appName string) (*catalog.ChartInfo, error) {
	ahResolver := catalog.NewArtifactHubResolver(nil)
	candidates, err := ahResolver.SearchCandidates(ctx, appName)

	if err != nil {
		a.logger().Warn("artifacthub search failed, showing manual input", zap.Error(err), zap.String("app", appName))
		candidates = nil
	}

	if a.selectionHandler == nil {
		if len(candidates) == 0 {
			return nil, fmt.Errorf("未找到应用 %q 的 Chart，请检查应用名称或手动指定 Chart 信息", appName)
		}
		// CLI 模式：自动选择第一个候选
		c := candidates[0]
		c.Source = "artifacthub"
		a.logger().Info("auto-selected first candidate from ArtifactHub",
			zap.String("app", appName),
			zap.String("repo", c.RepoName),
			zap.String("chart", c.ChartName),
		)
		return &c, nil
	}

	return a.selectionHandler.SelectChart(ctx, appName, candidates)
}

// confirmDeploy 调用确认处理器，如果未设置则返回默认执行决策。
func (a *Agent) confirmDeploy(ctx context.Context, plan events.DeployPlan) (events.DeployDecision, error) {
	if a.confirmHandler == nil {
		return events.DeployDecision{Action: "execute", Values: plan.CustomValues}, nil
	}
	return a.confirmHandler.ConfirmDeploy(ctx, plan)
}

// emit 向 TUI 事件通道发送事件（非阻塞）。
func (a *Agent) emit(e events.TUIEvent) {
	if a.eventCh == nil {
		return
	}
	select {
	case a.eventCh <- e:
	default:
	}
}

// buildReport 构建部署完成后的报告文本。
func (a *Agent) buildReport(rel *helm.Release, chartInfo *catalog.ChartInfo) string {
	if rel == nil {
		return fmt.Sprintf("✅ %s 部署完成", chartInfo.ChartName)
	}
	return fmt.Sprintf(`✅ 部署成功

Release:   %s
Namespace: %s
Chart:     %s
Status:    %s

提示：使用 kubectl get pods -n %s 查看 Pod 状态`,
		rel.Name,
		rel.Namespace,
		rel.Chart,
		rel.Status,
		rel.Namespace,
	)
}

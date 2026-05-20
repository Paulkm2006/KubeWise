package router

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kubewise/kubewise/pkg/agent/deploy"
	"github.com/kubewise/kubewise/pkg/agent/operation"
	"github.com/kubewise/kubewise/pkg/agent/query"
	"github.com/kubewise/kubewise/pkg/agent/security"
	"github.com/kubewise/kubewise/pkg/agent/troubleshooting"
	"github.com/kubewise/kubewise/pkg/catalog"
	"github.com/kubewise/kubewise/pkg/helm"
	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/llm"
	"github.com/kubewise/kubewise/pkg/tui/events"
	"github.com/kubewise/kubewise/pkg/types"
	"go.uber.org/zap"
)

// Agent 路由Agent
type Agent struct {
	k8sClient            *k8s.Client
	llmClient            *llm.Client
	queryAgent           *query.Agent
	troubleshootingAgent *troubleshooting.Agent
	securityAgent        *security.Agent
	operationAgent       *operation.Agent
	deployAgent          *deploy.Agent
	helmClient           *helm.Client
	log                  *zap.Logger
}

// SetLogger injects a logger and propagates it to all sub-agents.
func (a *Agent) SetLogger(l *zap.Logger) {
	a.log = l
	a.queryAgent.SetLogger(l)
	a.troubleshootingAgent.SetLogger(l)
	a.securityAgent.SetLogger(l)
	a.operationAgent.SetLogger(l)
	a.deployAgent.SetLogger(l)
	if a.helmClient != nil {
		a.helmClient.SetLogger(l)
	}
}

func (a *Agent) logger() *zap.Logger {
	if a.log == nil {
		return zap.NewNop()
	}
	return a.log
}

// New 创建路由Agent
func New(k8sClient *k8s.Client, llmClient *llm.Client) (*Agent, error) {
	queryAgent, err := query.New(k8sClient, llmClient)
	if err != nil {
		return nil, fmt.Errorf("初始化查询Agent失败: %w", err)
	}
	troubleshootingAgent, err := troubleshooting.New(k8sClient, llmClient)
	if err != nil {
		return nil, fmt.Errorf("初始化故障排查Agent失败: %w", err)
	}
	securityAgent, err := security.New(k8sClient, llmClient)
	if err != nil {
		return nil, fmt.Errorf("初始化安全审计Agent失败: %w", err)
	}
	operationAgent, err := operation.New(k8sClient, llmClient)
	if err != nil {
		return nil, fmt.Errorf("初始化操作Agent失败: %w", err)
	}
	helmClient := helm.New("")
	deployAgent := deploy.New(llmClient, helmClient, k8sClient)
	return &Agent{
		k8sClient:            k8sClient,
		llmClient:            llmClient,
		queryAgent:           queryAgent,
		troubleshootingAgent: troubleshootingAgent,
		securityAgent:        securityAgent,
		operationAgent:       operationAgent,
		deployAgent:          deployAgent,
		helmClient:           helmClient,
	}, nil
}

// HandleQuery 处理用户查询
func (a *Agent) HandleQuery(userQuery string) (string, error) {
	ctx := context.Background()

	// 1. 意图分类
	intent, err := a.classifyIntent(ctx, userQuery)
	if err != nil {
		a.logger().Error("intent classification failed", zap.Error(err))
		return "", fmt.Errorf("意图分类失败: %w", err)
	}
	a.logger().Info("intent classified",
		zap.String("task_type", string(intent.TaskType)),
		zap.Float64("confidence", intent.Confidence),
	)

	if intent.Entities.Namespace != "" {
	}
	if intent.Entities.ResourceName != "" && len(intent.Entities.ResourceType) > 0 {
	}

	// 2. 路由到对应的Agent处理
	switch intent.TaskType {
	case types.TaskTypeQuery:
		return a.queryAgent.HandleQuery(ctx, userQuery, intent.Entities)
	case types.TaskTypeOperation:
		return a.operationAgent.HandleQuery(ctx, userQuery, intent.Entities)
	case types.TaskTypeTroubleshooting:
		return a.troubleshootingAgent.HandleQuery(ctx, userQuery, intent.Entities)
	case types.TaskTypeSecurity:
		return a.securityAgent.HandleQuery(ctx, userQuery, intent.Entities)
	case types.TaskTypeDeploy:
		return a.deployAgent.HandleQuery(ctx, userQuery, intent.Entities)
	default:
		return "", fmt.Errorf("不支持的任务类型: %s", intent.TaskType)
	}
}

// HandleQueryStream classifies the query, creates fresh sub-agents with event
// channel support, routes to the appropriate sub-agent, and emits structured
// render events followed by StreamDoneEvent on success.
func (a *Agent) HandleQueryStream(ctx context.Context, userQuery, queryID string, eventCh chan<- events.TUIEvent) error {
	emit := func(e events.TUIEvent) {
		select {
		case eventCh <- e:
		default:
		}
	}

	// 1. Classify intent.
	emit(events.PhaseEvent{QueryID: queryID, Phase: "classifying intent"})
	intent, err := a.classifyIntent(ctx, userQuery)
	if err != nil {
		a.logger().Error("intent classification failed", zap.Error(err))
		emit(events.StreamErrEvent{QueryID: queryID, Err: err})
		return err
	}
	a.logger().Info("intent classified",
		zap.String("task_type", string(intent.TaskType)),
		zap.Float64("confidence", intent.Confidence),
	)

	var result string

	// 2. Route to the appropriate sub-agent (fresh instance with eventCh).
	phaseLabel := fmt.Sprintf("routing to %s agent", intent.TaskTypeDescription)
	emit(events.PhaseEvent{QueryID: queryID, Phase: phaseLabel})
	switch intent.TaskType {
	case types.TaskTypeQuery:
		ag, agErr := query.New(a.k8sClient, a.llmClient, query.WithEventCh(eventCh, queryID))
		if agErr != nil {
			emit(events.StreamErrEvent{QueryID: queryID, Err: agErr})
			return agErr
		}
		ag.SetLogger(a.log)
		result, err = ag.HandleQuery(ctx, userQuery, intent.Entities)

	case types.TaskTypeTroubleshooting:
		ag, agErr := troubleshooting.New(a.k8sClient, a.llmClient, troubleshooting.WithEventCh(eventCh, queryID))
		if agErr != nil {
			emit(events.StreamErrEvent{QueryID: queryID, Err: agErr})
			return agErr
		}
		ag.SetLogger(a.log)
		result, err = ag.HandleQuery(ctx, userQuery, intent.Entities)

	case types.TaskTypeSecurity:
		ag, agErr := security.New(a.k8sClient, a.llmClient, security.WithEventCh(eventCh, queryID))
		if agErr != nil {
			emit(events.StreamErrEvent{QueryID: queryID, Err: agErr})
			return agErr
		}
		ag.SetLogger(a.log)
		result, err = ag.HandleQuery(ctx, userQuery, intent.Entities)

	case types.TaskTypeDeploy:
		// Bridge goroutine: 将 Deploy Agent 的同步 ConfirmHandler / SelectionHandler
		// 转换为通过 eventCh 发送 RequestEvent → TUI → 接收 DoneMsg → 解阻塞 Agent。
		bridgeCtx, bridgeCancel := context.WithCancel(ctx)
		defer bridgeCancel()

		selectionHandler := &tuiChartSelectionHandler{
			eventCh:   eventCh,
			queryID:   queryID,
			bridgeCtx: bridgeCtx,
		}
		confirmHandler := &tuiDeployConfirmHandler{
			eventCh:   eventCh,
			queryID:   queryID,
			bridgeCtx: bridgeCtx,
		}

		deployAgentWithEvents := deploy.New(
			a.llmClient,
			a.helmClient,
			a.k8sClient,
			deploy.WithEventChannel(eventCh, queryID),
			deploy.WithSelectionHandler(selectionHandler),
			deploy.WithConfirmHandler(confirmHandler),
		)
		deployAgentWithEvents.SetLogger(a.log)
		result, err = deployAgentWithEvents.HandleQuery(ctx, userQuery, intent.Entities)

	case types.TaskTypeOperation:
		handler := operation.NewChannelConfirmationHandler()

		// Bridge goroutine: forwards ConfirmRequest → ConfirmRequestEvent → ConfirmResponse.
		bridgeCtx, bridgeCancel := context.WithCancel(ctx)
		defer bridgeCancel()
		go func() {
			for {
				select {
				case req, ok := <-handler.Requests:
					if !ok {
						return
					}
					respCh := make(chan any, 1)
					emit(events.ConfirmRequestEvent{
						QueryID:    queryID,
						Step:       req.Step,
						TotalSteps: req.TotalSteps,
						RespCh:     respCh,
					})
					select {
					case resp := <-respCh:
						if cr, ok := resp.(operation.ConfirmResponse); ok {
							select {
							case handler.Responses <- cr:
							case <-bridgeCtx.Done():
								return
							}
						}
					case <-bridgeCtx.Done():
						return
					}
				case <-bridgeCtx.Done():
					return
				}
			}
		}()

		opAgent, agErr := operation.New(
			a.k8sClient, a.llmClient,
			operation.WithConfirmationHandler(handler),
			operation.WithEventCh(eventCh, queryID),
		)
		if agErr != nil {
			emit(events.StreamErrEvent{QueryID: queryID, Err: agErr})
			return agErr
		}
		opAgent.SetLogger(a.log)
		result, err = opAgent.HandleQuery(ctx, userQuery, intent.Entities)

	default:
		a.logger().Error("unsupported task type", zap.String("task_type", string(intent.TaskType)))
		err = fmt.Errorf("不支持的任务类型: %s", intent.TaskType)
	}

	if err != nil {
		emit(events.StreamErrEvent{QueryID: queryID, Err: err})
		return err
	}

	emitRenderEvent(emit, queryID, result)
	emit(events.StreamDoneEvent{QueryID: queryID, Result: result})
	return nil
}

// emitRenderEvent detects the best render format for result and emits the
// corresponding event. Detection priority: YAML → JSON → Table → List → KV → Text.
func emitRenderEvent(emit func(events.TUIEvent), queryID, result string) {
	// 1. YAML code block.
	for line := range strings.SplitSeq(result, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "apiVersion:") || strings.HasPrefix(trimmed, "kind:") {
			emit(events.RenderCodeEvent{QueryID: queryID, Language: "yaml", Content: result})
			return
		}
	}

	// 2. JSON code block.
	trimmed := strings.TrimSpace(result)
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		emit(events.RenderCodeEvent{QueryID: queryID, Language: "json", Content: result})
		return
	}

	// 3. Table (pipe-delimited, ≥ 3 lines with "|").
	if headers, rows, ok := parseTable(result); ok {
		emit(events.RenderTableEvent{QueryID: queryID, Headers: headers, Rows: rows})
		return
	}

	// 4. Status list.
	// Build items from ALL non-empty lines; matched lines get their detected status,
	// others get "info".
	statusOf := make(map[int]string) // line index → status
	lines := strings.Split(result, "\n")
	matchCount := 0
	for i, line := range lines {
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		var status string
		switch {
		case containsAny(lower, "error", "failed", "crashloopbackoff", "unhealthy", "critical"):
			status = "error"
		case containsAny(lower, "pending", "terminating", "warning"):
			status = "warn"
		case containsAny(lower, "running", "healthy"):
			status = "ok"
		default:
			status = ""
		}
		if status != "" {
			statusOf[i] = status
			matchCount++
		}
	}
	if matchCount >= 2 {
		items := make([]events.ListItem, 0)
		for i, line := range lines {
			if line == "" {
				continue
			}
			status, matched := statusOf[i]
			if !matched {
				status = "info"
			}
			items = append(items, events.ListItem{Status: status, Text: line})
		}
		emit(events.RenderListEvent{QueryID: queryID, Items: items})
		return
	}

	// 5. KV pairs (key: value pattern).
	var kvLines []string
	var nonEmptyCount int
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		nonEmptyCount++
		if idx := strings.Index(l, ": "); idx > 0 {
			before := strings.TrimSpace(l[:idx])
			if before != "" && !strings.Contains(before, " ") {
				kvLines = append(kvLines, l)
			}
		}
	}
	if len(kvLines) >= 2 && nonEmptyCount > 0 && len(kvLines)*2 >= nonEmptyCount {
		pairs := make([]events.KVPair, 0, len(kvLines))
		for _, l := range kvLines {
			key, val, _ := strings.Cut(l, ": ")
			pairs = append(pairs, events.KVPair{
				Key:   strings.TrimSpace(key),
				Value: strings.TrimSpace(val),
			})
		}
		emit(events.RenderKVEvent{QueryID: queryID, Pairs: pairs})
		return
	}

	// 6. Default: plain text.
	emit(events.RenderTextEvent{QueryID: queryID, Text: result})
}

// containsAny reports whether s contains any of the given substrings.
func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// isSeparatorRow reports whether l is a markdown table separator row
// (every non-empty cell consists only of '-' and ':' characters).
func isSeparatorRow(line string) bool {
	hasCell := false
	for cell := range strings.SplitSeq(line, "|") {
		cell = strings.TrimSpace(cell)
		if cell == "" {
			continue
		}
		hasCell = true
		for _, ch := range cell {
			if ch != '-' && ch != ':' {
				return false
			}
		}
	}
	return hasCell
}

// parseTable tries to parse a pipe-delimited markdown table from result.
// Returns ok=true only when at least one header and one data row are found.
func parseTable(result string) (headers []string, rows [][]string, ok bool) {
	lines := strings.Split(result, "\n")
	var tableLines []string
	for _, l := range lines {
		if strings.Contains(l, "|") {
			tableLines = append(tableLines, l)
		}
	}
	if len(tableLines) < 3 {
		return nil, nil, false
	}
	for _, l := range tableLines {
		if isSeparatorRow(l) {
			continue
		}
		if len(headers) == 0 {
			for cell := range strings.SplitSeq(l, "|") {
				cell = strings.TrimSpace(cell)
				if cell != "" {
					headers = append(headers, cell)
				}
			}
		} else {
			var row []string
			for cell := range strings.SplitSeq(l, "|") {
				cell = strings.TrimSpace(cell)
				if cell != "" {
					row = append(row, cell)
				}
			}
			if len(row) > 0 {
				rows = append(rows, row)
			}
		}
	}
	return headers, rows, len(headers) > 0 && len(rows) > 0
}

// classifyIntent 意图分类
func (a *Agent) classifyIntent(ctx context.Context, userQuery string) (*types.IntentClassification, error) {
	systemPrompt := `你是Kubernetes智能运维系统的路由分析器，负责将用户的自然语言查询分类到以下五种任务类型之一：
1. operation（操作类）：用户需要对已有资源执行原子操作，如扩缩容、重启、删除 Pod/Deployment 等
2. query（查询类）：用户需要查询集群的状态、信息、统计等
3. troubleshooting（故障排查类）：用户需要排查异常、错误、崩溃等问题
4. security（安全审计类）：用户需要进行安全检查、权限审计、合规扫描等
5. deploy（应用部署类）：用户想要安装、部署、升级或卸载一个完整应用（如 ArgoCD、Prometheus、Nginx Ingress 等 Helm Chart）。与 operation 的区别：deploy 用于安装完整应用，operation 用于对已有资源的原子操作。

请分析用户查询，返回JSON格式的结果，包含：
- task_type: 任务类型枚举值（operation/query/troubleshooting/security/deploy）
- task_type_description: 任务类型中文描述
- entities: 提取的关键实体，包含：
  - namespace: 提到的命名空间（如果有）
  - resource_name: 提到的资源名称（如果有）
  - resource_type: 资源类型（Pod/Deployment/Service/PV/PVC等，如果有）
  - app_name: 应用名称（如果有）
  - operation: 操作类型（如果有）
- confidence: 分类置信度（0-1之间的浮点数）

注意：只返回JSON，不要有其他解释性文字。`

	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userQuery},
	}

	resp, err := a.llmClient.ChatCompletion(ctx, messages, nil)
	if err != nil {
		return nil, err
	}

	// 解析结果，支持各种格式
	var intent types.IntentClassification
	content := strings.TrimSpace(resp.Content)

	// 去掉可能的markdown包裹
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	if err := json.Unmarshal([]byte(content), &intent); err != nil {
		a.logger().Error("failed to parse intent JSON", zap.Error(err), zap.String("content", content))
		return nil, fmt.Errorf("解析意图结果失败: %w，原始内容: %s", err, content)
	}

	return &intent, nil
}

// tuiChartSelectionHandler 通过 eventCh 将 Chart 选择请求转发给 TUI，
// 阻塞等待用户在 TUI 中完成选择或取消。
type tuiChartSelectionHandler struct {
	eventCh   chan<- events.TUIEvent
	queryID   string
	bridgeCtx context.Context
}

func (h *tuiChartSelectionHandler) SelectChart(ctx context.Context, appName string, candidates []catalog.ChartInfo) (*catalog.ChartInfo, error) {
	respCh := make(chan *catalog.ChartInfo, 1)

	select {
	case h.eventCh <- events.ChartSelectRequestEvent{
		QueryID:    h.queryID,
		AppName:    appName,
		Candidates: candidates,
		RespCh:     respCh,
	}:
	case <-h.bridgeCtx.Done():
		return nil, h.bridgeCtx.Err()
	}

	select {
	case result := <-respCh:
		return result, nil
	case <-h.bridgeCtx.Done():
		return nil, h.bridgeCtx.Err()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// tuiDeployConfirmHandler 通过 eventCh 将 Deploy 确认请求转发给 TUI，
// 阻塞等待用户在 TUI 中执行、编辑或取消。
type tuiDeployConfirmHandler struct {
	eventCh   chan<- events.TUIEvent
	queryID   string
	bridgeCtx context.Context
}

func (h *tuiDeployConfirmHandler) ConfirmDeploy(ctx context.Context, plan events.DeployPlan) (events.DeployDecision, error) {
	respCh := make(chan events.DeployDecision, 1)

	select {
	case h.eventCh <- events.DeployConfirmRequestEvent{
		QueryID: h.queryID,
		Plan:    plan,
		RespCh:  respCh,
	}:
	case <-h.bridgeCtx.Done():
		return events.DeployDecision{Action: "cancel"}, h.bridgeCtx.Err()
	}

	select {
	case result := <-respCh:
		return result, nil
	case <-h.bridgeCtx.Done():
		return events.DeployDecision{Action: "cancel"}, h.bridgeCtx.Err()
	case <-ctx.Done():
		return events.DeployDecision{Action: "cancel"}, ctx.Err()
	}
}

package operation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/kubewise/kubewise/internal/config"
	"go.uber.org/zap"

	"github.com/kubewise/kubewise/internal/platform/agentruntime/event"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/loop"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/router/types"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/supervisor"
	operationtools "github.com/kubewise/kubewise/internal/platform/agentruntime/tool/operation"
	toolquery "github.com/kubewise/kubewise/internal/platform/agentruntime/tool/query"
	toolv2 "github.com/kubewise/kubewise/internal/platform/agentruntime/tool/v2"
	"github.com/kubewise/kubewise/internal/platform/cluster"
	"github.com/kubewise/kubewise/internal/utils/llm"
)

// writeRegistryI is satisfied by *toolv2.Registry and by mockRegistry in tests.
type writeRegistryI interface {
	Get(name string) (toolv2.Tool, bool)
}

// DefaultMaxSteps is the default maximum number of planning rounds.
const DefaultMaxSteps = 20

// Option configures the Agent.
type Option func(*Agent)

// WithConfirmationHandler injects a custom confirmation handler (for TUI/API use).
func WithConfirmationHandler(h ConfirmationHandler) Option {
	return func(a *Agent) { a.confirmHandler = h }
}

// WithEventChannel injects the stream event channel and query ID for streaming events.
func WithEventChannel(ch chan<- event.Event, queryID string) Option {
	return func(a *Agent) {
		a.eventCh = ch
		a.queryID = queryID
	}
}

// WithMaxSteps sets the maximum number of planning rounds.
func WithMaxSteps(n int) Option {
	return func(a *Agent) {
		if n > 0 {
			a.maxSteps = n
		}
	}
}

// WithSupervisorConfig configures the supervisor.
func WithSupervisorConfig(cfg supervisor.Config) Option {
	return func(a *Agent) {
		a.supervisorCfg = cfg
	}
}

// stepResult records the outcome of a single executed step.
type stepResult struct {
	step   OperationStep
	status string // "executed", "skipped", "failed"
	detail string
}

// Agent is the operation agent. It plans via LLM + read tools, then executes
// each step only after receiving user confirmation.
type Agent struct {
	k8sClient      *cluster.Client
	llmClient      *llm.Client
	readRegistry   *toolv2.Registry
	writeRegistry  writeRegistryI
	confirmHandler ConfirmationHandler
	eventCh        chan<- event.Event
	queryID        string
	maxSteps       int
	supervisorCfg  supervisor.Config
	inTokens       int
	outTokens      int
	log            *zap.Logger
}

// SetEventChannel sets the event channel and query ID for streaming progress.
func (a *Agent) SetEventChannel(eventCh chan<- event.Event, queryID string) {
	a.eventCh = eventCh
	a.queryID = queryID
}

// SetConfirmationHandler sets the confirmation handler for this agent.
func (a *Agent) SetConfirmationHandler(h ConfirmationHandler) {
	a.confirmHandler = h
}

// SetLogger injects a logger for debug output.

func (a *Agent) logger() *zap.Logger {
	return config.L()
}
func New(k8sClient *cluster.Client, llmClient *llm.Client, opts ...Option) (*Agent, error) {
	readReg := toolv2.NewRegistry()
	if err := toolquery.RegisterQueryTools(readReg, k8sClient); err != nil {
		return nil, fmt.Errorf("加载读工具注册中心失败: %w", err)
	}

	writeV2Reg, err := operationtools.NewOperationWriteRegistry(k8sClient)
	if err != nil {
		return nil, fmt.Errorf("加载写工具注册中心失败: %w", err)
	}

	a := &Agent{
		k8sClient:      k8sClient,
		llmClient:      llmClient,
		readRegistry:   readReg,
		writeRegistry:  writeV2Reg,
		confirmHandler: NewStdinConfirmationHandler(),
		maxSteps:       DefaultMaxSteps,
		supervisorCfg:  supervisor.DefaultConfig(),
	}
	for _, opt := range opts {
		opt(a)
	}
	return a, nil
}

// emit sends a stream event non-blocking. It is a no-op when eventCh is nil.
func (a *Agent) emit(e event.Event) {
	if a.eventCh == nil {
		return
	}
	select {
	case a.eventCh <- e:
	default:
	}
}

// accumulate adds token counts from an LLM response to the running totals.
func (a *Agent) accumulate(resp *llm.Message) {
	if resp.Usage != nil {
		a.inTokens += resp.Usage.PromptTokens
		a.outTokens += resp.Usage.CompletionTokens
	}
}

// HandleQuery is the entry point called by the router.
func (a *Agent) HandleQuery(ctx context.Context, userQuery string, entities types.Entities) (result string, err error) {
	a.inTokens = 0
	a.outTokens = 0
	start := time.Now()
	a.emit(event.AgentStart{AgentName: "Operation Agent", QueryID: a.queryID})
	a.logger().Debug("handling operation query", zap.String("query", userQuery))
	defer func() {
		a.emit(event.AgentDone{
			QueryID:   a.queryID,
			Result:    result,
			Duration:  time.Since(start),
			InTokens:  a.inTokens,
			OutTokens: a.outTokens,
		})
	}()

	fmt.Println("正在分析操作意图并规划执行步骤...")

	steps, err := a.plan(ctx, userQuery, entities)
	if err != nil {
		return "", fmt.Errorf("规划阶段失败: %w", err)
	}
	if len(steps) == 0 {
		return "未生成任何操作步骤", nil
	}

	return a.execute(ctx, steps)
}

// plan runs a ReAct loop with read-only tools to produce []OperationStep.
func (a *Agent) plan(ctx context.Context, userQuery string, _ types.Entities) ([]OperationStep, error) {
	submitToolDef := llm.FunctionDefinition{
		Name:        "submit_operation_plan",
		Description: "提交操作计划。在分析集群状态并确定操作步骤后，调用此工具提交计划列表。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"steps": map[string]any{
					"type":        "array",
					"description": "操作步骤列表，按执行顺序排列",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"step_index":     map[string]any{"type": "integer"},
							"operation_type": map[string]any{"type": "string", "enum": []string{"scale", "restart", "delete", "apply", "cordon_drain", "label_annotate"}},
							"resource_kind":  map[string]any{"type": "string"},
							"resource_name":  map[string]any{"type": "string"},
							"namespace":      map[string]any{"type": "string"},
							"group":          map[string]any{"type": "string"},
							"version":        map[string]any{"type": "string"},
							"resource":       map[string]any{"type": "string"},
							"replicas":       map[string]any{"type": "integer"},
							"action":         map[string]any{"type": "string", "enum": []string{"cordon", "uncordon", "drain"}},
							"labels":         map[string]any{"type": "object"},
							"annotations":    map[string]any{"type": "object"},
							"generated_yaml": map[string]any{"type": "string"},
							"description":    map[string]any{"type": "string"},
						},
						"required": []string{"step_index", "operation_type", "resource_name", "description"},
					},
				},
			},
			"required": []string{"steps"},
		},
	}

	functions := a.readRegistry.Definitions(a.readRegistry.Names())
	functions = append(functions, submitToolDef)

	messages := []llm.Message{
		{Role: "system", Content: a.buildPlanningSystemPrompt()},
		{Role: "user", Content: userQuery},
	}
	loopResult, err := loop.Run(ctx, loop.Config{
		QueryID:    a.queryID,
		LLM:        a.llmClient,
		Messages:   messages,
		Tools:      functions,
		Executor:   toolv2.NewExecutor(a.readRegistry),
		Policy:     toolv2.ExecutePolicy{AllowedCapabilities: []toolv2.Capability{toolv2.CapabilityRead}, AllowedTools: a.readRegistry.Names(), MaxOutputBytes: 20_000, EmitEvents: true},
		MaxSteps:   a.maxSteps,
		Emit:       a.emit,
		Supervisor: supervisor.New(a.supervisorCfg, a.llmClient),
		TerminalTools: map[string]loop.TerminalHandler{
			"submit_operation_plan": func(_ context.Context, call llm.ToolCall, _ []llm.Message) (loop.TerminalResult, error) {
				plan, err := parseOperationPlan(call.Function.Arguments)
				if err != nil {
					return loop.TerminalResult{ToolMessage: fmt.Sprintf("操作计划解析失败：%v\n请重新提交。", err)}, nil
				}
				return loop.TerminalResult{Done: true, Data: plan, Content: fmt.Sprintf("submitted %d operation steps", len(plan))}, nil
			},
		},
	})
	a.inTokens += loopResult.InTokens
	a.outTokens += loopResult.OutTokens
	if err != nil {
		return nil, fmt.Errorf("规划未完成（LLM 未调用 submit_operation_plan）: %w", err)
	}
	steps, ok := loopResult.TerminalData.([]OperationStep)
	if !ok || len(steps) == 0 {
		return nil, fmt.Errorf("规划未完成（LLM 未调用 submit_operation_plan），请重新描述您的操作需求")
	}
	return steps, nil
}

// execute iterates steps with per-step confirmation. Supports correction-based replan.
func (a *Agent) execute(ctx context.Context, steps []OperationStep) (string, error) {
	results := make([]stepResult, 0, len(steps))

	for _, step := range steps {
		const maxReplanAttempts = 2
		attempts := 0

		for {
			confirmed, correction, err := a.confirmHandler.Confirm(ctx, step, len(steps))
			if err != nil {
				return "", fmt.Errorf("确认交互失败: %w", err)
			}

			if confirmed {
				toolName, args, mappingErr := stepToToolCall(step)
				if mappingErr != nil {
					results = append(results, stepResult{step: step, status: "failed", detail: mappingErr.Error()})
					break
				}
				t, exists := a.writeRegistry.Get(toolName)
				if !exists {
					results = append(results, stepResult{step: step, status: "failed", detail: fmt.Sprintf("写工具 %s 未注册", toolName)})
					break
				}
				execResult, execErr := t.Execute(ctx, args)
				if execErr != nil {
					results = append(results, stepResult{step: step, status: "failed", detail: execErr.Error()})
				} else {
					results = append(results, stepResult{step: step, status: "executed", detail: toolv2.ToolResultToLLMMessage(execResult)})
				}
				break
			}

			if correction == "" {
				results = append(results, stepResult{step: step, status: "skipped"})
				break
			}

			attempts++
			if attempts > maxReplanAttempts {
				results = append(results, stepResult{step: step, status: "skipped", detail: "超过最大修正次数"})
				break
			}

			replanned, replanErr := a.replan(ctx, step, correction)
			if replanErr != nil {
				results = append(results, stepResult{step: step, status: "skipped", detail: replanErr.Error()})
				break
			}
			step = replanned
		}
	}

	return buildSummary(results), nil
}

// replan asks the LLM to revise a single step given the user's correction.
func (a *Agent) replan(ctx context.Context, original OperationStep, correction string) (OperationStep, error) {
	originalJSON, err := json.Marshal(original)
	if err != nil {
		return OperationStep{}, fmt.Errorf("failed to marshal original step: %w", err)
	}

	messages := []llm.Message{
		{Role: "system", Content: "你是 Kubernetes 操作规划助手。用户对某个操作步骤有修正意见，请根据用户的修正指令返回修改后的操作步骤 JSON，只返回一个 JSON 对象，不要有任何额外说明。"},
		{Role: "user", Content: fmt.Sprintf("原始操作步骤：\n%s\n\n用户修正指令：%s", string(originalJSON), correction)},
	}

	a.emit(event.Phase{QueryID: a.queryID, Phase: "thinking"})
	resp, err := a.llmClient.ChatCompletion(ctx, messages, nil, func(chunk llm.StreamChunk) {
		if chunk.Content != "" {
			a.emit(event.LLMTextDelta{QueryID: a.queryID, Delta: chunk.Content})
		}
	})
	if err != nil {
		return OperationStep{}, err
	}
	a.accumulate(resp)

	content := strings.TrimSpace(resp.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var revised OperationStep
	if err := json.Unmarshal([]byte(content), &revised); err != nil {
		return OperationStep{}, fmt.Errorf("修正结果 JSON 解析失败: %w", err)
	}
	return revised, nil
}

func parseOperationPlan(args map[string]any) ([]OperationStep, error) {
	stepsRaw, ok := args["steps"]
	if !ok {
		return nil, fmt.Errorf("submit_operation_plan 缺少 steps 参数")
	}
	data, err := json.Marshal(stepsRaw)
	if err != nil {
		return nil, err
	}
	var steps []OperationStep
	if err := json.Unmarshal(data, &steps); err != nil {
		return nil, fmt.Errorf("操作计划 JSON 解析失败: %w", err)
	}
	return steps, nil
}

func buildSummary(results []stepResult) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "\n操作执行完成，共 %d 步：\n", len(results))
	for _, r := range results {
		icon := map[string]string{"executed": "✓", "skipped": "○", "failed": "✗"}[r.status]
		fmt.Fprintf(&sb, "  %s 步骤%d [%s] %s\n", icon, r.step.StepIndex, r.status, r.step.Description)
		if r.detail != "" && r.status != "executed" {
			fmt.Fprintf(&sb, "      → %s\n", r.detail)
		}
	}
	return sb.String()
}

func (a *Agent) buildPlanningSystemPrompt() string {
	return `你是 Kubernetes 集群操作规划专家。你的任务是：
1. 使用查询工具了解集群当前状态（如确认资源存在、查询当前副本数等）
2. 规划出精确的操作步骤列表
3. 调用 submit_operation_plan 工具提交操作计划

支持的操作类型：
- scale: 调整副本数（支持 Deployment, StatefulSet），需填写 replicas 字段
- restart: 触发滚动重启（支持 Deployment, StatefulSet, DaemonSet）
- delete: 删除资源，需填写 group/version/resource（GVR）字段
- apply: 创建或更新资源，需在 generated_yaml 中填写完整的 YAML
- cordon_drain: 节点封锁/解封/驱逐，需填写 action（cordon/uncordon/drain）
- label_annotate: 修改标签/注解，需填写 group/version/resource 和 labels/annotations

注意事项：
- scale 操作前，请先查询当前副本数并在 description 中注明变化（如"3 → 5"）
- delete 操作前，请先确认资源存在
- apply 操作，generated_yaml 必须是完整合法的 Kubernetes YAML
- 尽量在一次回复中调用多个查询工具来并行收集信息，减少对话轮次

常见 GVR 对照：
- Pod: group="", version="v1", resource="pods"
- Deployment: group="apps", version="v1", resource="deployments"
- StatefulSet: group="apps", version="v1", resource="statefulsets"
- DaemonSet: group="apps", version="v1", resource="daemonsets"
- Service: group="", version="v1", resource="services"
- ConfigMap: group="", version="v1", resource="configmaps"
- Node: group="", version="v1", resource="nodes"`
}

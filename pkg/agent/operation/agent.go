package operation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kubewise/kubewise/pkg/agent/supervisor"
	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/llm"
	"github.com/kubewise/kubewise/pkg/stream"
	"github.com/kubewise/kubewise/pkg/tool"
	"github.com/kubewise/kubewise/pkg/types"

	// Trigger init() registration of all read tools
	_ "github.com/kubewise/kubewise/pkg/tools/v1/query"
	// Trigger init() registration of all write tools
	_ "github.com/kubewise/kubewise/pkg/tools/v1/operation"
)

// toolExecutor is the minimal interface needed from a registry tool.
type toolExecutor interface {
	Execute(ctx context.Context, args map[string]any) (string, error)
}

// writeRegistryI is satisfied by *tool.Registry and by mockRegistry in tests.
type writeRegistryI interface {
	GetTool(name string) (toolExecutor, bool)
}

// toolRegistryAdapter wraps *tool.Registry to satisfy writeRegistryI.
type toolRegistryAdapter struct{ reg *tool.Registry }

func (a *toolRegistryAdapter) GetTool(name string) (toolExecutor, bool) {
	t, ok := a.reg.GetTool(name)
	return t, ok
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
func WithEventChannel(ch chan<- stream.Event, queryID string) Option {
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
	k8sClient      *k8s.Client
	llmClient      *llm.Client
	readRegistry   *tool.Registry
	writeRegistry  writeRegistryI
	confirmHandler ConfirmationHandler
	eventCh        chan<- stream.Event
	queryID        string
	maxSteps       int
	supervisorCfg  supervisor.Config
	inTokens       int
	outTokens      int
	log            *zap.Logger
}

// SetEventChannel sets the event channel and query ID for streaming progress.
func (a *Agent) SetEventChannel(eventCh chan<- stream.Event, queryID string) {
	a.eventCh = eventCh
	a.queryID = queryID
}

// SetConfirmationHandler sets the confirmation handler for this agent.
func (a *Agent) SetConfirmationHandler(h ConfirmationHandler) {
	a.confirmHandler = h
}

// SetLogger injects a logger for debug output.
func (a *Agent) SetLogger(l *zap.Logger) { a.log = l }

func (a *Agent) logger() *zap.Logger {
	if a.log == nil {
		return zap.NewNop()
	}
	return a.log
}

// New creates a new Agent. Defaults to StdinConfirmationHandler.
func New(k8sClient *k8s.Client, llmClient *llm.Client, opts ...Option) (*Agent, error) {
	dep := tool.ToolDependency{K8sClient: k8sClient}

	readReg, err := tool.LoadGlobalRegistryByCategory(dep, "")
	if err != nil {
		return nil, fmt.Errorf("加载读工具注册中心失败: %w", err)
	}

	writeReg, err := tool.LoadGlobalRegistryByCategory(dep, "operation")
	if err != nil {
		return nil, fmt.Errorf("加载写工具注册中心失败: %w", err)
	}

	a := &Agent{
		k8sClient:      k8sClient,
		llmClient:      llmClient,
		readRegistry:   readReg,
		writeRegistry:  &toolRegistryAdapter{reg: writeReg},
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
func (a *Agent) emit(e stream.Event) {
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
	a.emit(stream.AgentStart{AgentName: "Operation Agent", QueryID: a.queryID})
	a.logger().Debug("handling operation query", zap.String("query", userQuery))
	defer func() {
		a.emit(stream.AgentDone{
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

	functions := a.readRegistry.GetAllFunctionDefinitions()
	functions = append(functions, submitToolDef)

	messages := []llm.Message{
		{Role: "system", Content: a.buildPlanningSystemPrompt()},
		{Role: "user", Content: userQuery},
	}

	sv := supervisor.New(a.supervisorCfg, a.llmClient)
	sv.Reset()
	iterationsRemaining := a.maxSteps

outer:
	for iterationsRemaining > 0 {
		for round := range iterationsRemaining {
			resp, err := a.llmClient.ChatCompletion(ctx, messages, functions, func(chunk llm.StreamChunk) {
				if chunk.Content != "" {
					a.emit(stream.LLMTextDelta{QueryID: a.queryID, Delta: chunk.Content})
				}
			})
			if err != nil {
				return nil, fmt.Errorf("LLM 调用失败: %w", err)
			}
			a.accumulate(resp)

			if len(resp.ToolCalls) == 0 {
				return nil, fmt.Errorf("规划未完成（LLM 未调用 submit_operation_plan），请重新描述您的操作需求")
			}

			// 执行所有工具调用（支持并行tool calls）
			messages = append(messages, *resp)
			var planResult []OperationStep
			for _, tc := range resp.ToolCalls {
				funcCall := &tc.Function

				if funcCall.Name == "submit_operation_plan" {
					plan, parseErr := parseOperationPlan(funcCall.Arguments)
					if parseErr != nil {
						messages = append(messages, llm.Message{
							Role: "tool", Content: fmt.Sprintf("操作计划解析失败：%v\n请重新提交。", parseErr), ToolCallID: tc.ID,
						})
						continue
					}
					planResult = plan
					continue
				}

				t, exists := a.readRegistry.GetTool(funcCall.Name)
				if !exists {
					messages = append(messages, llm.Message{
						Role: "tool", Content: fmt.Sprintf("工具 %s 不存在，请选择可用工具", funcCall.Name), ToolCallID: tc.ID,
					})
					continue
				}

				toolStart := time.Now()
				a.emit(stream.ToolCall{QueryID: a.queryID, ToolName: funcCall.Name, Step: round + 1})
				result, toolErr := t.Execute(ctx, funcCall.Arguments)
				a.emit(stream.ToolDone{QueryID: a.queryID, ToolName: funcCall.Name, Elapsed: time.Since(toolStart), Step: round + 1})
				if toolErr != nil {
					result = fmt.Sprintf("工具调用失败：%v\n请修正参数后重新调用。", toolErr)
				}

				messages = append(messages, llm.Message{
					Role:       "tool",
					Content:    fmt.Sprintf("工具返回结果：\n%s", result),
					ToolCallID: tc.ID,
				})
			}

			if planResult != nil {
				return planResult, nil
			}

			// Supervisor: cheap loop check after tool execution
			if triggered, loopResult := sv.CheckLoop(ctx, round, resp.ToolCalls, messages); triggered {
				a.emit(stream.Supervisor{
					QueryID:  a.queryID,
					Reason:   "loop detected",
					Decision: string(loopResult.Decision),
					Detail:   loopResult.Explanation,
				})
				switch loopResult.Decision {
				case supervisor.DecisionContinue:
					iterationsRemaining = loopResult.ExtraSteps
					continue outer
				case supervisor.DecisionReset:
					hint := loopResult.Hint
					if hint == "" {
						hint = "你已经陷入循环，请根据已获取的信息提交你的计划（调用 submit_operation_plan），或换一种方式来获取信息。"
					} else {
						hint = hint + " 请提交你的计划（调用 submit_operation_plan）。"
					}
					messages = append(messages, llm.Message{Role: "user", Content: hint})
					iterationsRemaining = a.maxSteps
					sv.Reset()
					continue outer
				case supervisor.DecisionAbort:
					return nil, fmt.Errorf("supervisor: %s", loopResult.Explanation)
				}
			}
		}

		// Reached maxSteps boundary — ask supervisor to evaluate
		evalResult, err := sv.Evaluate(ctx, messages, iterationsRemaining, a.maxSteps)
		if err != nil {
			return nil, fmt.Errorf("超过最大规划轮次（%d），无法生成操作计划，可通过 --max-steps 参数或 agent.max_steps 配置项调大", a.maxSteps)
		}
		a.emit(stream.Supervisor{
			QueryID:  a.queryID,
			Reason:   "max steps reached",
			Decision: string(evalResult.Decision),
			Detail:   evalResult.Explanation,
		})
		switch evalResult.Decision {
		case supervisor.DecisionContinue:
			iterationsRemaining = evalResult.ExtraSteps
		case supervisor.DecisionReset:
			hint := evalResult.Hint
			if hint == "" {
				hint = "你已经陷入循环，请根据已获取的信息提交你的计划（调用 submit_operation_plan），或换一种方式来获取信息。"
			} else {
				hint = hint + " 请提交你的计划（调用 submit_operation_plan）。"
			}
			messages = append(messages, llm.Message{Role: "user", Content: hint})
			iterationsRemaining = a.maxSteps
			sv.Reset()
		case supervisor.DecisionAbort:
			return nil, fmt.Errorf("supervisor: %s", evalResult.Explanation)
		}
	}

	return nil, fmt.Errorf("超过最大规划轮次，无法生成操作计划")
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
				t, exists := a.writeRegistry.GetTool(toolName)
				if !exists {
					results = append(results, stepResult{step: step, status: "failed", detail: fmt.Sprintf("写工具 %s 未注册", toolName)})
					break
				}
				execResult, execErr := t.Execute(ctx, args)
				if execErr != nil {
					results = append(results, stepResult{step: step, status: "failed", detail: execErr.Error()})
				} else {
					results = append(results, stepResult{step: step, status: "executed", detail: execResult})
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

	a.emit(stream.Phase{QueryID: a.queryID, Phase: "thinking"})
	resp, err := a.llmClient.ChatCompletion(ctx, messages, nil, func(chunk llm.StreamChunk) {
		if chunk.Content != "" {
			a.emit(stream.LLMTextDelta{QueryID: a.queryID, Delta: chunk.Content})
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

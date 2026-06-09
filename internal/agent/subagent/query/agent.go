package query

import (
	"context"
	"fmt"
	"time"

	"github.com/kubewise/kubewise/internal/agent/supervisor"
	"github.com/kubewise/kubewise/internal/cluster"
	"github.com/kubewise/kubewise/internal/utils/llm"
	"github.com/kubewise/kubewise/internal/agent/event"
	"github.com/kubewise/kubewise/internal/agent/tool"
	"github.com/kubewise/kubewise/internal/agent/router/types"
	"go.uber.org/zap"
	"github.com/kubewise/kubewise/internal/config"

	// 导入所有工具包，触发init函数注册工具
	_ "github.com/kubewise/kubewise/internal/agent/tool/v1/query"
)

// DefaultMaxSteps is the default maximum number of tool-calling rounds.
const DefaultMaxSteps = 20

// Option is a functional option for Agent.
type Option func(*Agent)

// WithEventChannel sets the stream event channel and query ID on the agent.
func WithEventChannel(ch chan<- event.Event, queryID string) Option {
	return func(a *Agent) {
		a.eventCh = ch
		a.queryID = queryID
	}
}

// WithMaxSteps sets the maximum number of tool-calling rounds.
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

// Agent 查询Agent
type Agent struct {
	k8sClient     *cluster.Client
	llmClient     *llm.Client
	toolRegistry  *tool.Registry
	eventCh       chan<- event.Event
	queryID       string
	log           *zap.Logger
	maxSteps      int
	supervisorCfg supervisor.Config
}

// SetLogger injects a logger for debug output.

func (a *Agent) logger() *zap.Logger {
	return config.L()
}
func (a *Agent) SetEventChannel(eventCh chan<- event.Event, queryID string) {
	a.eventCh = eventCh
	a.queryID = queryID
}

// emit sends an event to the event channel if one is set.
func (a *Agent) emit(e event.Event) {
	if a.eventCh == nil {
		return
	}
	select {
	case a.eventCh <- e:
	default:
	}
}

// New 创建查询Agent
func New(k8sClient *cluster.Client, llmClient *llm.Client, opts ...Option) (*Agent, error) {
	// 加载工具注册中心（必须成功，否则无法工作）
	toolDep := tool.ToolDependency{
		K8sClient: k8sClient,
	}
	registry, err := tool.LoadGlobalRegistryByCategory(toolDep, "")
	if err != nil {
		return nil, fmt.Errorf("加载工具注册中心失败: %w", err)
	}

	a := &Agent{
		k8sClient:     k8sClient,
		llmClient:     llmClient,
		toolRegistry:  registry,
		maxSteps:      DefaultMaxSteps,
		supervisorCfg: supervisor.DefaultConfig(),
	}
	for _, opt := range opts {
		opt(a)
	}
	return a, nil
}

// buildDynamicSystemPrompt 动态生成系统提示词
func (a *Agent) buildDynamicSystemPrompt() string {
	return `你是Kubernetes智能查询助手，可以调用工具来回答用户的问题。
你现在拥有通用资源查询能力：
1. 使用 list_resources_by_gvr 工具可以查询任意类型的Kubernetes资源列表（包括内置资源和自定义资源）
2. 使用 get_resource_by_gvr_and_name 工具可以查询任意单个Kubernetes资源的详细信息
3. 对于核心API组的资源（如pods、services、configmaps等），group参数请设置为空字符串""
4. 常见的资源GVR对照表：
   - Pod: group="", version="v1", resource="pods"
   - Service: group="", version="v1", resource="services"
   - ConfigMap: group="", version="v1", resource="configmaps"
   - Secret: group="", version="v1", resource="secrets"
   - Deployment: group="apps", version="v1", resource="deployments"
   - StatefulSet: group="apps", version="v1", resource="statefulsets"
   - DaemonSet: group="apps", version="v1", resource="daemonsets"
   - Job: group="batch", version="v1", resource="jobs"
   - CronJob: group="batch", version="v1", resource="cronjobs"
   - PersistentVolume: group="", version="v1", resource="persistentvolumes"
   - PersistentVolumeClaim: group="", version="v1", resource="persistentvolumeclaims"
   - Namespace: group="", version="v1", resource="namespaces"
   - Node: group="", version="v1", resource="nodes"

重要：尽量在一次回复中调用多个工具来并行收集信息，减少对话轮次。如果已经获取到足够的信息，请直接用自然语言回答用户的问题，不要调用不必要的工具。`
}

// HandleQuery 处理查询请求
func (a *Agent) HandleQuery(ctx context.Context, userQuery string, entities types.Entities) (result string, err error) {
	start := time.Now()
	var inTokens, outTokens int
	a.emit(event.AgentStart{AgentName: "Query Agent", QueryID: a.queryID})
	a.logger().Debug("handling query", zap.String("query", userQuery))
	defer func() {
		a.emit(event.AgentDone{
			QueryID:   a.queryID,
			Result:    result,
			Duration:  time.Since(start),
			InTokens:  inTokens,
			OutTokens: outTokens,
		})
	}()

	var systemPrompt string
	var functions []llm.FunctionDefinition

	// 如果工具注册中心可用，使用动态生成的工具列表
	functions = a.toolRegistry.GetAllFunctionDefinitions()
	systemPrompt = a.buildDynamicSystemPrompt()

	// 初始化消息历史
	messages := []llm.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userQuery},
	}

	sv := supervisor.New(a.supervisorCfg, a.llmClient)
	sv.Reset()
	iterationsRemaining := a.maxSteps

outer:
	for iterationsRemaining > 0 {
		for step := range iterationsRemaining {
			// 调用LLM（内部使用流式 API，onChunk 回调将 token delta 发出）
			a.emit(event.Phase{QueryID: a.queryID, Phase: "thinking"})
			resp, err := a.llmClient.ChatCompletion(ctx, messages, functions, func(chunk llm.StreamChunk) {
				if chunk.Content != "" {
					a.emit(event.LLMTextDelta{QueryID: a.queryID, Delta: chunk.Content})
				}
			})
			if err != nil {
				return "", fmt.Errorf("LLM调用失败: %w", err)
			}

			if resp.Usage != nil {
				inTokens += resp.Usage.PromptTokens
				outTokens += resp.Usage.CompletionTokens
			}

			// 检查是否有工具调用
			if len(resp.ToolCalls) == 0 {
				return resp.Content, nil
			}

			// 执行所有工具调用（支持并行tool calls）
			messages = append(messages, *resp)
			for _, tc := range resp.ToolCalls {
				funcCall := &tc.Function

				t, exists := a.toolRegistry.GetTool(funcCall.Name)
				if !exists {
					messages = append(messages, llm.Message{
						Role:       "tool",
						Content:    fmt.Sprintf("未知工具: %s", funcCall.Name),
						ToolCallID: tc.ID,
					})
					continue
				}
				a.emit(event.Phase{QueryID: a.queryID, Phase: fmt.Sprintf("running tool: %s", funcCall.Name)})
				toolStart := time.Now()
				a.emit(event.ToolCall{QueryID: a.queryID, ToolName: funcCall.Name, Step: step + 1})
				result, err := t.Execute(ctx, funcCall.Arguments)
				a.emit(event.ToolDone{QueryID: a.queryID, ToolName: funcCall.Name, Elapsed: time.Since(toolStart), Step: step + 1})

				if err != nil {
					result = fmt.Sprintf("工具调用失败：%v\n请修正参数后重新调用工具。", err)
				}

				messages = append(messages, llm.Message{
					Role:       "tool",
					Content:    fmt.Sprintf("工具返回结果：\n%s", result),
					ToolCallID: tc.ID,
				})
			}

			// Supervisor: cheap loop check after tool execution
			if triggered, loopResult := sv.CheckLoop(ctx, step, resp.ToolCalls, messages); triggered {
				a.emit(event.Supervisor{
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
					messages = append(messages, llm.Message{Role: "user", Content: loopResult.Hint})
					iterationsRemaining = a.maxSteps
					sv.Reset()
					continue outer
				case supervisor.DecisionAbort:
					return "", fmt.Errorf("supervisor: %s", loopResult.Explanation)
				}
			}
		}

		// Reached maxSteps boundary — ask supervisor to evaluate
		result, err := sv.Evaluate(ctx, messages, iterationsRemaining, a.maxSteps)
		if err != nil {
			return "", fmt.Errorf("超过最大调用轮次（%d），无法完成查询，可通过 --max-steps 参数或 agent.max_steps 配置项调大", a.maxSteps)
		}
		a.emit(event.Supervisor{
			QueryID:  a.queryID,
			Reason:   "max steps reached",
			Decision: string(result.Decision),
			Detail:   result.Explanation,
		})
		switch result.Decision {
		case supervisor.DecisionContinue:
			iterationsRemaining = result.ExtraSteps
		case supervisor.DecisionReset:
			messages = append(messages, llm.Message{Role: "user", Content: result.Hint})
			iterationsRemaining = a.maxSteps
			sv.Reset()
		case supervisor.DecisionAbort:
			return "", fmt.Errorf("supervisor: %s", result.Explanation)
		}
	}

	return "", fmt.Errorf("超过最大调用轮次，无法完成查询")
}

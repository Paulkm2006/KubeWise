package security

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/kubewise/kubewise/pkg/stream"
	"github.com/kubewise/kubewise/pkg/agent/supervisor"
	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/llm"
	"github.com/kubewise/kubewise/pkg/tool"
	"github.com/kubewise/kubewise/pkg/types"

	// 加载安全审计工具，触发init函数注册
	_ "github.com/kubewise/kubewise/pkg/tools/v1/security"
)

// DefaultMaxSteps is the default maximum number of tool-calling rounds.
const DefaultMaxSteps = 20

// Option is a functional option for Agent.
type Option func(*Agent)

// WithEventChannel sets the stream event channel and query ID on the agent.
func WithEventChannel(ch chan<- stream.Event, queryID string) Option {
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

// Agent 安全审计Agent
type Agent struct {
	k8sClient     *k8s.Client
	llmClient     *llm.Client
	toolRegistry  *tool.Registry
	eventCh       chan<- stream.Event
	queryID       string
	log           *zap.Logger
	maxSteps      int
	supervisorCfg supervisor.Config
}

// SetLogger injects a logger for debug output.
func (a *Agent) SetLogger(l *zap.Logger) { a.log = l }

func (a *Agent) logger() *zap.Logger {
	if a.log == nil {
		return zap.NewNop()
	}
	return a.log
}

// emit sends an event to the event channel if one is set.
func (a *Agent) emit(e stream.Event) {
	if a.eventCh == nil {
		return
	}
	select {
	case a.eventCh <- e:
	default:
	}
}

// New 创建安全审计Agent
func New(k8sClient *k8s.Client, llmClient *llm.Client, opts ...Option) (*Agent, error) {
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

// buildSystemPrompt 生成系统提示词
func (a *Agent) buildSystemPrompt() string {
	return `你是Kubernetes安全审计助手。你有四个审计工具可用：
- audit_rbac：审计RBAC配置（cluster-admin滥用、通配符权限、exec/portforward授权、孤立ServiceAccount）
- audit_pod_security：审计Pod安全配置（privileged容器、hostNetwork/hostPID/hostIPC、allowPrivilegeEscalation、root用户、hostPath）
- audit_network_policies：审计网络策略（无NetworkPolicy的命名空间、未覆盖的Pod）
- audit_image_security：审计镜像安全（latest标签、imagePullPolicy:Never、缺少imagePullSecrets）

## 响应策略

**针对具体问题的查询**（如"列出所有privileged pod"、"检查default命名空间的RBAC"）：
- 调用最相关的单个工具，使用用户指定的命名空间范围
- 直接返回工具结果，无需添加严重程度分组或修复建议

**针对全面审计的查询**（如"审计集群安全"、"检查所有安全问题"、"做一次安全扫描"）：
- 尽量在一次回复中调用全部四个工具，减少对话轮次
- 将结果整合为按严重程度分组的报告：Critical → High → Medium → Low
- 每类问题附上简要的修复建议

## 命名空间范围
如果用户提到了特定命名空间，在工具调用时传入 namespace 参数。否则留空（审计所有命名空间）。`
}

// HandleQuery 处理安全审计请求
func (a *Agent) HandleQuery(ctx context.Context, userQuery string, entities types.Entities) (string, error) {
	start := time.Now()
	var inTokens, outTokens int
	a.emit(stream.AgentStart{AgentName: "Security Agent", QueryID: a.queryID})
	a.logger().Debug("handling security query", zap.String("query", userQuery))
	defer func() {
		a.emit(stream.AgentDone{
			QueryID:   a.queryID,
			Duration:  time.Since(start),
			InTokens:  inTokens,
			OutTokens: outTokens,
		})
	}()

	functions := a.toolRegistry.GetAllFunctionDefinitions()

	userMsg := userQuery
	if len(entities.Namespace) > 0 {
		userMsg = fmt.Sprintf("%s\\n\\n（目标命名空间：%s）", userQuery, entities.Namespace[0])
	}

	messages := []llm.Message{
		{Role: "system", Content: a.buildSystemPrompt()},
		{Role: "user", Content: userMsg},
	}

	sv := supervisor.New(a.supervisorCfg, a.llmClient)
	sv.Reset()
	iterationsRemaining := a.maxSteps

outer:
	for iterationsRemaining > 0 {
		for step := range iterationsRemaining {
			a.emit(stream.Phase{QueryID: a.queryID, Phase: "thinking"})
			resp, err := a.llmClient.ChatCompletion(ctx, messages, functions, nil)
			if err != nil {
				return "", fmt.Errorf("LLM调用失败: %w", err)
			}

			if resp.Usage != nil {
				inTokens += resp.Usage.PromptTokens
				outTokens += resp.Usage.CompletionTokens
			}

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
				a.emit(stream.Phase{QueryID: a.queryID, Phase: fmt.Sprintf("running tool: %s", funcCall.Name)})
				toolStart := time.Now()
				a.emit(stream.ToolCall{QueryID: a.queryID, ToolName: funcCall.Name, Step: step + 1})
				result, err := t.Execute(ctx, funcCall.Arguments)
				a.emit(stream.ToolDone{QueryID: a.queryID, ToolName: funcCall.Name, Elapsed: time.Since(toolStart), Step: step + 1})
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
			return "", fmt.Errorf("超过最大调用轮次（%d），无法完成安全审计，可通过 --max-steps 参数或 agent.max_steps 配置项调大", a.maxSteps)
		}
		a.emit(stream.Supervisor{
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

	return "", fmt.Errorf("超过最大调用轮次，无法完成安全审计")
}

package security

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/llm"
	"github.com/kubewise/kubewise/pkg/tool"
	"github.com/kubewise/kubewise/pkg/tui/events"
	"github.com/kubewise/kubewise/pkg/types"

	// 加载安全审计工具，触发init函数注册
	_ "github.com/kubewise/kubewise/pkg/tools/v1/security"
)

// Option is a functional option for Agent.
type Option func(*Agent)

// WithEventCh sets an event channel and query ID on the agent.
func WithEventCh(ch chan<- events.TUIEvent, queryID string) Option {
	return func(a *Agent) {
		a.eventCh = ch
		a.queryID = queryID
	}
}

// Agent 安全审计Agent
type Agent struct {
	k8sClient    *k8s.Client
	llmClient    *llm.Client
	toolRegistry *tool.Registry
	eventCh      chan<- events.TUIEvent
	queryID      string
}

// emit sends an event to the event channel if one is set.
func (a *Agent) emit(e events.TUIEvent) {
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
		k8sClient:    k8sClient,
		llmClient:    llmClient,
		toolRegistry: registry,
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
- 依次调用全部四个工具
- 将结果整合为按严重程度分组的报告：Critical → High → Medium → Low
- 每类问题附上简要的修复建议

## 命名空间范围
如果用户提到了特定命名空间，在工具调用时传入 namespace 参数。否则留空（审计所有命名空间）。`
}

// HandleQuery 处理安全审计请求
func (a *Agent) HandleQuery(ctx context.Context, userQuery string, entities types.Entities) (string, error) {
	start := time.Now()
	var inTokens, outTokens int
	a.emit(events.AgentStartEvent{AgentName: "Security Agent", QueryID: a.queryID})
	defer func() {
		a.emit(events.AgentDoneEvent{
			QueryID:   a.queryID,
			Duration:  time.Since(start),
			InTokens:  inTokens,
			OutTokens: outTokens,
		})
	}()

	functions := a.toolRegistry.GetAllFunctionDefinitions()

	userMsg := userQuery
	if entities.Namespace != "" {
		userMsg = fmt.Sprintf("%s\n\n（目标命名空间：%s）", userQuery, entities.Namespace)
	}

	messages := []llm.Message{
		{Role: "system", Content: a.buildSystemPrompt()},
		{Role: "user", Content: userMsg},
	}

	maxSteps := 10
	for step := range maxSteps {
		a.emit(events.PhaseEvent{QueryID: a.queryID, Phase: "thinking"})
		resp, err := a.llmClient.ChatCompletion(ctx, messages, functions)
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

		funcCall := &resp.ToolCalls[0].Function

		fmt.Printf("第%d步：调用工具 %s\n", step+1, funcCall.Name)
		if len(funcCall.Arguments) > 0 {
			args := make([]string, 0, len(funcCall.Arguments))
			for k, v := range funcCall.Arguments {
				args = append(args, fmt.Sprintf("%s=%v", k, v))
			}
			fmt.Printf("参数：%s\n", strings.Join(args, ", "))
		}

		t, exists := a.toolRegistry.GetTool(funcCall.Name)
		if !exists {
			return "", fmt.Errorf("未知工具: %s", funcCall.Name)
		}
		a.emit(events.PhaseEvent{QueryID: a.queryID, Phase: fmt.Sprintf("running tool: %s", funcCall.Name)})
		toolStart := time.Now()
		a.emit(events.ToolCallEvent{QueryID: a.queryID, ToolName: funcCall.Name, Step: step + 1})
		result, err := t.Execute(ctx, funcCall.Arguments)
		a.emit(events.ToolDoneEvent{QueryID: a.queryID, ToolName: funcCall.Name, Elapsed: time.Since(toolStart), Step: step + 1})
		if err != nil {
			fmt.Printf("工具调用失败：%v\n", err)
			result = fmt.Sprintf("工具调用失败：%v\n请修正参数后重新调用工具。", err)
		} else {
			fmt.Printf("工具返回结果长度：%d 字节\n", len(result))
		}

		messages = append(messages, *resp)
		toolMsg := llm.Message{
			Role:    "tool",
			Content: fmt.Sprintf("工具返回结果：\n%s", result),
		}
		if len(resp.ToolCalls) > 0 {
			toolMsg.ToolCallID = resp.ToolCalls[0].ID
		}
		messages = append(messages, toolMsg)
	}

	return "", fmt.Errorf("超过最大调用轮次，无法完成安全审计")
}

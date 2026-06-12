package security

import (
	"context"
	"fmt"
	"time"

	"github.com/kubewise/kubewise/internal/config"
	"go.uber.org/zap"

	"github.com/kubewise/kubewise/internal/platform/agentruntime/event"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/loop"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/router/types"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/supervisor"
	securitytools "github.com/kubewise/kubewise/internal/platform/agentruntime/tool/security"
	toolv2 "github.com/kubewise/kubewise/internal/platform/agentruntime/tool/v2"
	"github.com/kubewise/kubewise/internal/platform/cluster"
	"github.com/kubewise/kubewise/internal/utils/llm"
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

// Agent 安全审计Agent
type Agent struct {
	k8sClient     *cluster.Client
	llmClient     *llm.Client
	eventCh       chan<- event.Event
	queryID       string
	log           *zap.Logger
	maxSteps      int
	supervisorCfg supervisor.Config
}

// SetEventChannel sets the event channel and query ID for streaming progress.
func (a *Agent) SetEventChannel(eventCh chan<- event.Event, queryID string) {
	a.eventCh = eventCh
	a.queryID = queryID
}

// SetLogger injects a logger for debug output.

func (a *Agent) logger() *zap.Logger {
	return config.L()
}
func (a *Agent) emit(e event.Event) {
	if a.eventCh == nil {
		return
	}
	select {
	case a.eventCh <- e:
	default:
	}
}

// New 创建安全审计Agent
func New(k8sClient *cluster.Client, llmClient *llm.Client, opts ...Option) (*Agent, error) {
	a := &Agent{
		k8sClient:     k8sClient,
		llmClient:     llmClient,
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
func (a *Agent) HandleQuery(ctx context.Context, userQuery string, entities types.Entities) (result string, err error) {
	start := time.Now()
	var inTokens, outTokens int
	a.emit(event.AgentStart{AgentName: "Security Agent", QueryID: a.queryID})
	a.logger().Debug("handling security query", zap.String("query", userQuery))
	defer func() {
		a.emit(event.AgentDone{
			QueryID:   a.queryID,
			Result:    result,
			Duration:  time.Since(start),
			InTokens:  inTokens,
			OutTokens: outTokens,
		})
	}()

	userMsg := userQuery
	if len(entities.Namespace) > 0 {
		userMsg = fmt.Sprintf("%s\\n\\n（目标命名空间：%s）", userQuery, entities.Namespace[0])
	}

	v2reg := toolv2.NewRegistry()
	if err := securitytools.RegisterAuditTools(v2reg, a.k8sClient); err != nil {
		return "", fmt.Errorf("加载安全审计工具失败: %w", err)
	}
	allowedTools := toolv2.NewBundleSet(securitytools.NewAuditBundle()).Tools(toolv2.BundleSecurityAudit)
	messages := []llm.Message{
		{Role: "system", Content: a.buildSystemPrompt()},
		{Role: "user", Content: userMsg},
	}
	loopResult, err := loop.Run(ctx, loop.Config{
		QueryID:  a.queryID,
		LLM:      a.llmClient,
		Messages: messages,
		Tools:    v2reg.Definitions(allowedTools),
		Executor: toolv2.NewExecutor(v2reg),
		Policy: toolv2.ExecutePolicy{
			AllowedCapabilities: []toolv2.Capability{toolv2.CapabilityAudit},
			AllowedTools:        allowedTools,
			MaxOutputBytes:      20_000,
			EmitEvents:          true,
		},
		MaxSteps: a.maxSteps,
		Emit:     a.emit,
	})
	if err != nil {
		return "", fmt.Errorf("LLM调用失败: %w", err)
	}
	inTokens = loopResult.InTokens
	outTokens = loopResult.OutTokens
	return loopResult.Content, nil
}

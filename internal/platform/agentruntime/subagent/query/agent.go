package query

import (
	"context"
	"fmt"
	"time"

	"github.com/kubewise/kubewise/internal/config"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/event"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/loop"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/router/types"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/supervisor"
	toolquery "github.com/kubewise/kubewise/internal/platform/agentruntime/tool/query"
	toolv2 "github.com/kubewise/kubewise/internal/platform/agentruntime/tool/v2"
	"github.com/kubewise/kubewise/internal/platform/cluster"
	"github.com/kubewise/kubewise/internal/utils/llm"
	"go.uber.org/zap"
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

	v2reg := toolv2.NewRegistry()
	if err := toolquery.RegisterQueryTools(v2reg, a.k8sClient); err != nil {
		return "", err
	}
	defs := v2reg.Definitions(v2reg.Names())
	messages := []llm.Message{
		{Role: "system", Content: a.buildDynamicSystemPrompt()},
		{Role: "user", Content: userQuery},
	}
	loopResult, err := loop.Run(ctx, loop.Config{
		QueryID:  a.queryID,
		LLM:      a.llmClient,
		Messages: messages,
		Tools:    defs,
		Executor: toolv2.NewExecutor(v2reg),
		Policy: toolv2.ExecutePolicy{
			AllowedCapabilities: []toolv2.Capability{toolv2.CapabilityRead},
			AllowedTools:        v2reg.Names(),
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

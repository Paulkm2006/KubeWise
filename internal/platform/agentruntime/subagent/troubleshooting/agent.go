package troubleshooting

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/kubewise/kubewise/internal/config"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/event"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/loop"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/router/types"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/supervisor"
	toolquery "github.com/kubewise/kubewise/internal/platform/agentruntime/tool/query"
	tooltroubleshooting "github.com/kubewise/kubewise/internal/platform/agentruntime/tool/troubleshooting"
	toolv2 "github.com/kubewise/kubewise/internal/platform/agentruntime/tool/v2"
	"github.com/kubewise/kubewise/internal/platform/cluster"
	"github.com/kubewise/kubewise/internal/utils/llm"
	"go.uber.org/zap"
)

const DefaultMaxSteps = 20

type Option func(*Agent)

func WithEventChannel(ch chan<- event.Event, queryID string) Option {
	return func(a *Agent) {
		a.eventCh = ch
		a.queryID = queryID
	}
}

func WithMaxSteps(n int) Option {
	return func(a *Agent) {
		if n > 0 {
			a.maxSteps = n
		}
	}
}

func WithSupervisorConfig(cfg supervisor.Config) Option {
	return func(a *Agent) {
		a.supervisorCfg = cfg
	}
}

type Agent struct {
	k8sClient     *cluster.Client
	llmClient     *llm.Client
	eventCh       chan<- event.Event
	queryID       string
	maxSteps      int
	supervisorCfg supervisor.Config
}

func (a *Agent) SetEventChannel(eventCh chan<- event.Event, queryID string) {
	a.eventCh = eventCh
	a.queryID = queryID
}

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

func (a *Agent) buildSystemPrompt() string {
	return `你是Kubernetes智能故障排查助手。当用户描述集群异常时，你需要系统性地收集信息并诊断根因。

## 信息收集顺序
1. 先获取目标资源的状态（使用 get_resource_by_gvr_and_name 或 list_resources_by_gvr）
2. 获取该资源的事件（使用 get_resource_events）
3. 如果是Pod问题，获取日志（使用 get_pod_logs）
4. 如果涉及调度失败，检查节点状态（使用 get_node_status）
5. 如果涉及Service不可达，检查Endpoints（使用 get_service_endpoints）

## 常见资源GVR参照表
- Pod: group="", version="v1", resource="pods"
- Service: group="", version="v1", resource="services"
- PersistentVolumeClaim: group="", version="v1", resource="persistentvolumeclaims"
- PersistentVolume: group="", version="v1", resource="persistentvolumes"
- Deployment: group="apps", version="v1", resource="deployments"
- StatefulSet: group="apps", version="v1", resource="statefulsets"
- Node: group="", version="v1", resource="nodes"
- IngressRoute (Traefik): group="traefik.io", version="v1alpha1", resource="ingressroutes"

对于不确定的CRD，可以先用 list_resources_by_gvr 尝试，或向用户确认GVR信息。

重要：尽量在一次回复中调用多个工具来并行收集信息，减少对话轮次。

## 输出格式
收集到足够信息后，必须输出以下固定Markdown格式的报告，不要调用更多工具：

## 故障摘要
（一段话描述故障现象和受影响的资源）

## 根因分析
（结合工具返回的具体数据，解释故障原因，引用关键错误信息）

## 修复建议
1. （具体操作步骤，优先给出kubectl命令或配置修改方案）
2. ...`
}

func (a *Agent) buildUserMessage(userQuery string, entities types.Entities) string {
	var parts []string
	parts = append(parts, userQuery)
	if len(entities.Namespace) > 0 {
		parts = append(parts, fmt.Sprintf("（目标命名空间：%s）", strings.Join(entities.Namespace, ", ")))
	}
	if entities.ResourceName != "" {
		rt := "resource"
		if len(entities.ResourceType) > 0 {
			rt = strings.Join(entities.ResourceType, "/")
		}
		parts = append(parts, fmt.Sprintf("（目标资源：%s %s）", rt, entities.ResourceName))
	}
	return strings.Join(parts, "\n\n")
}

func (a *Agent) registerTools() (*toolv2.Registry, error) {
	reg := toolv2.NewRegistry()
	if err := toolquery.RegisterQueryTools(reg, a.k8sClient); err != nil {
		return nil, fmt.Errorf("加载查询工具失败: %w", err)
	}
	if err := tooltroubleshooting.RegisterTools(reg, a.k8sClient); err != nil {
		return nil, fmt.Errorf("加载故障排查工具失败: %w", err)
	}
	return reg, nil
}

func (a *Agent) HandleQuery(ctx context.Context, userQuery string, entities types.Entities) (result string, err error) {
	start := time.Now()
	var inTokens, outTokens int
	a.emit(event.AgentStart{AgentName: "Troubleshooting Agent", QueryID: a.queryID})
	a.logger().Debug("handling troubleshooting query", zap.String("query", userQuery))
	defer func() {
		a.emit(event.AgentDone{
			QueryID:   a.queryID,
			Result:    result,
			Duration:  time.Since(start),
			InTokens:  inTokens,
			OutTokens: outTokens,
		})
	}()

	reg, err := a.registerTools()
	if err != nil {
		return "", err
	}
	allowedTools := reg.Names()
	messages := []llm.Message{
		{Role: "system", Content: a.buildSystemPrompt()},
		{Role: "user", Content: a.buildUserMessage(userQuery, entities)},
	}
	loopResult, err := loop.Run(ctx, loop.Config{
		QueryID:  a.queryID,
		LLM:      a.llmClient,
		Messages: messages,
		Tools:    reg.Definitions(allowedTools),
		Executor: toolv2.NewExecutor(reg),
		Policy: toolv2.ExecutePolicy{
			AllowedCapabilities: []toolv2.Capability{toolv2.CapabilityRead},
			AllowedTools:        allowedTools,
			MaxOutputBytes:      20_000,
			EmitEvents:          true,
		},
		MaxSteps:   a.maxSteps,
		Emit:       a.emit,
		Supervisor: supervisor.New(a.supervisorCfg, a.llmClient),
	})
	if err != nil {
		return "", fmt.Errorf("LLM调用失败: %w", err)
	}
	inTokens = loopResult.InTokens
	outTokens = loopResult.OutTokens
	return loopResult.Content, nil
}

package router

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/kubewise/kubewise/internal/config"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/diagnose"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/event"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/router/types"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/subagent/deploy"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/subagent/operation"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/subagent/query"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/subagent/security"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/subagent/troubleshooting"
	"github.com/kubewise/kubewise/internal/platform/agentruntime/supervisor"
	"github.com/kubewise/kubewise/internal/platform/cluster"
	"github.com/kubewise/kubewise/internal/utils/helm"
	"github.com/kubewise/kubewise/internal/utils/llm"
	"github.com/kubewise/kubewise/internal/utils/log"
	"go.uber.org/zap"
)

// Agent 路由Agent
type Agent struct {
	k8sClient      *cluster.Client
	clusterManager *cluster.ClusterClientManager
	llmClient      *llm.Client
	maxSteps       int
	supervisorCfg  supervisor.Config
	queryAgent            *query.Agent
	diagnoseAgent         *diagnose.Agent
	troubleshootingAgent  *troubleshooting.Agent
	securityAgent         *security.Agent
	operationAgent *operation.Agent
	deployAgent    *deploy.Agent
	helmClient     *helm.Client
	streamMu       sync.Mutex
}

type Config struct {
	K8sClient      *cluster.Client
	ClusterManager *cluster.ClusterClientManager
	LLMClient      *llm.Client
	MaxSteps       int
	SupervisorCfg  supervisor.Config
}

func (a *Agent) logger() *zap.Logger {
	return config.L()
}

// New 创建路由Agent
func New(cfg Config) (*Agent, error) {
	k8sClient := cfg.K8sClient
	clusterManager := cfg.ClusterManager
	llmClient := cfg.LLMClient
	maxSteps := cfg.MaxSteps
	supervisorCfg := cfg.SupervisorCfg
	if maxSteps <= 0 {
		maxSteps = 20
	}
	queryAgent, err := query.New(k8sClient, llmClient, query.WithMaxSteps(maxSteps), query.WithSupervisorConfig(supervisorCfg))
	if err != nil {
		return nil, fmt.Errorf("初始化查询Agent失败: %w", err)
	}
	diagnoseAgent := diagnose.NewAgent(k8sClient, llmClient)
	troubleshootingAgent, err := troubleshooting.New(k8sClient, llmClient, troubleshooting.WithMaxSteps(maxSteps), troubleshooting.WithSupervisorConfig(supervisorCfg))
	if err != nil {
		return nil, fmt.Errorf("初始化故障排查Agent失败: %w", err)
	}
	securityAgent, err := security.New(k8sClient, llmClient, security.WithMaxSteps(maxSteps), security.WithSupervisorConfig(supervisorCfg))
	if err != nil {
		return nil, fmt.Errorf("初始化安全审计Agent失败: %w", err)
	}
	operationAgent, err := operation.New(k8sClient, llmClient, operation.WithMaxSteps(maxSteps), operation.WithSupervisorConfig(supervisorCfg))
	if err != nil {
		return nil, fmt.Errorf("初始化操作Agent失败: %w", err)
	}
	helmClient := helm.New("")
	deployAgent := deploy.New(llmClient, helmClient, k8sClient)
	return &Agent{
		k8sClient:      k8sClient,
		clusterManager: clusterManager,
		llmClient:      llmClient,
		maxSteps:       maxSteps,
		supervisorCfg:  supervisorCfg,
		queryAgent:           queryAgent,
		diagnoseAgent:        diagnoseAgent,
		troubleshootingAgent: troubleshootingAgent,
		securityAgent:        securityAgent,
		operationAgent: operationAgent,
		deployAgent:    deployAgent,
		helmClient:     helmClient,
	}, nil
}

// HandleQuery 处理用户查询
func (a *Agent) HandleQuery(userQuery string) (string, error) {
	ctx := context.Background()
	clusterName, cleanQuery := parseClusterFromQuery(userQuery)
	k8sClient, err := a.k8sClientForCluster(ctx, clusterName)
	if err != nil {
		return "", fmt.Errorf("cluster %q: %w", clusterName, err)
	}
	agents, err := a.agentsForRequest(k8sClient)
	if err != nil {
		return "", err
	}

	log.Ctx(context.Background()).Info("agent handle query",
		zap.String("event", "agent.handle_query"),
		zap.String("cluster", clusterName),
	)

	// 1. 意图分类
	intent, err := a.classifyIntent(ctx, cleanQuery)
	if err != nil {
		a.logger().Error("intent classification failed", zap.Error(err))
		log.Ctx(context.Background()).Error("agent handle query failed",
			zap.String("event", "agent.error"),
			zap.Error(err),
		)
		return "", fmt.Errorf("意图分类失败: %w", err)
	}
	a.logger().Info("intent classified",
		zap.String("task_type", string(intent.TaskType)),
		zap.Float64("confidence", intent.Confidence),
	)

	if len(intent.Entities.Namespace) > 0 {
	}
	if intent.Entities.ResourceName != "" && len(intent.Entities.ResourceType) > 0 {
	}

	// 2. 路由到对应的Agent处理
	switch intent.TaskType {
	case types.TaskTypeQuery:
		return agents.query.HandleQuery(ctx, cleanQuery, intent.Entities)
	case types.TaskTypeOperation:
		return agents.operation.HandleQuery(ctx, cleanQuery, intent.Entities)
	case types.TaskTypeTroubleshooting:
		agents.troubleshooting.SetEventChannel(nil, "")
		return agents.troubleshooting.HandleQuery(ctx, cleanQuery, intent.Entities)
	case types.TaskTypeSecurity:
		return agents.security.HandleQuery(ctx, cleanQuery, intent.Entities)
	case types.TaskTypeDeploy:
		return agents.deploy.HandleQuery(ctx, cleanQuery, intent.Entities)
	default:
		log.Ctx(context.Background()).Error("agent handle query failed",
			zap.String("event", "agent.error"),
			zap.Error(fmt.Errorf("不支持的任务类型: %s", intent.TaskType)),
		)
		return "", fmt.Errorf("不支持的任务类型: %s", intent.TaskType)
	}
}

// HandleQueryStream classifies the query, creates fresh sub-agents with event
// channel support, routes to the appropriate sub-agent, and emits structured
// render events followed by StreamDoneEvent on success.
func (a *Agent) HandleQueryStream(ctx context.Context, userQuery, queryID string, eventCh chan<- event.Event) error {
	// Sub-agents are shared and mutate per-request event routing state.
	// Serialize streaming queries to prevent cross-request channel/queryID races.
	a.streamMu.Lock()
	defer a.streamMu.Unlock()

	se := event.NewEmitter(eventCh, queryID)
	emit := func(ev event.Event) {
		_ = se.Emit(ctx, ev)
	}

	clusterName, cleanQuery := parseClusterFromQuery(userQuery)
	k8sClient, err := a.k8sClientForCluster(ctx, clusterName)
	if err != nil {
		emit(event.StreamErr{QueryID: queryID, Err: fmt.Errorf("cluster %q: %w", clusterName, err)})
		return err
	}
	agents, err := a.agentsForRequest(k8sClient)
	if err != nil {
		emit(event.StreamErr{QueryID: queryID, Err: err})
		return err
	}
	if clusterName != "" {
		emit(event.Phase{QueryID: queryID, Phase: fmt.Sprintf("cluster: %s", clusterName)})
	}

	// 1. Classify intent.
	emit(event.Phase{QueryID: queryID, Phase: "classifying intent"})
	intent, err := a.classifyIntent(ctx, cleanQuery)
	if err != nil {
		a.logger().Error("intent classification failed", zap.Error(err))
		emit(event.StreamErr{QueryID: queryID, Err: err})
		return err
	}
	a.logger().Info("intent classified",
		zap.String("task_type", string(intent.TaskType)),
		zap.Float64("confidence", intent.Confidence),
	)

	log.Ctx(ctx).Info("routing to sub-agent",
		zap.String("event", "agent.routed"),
		zap.String("task_type", string(intent.TaskType)),
	)

	// 2. Route to the appropriate sub-agent (fresh instance with eventCh).
	phaseLabel := fmt.Sprintf("routing to %s agent", intent.TaskTypeDescription)
	emit(event.Phase{QueryID: queryID, Phase: phaseLabel})
	switch intent.TaskType {
	case types.TaskTypeQuery:
		agents.query.SetEventChannel(eventCh, queryID)
		_, err = agents.query.HandleQuery(ctx, cleanQuery, intent.Entities)

	case types.TaskTypeTroubleshooting:
		agents.troubleshooting.SetEventChannel(eventCh, queryID)
		_, err = agents.troubleshooting.HandleQuery(ctx, cleanQuery, intent.Entities)

	case types.TaskTypeSecurity:
		agents.security.SetEventChannel(eventCh, queryID)
		_, err = agents.security.HandleQuery(ctx, cleanQuery, intent.Entities)

	case types.TaskTypeDeploy:
		bridgeCtx, bridgeCancel := context.WithCancel(ctx)
		defer bridgeCancel()

		selectionHandler := &streamChartSelectionHandler{
			emitter:   se,
			queryID:   queryID,
			bridgeCtx: bridgeCtx,
		}
		confirmHandler := &streamDeployConfirmHandler{
			emitter:   se,
			queryID:   queryID,
			bridgeCtx: bridgeCtx,
		}

		agents.deploy.SetEventChannel(eventCh, queryID)
		agents.deploy.SetSelectionHandler(selectionHandler)
		agents.deploy.SetConfirmHandler(confirmHandler)
		_, err = agents.deploy.HandleQuery(ctx, cleanQuery, intent.Entities)

	case types.TaskTypeOperation:
		handler := operation.NewChannelConfirmationHandler()

		bridgeCtx, bridgeCancel := context.WithCancel(ctx)
		defer bridgeCancel()
		go func() {
			for {
				select {
				case req, ok := <-handler.Requests:
					if !ok {
						return
					}
					stepBytes, mErr := json.Marshal(req.Step)
					if mErr != nil {
						stepBytes = []byte("{}")
					}
					respCh := make(chan json.RawMessage, 1)
					if err := se.Emit(ctx, event.InteractionRequest{
						QueryID:    queryID,
						Kind:       event.KindOperationStep,
						Payload:    stepBytes,
						TotalSteps: req.TotalSteps,
						RespCh:     respCh,
					}); err != nil {
						return
					}
					select {
					case raw := <-respCh:
						var cr event.OperationConfirmResponse
						_ = json.Unmarshal(raw, &cr)
						select {
						case handler.Responses <- operation.ConfirmResponse{
							Confirmed:  cr.Confirmed,
							Correction: cr.Correction,
						}:
						case <-bridgeCtx.Done():
							return
						}
					case <-bridgeCtx.Done():
						return
					}
				case <-bridgeCtx.Done():
					return
				}
			}
		}()

		agents.operation.SetConfirmationHandler(handler)
		agents.operation.SetEventChannel(eventCh, queryID)
		_, err = agents.operation.HandleQuery(ctx, cleanQuery, intent.Entities)

	default:
		a.logger().Error("unsupported task type", zap.String("task_type", string(intent.TaskType)))
		err = fmt.Errorf("不支持的任务类型: %s", intent.TaskType)
	}

	if err != nil {
		log.Ctx(ctx).Error("agent stream query failed",
			zap.String("event", "agent.error"),
			zap.String("task_type", string(intent.TaskType)),
			zap.Error(err),
		)
		emit(event.StreamErr{QueryID: queryID, Err: err})
		return err
	}

	emit(event.StreamDone{QueryID: queryID})
	return nil
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

	resp, err := a.llmClient.ChatCompletion(ctx, messages, nil, nil)
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

	log.Ctx(ctx).Info("intent classified",
		zap.String("event", "agent.intent"),
		zap.String("task_type", string(intent.TaskType)),
		zap.Float64("confidence", intent.Confidence),
	)

	return &intent, nil
}

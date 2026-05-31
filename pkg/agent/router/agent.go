package router

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/kubewise/kubewise/pkg/agent/chat"
	"github.com/kubewise/kubewise/pkg/agent/deploy"
	"github.com/kubewise/kubewise/pkg/agent/operation"
	"github.com/kubewise/kubewise/pkg/agent/query"
	"github.com/kubewise/kubewise/pkg/agent/security"
	"github.com/kubewise/kubewise/pkg/agent/supervisor"
	"github.com/kubewise/kubewise/pkg/agent/troubleshooting"
	"github.com/kubewise/kubewise/pkg/helm"
	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/llm"
	"github.com/kubewise/kubewise/pkg/stream"
	"github.com/kubewise/kubewise/pkg/types"
	"go.uber.org/zap"
)

const chatFallbackConfidence = 0.60

// Agent 路由Agent
type Agent struct {
	k8sClient            *k8s.Client
	llmClient            *llm.Client
	maxSteps             int
	supervisorCfg        supervisor.Config
	queryAgent           *query.Agent
	troubleshootingAgent *troubleshooting.Agent
	securityAgent        *security.Agent
	operationAgent       *operation.Agent
	deployAgent          *deploy.Agent
	chatAgent            *chat.Agent
	helmClient           *helm.Client
	streamMu             sync.Mutex
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
	a.chatAgent.SetLogger(l)
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

func (a *Agent) normalizeIntent(intent *types.IntentClassification) *types.IntentClassification {
	if intent == nil {
		return intent
	}
	if intent.TaskType == types.TaskTypeChat || intent.Confidence >= chatFallbackConfidence {
		return intent
	}
	a.logger().Info("intent confidence below threshold, routing to chat",
		zap.String("original_task_type", string(intent.TaskType)),
		zap.Float64("confidence", intent.Confidence),
		zap.Float64("threshold", chatFallbackConfidence),
	)
	intent.TaskType = types.TaskTypeChat
	intent.TaskTypeDescription = "通用聊天"
	return intent
}

// New 创建路由Agent
func New(k8sClient *k8s.Client, llmClient *llm.Client, maxSteps int, supervisorCfg supervisor.Config) (*Agent, error) {
	if maxSteps <= 0 {
		maxSteps = 20
	}
	queryAgent, err := query.New(k8sClient, llmClient, query.WithMaxSteps(maxSteps), query.WithSupervisorConfig(supervisorCfg))
	if err != nil {
		return nil, fmt.Errorf("初始化查询Agent失败: %w", err)
	}
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
	chatAgent := chat.New(llmClient)
	return &Agent{
		k8sClient:            k8sClient,
		llmClient:            llmClient,
		maxSteps:             maxSteps,
		supervisorCfg:        supervisorCfg,
		queryAgent:           queryAgent,
		troubleshootingAgent: troubleshootingAgent,
		securityAgent:        securityAgent,
		operationAgent:       operationAgent,
		deployAgent:          deployAgent,
		chatAgent:            chatAgent,
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
	intent = a.normalizeIntent(intent)
	a.logger().Info("intent routed",
		zap.String("task_type", string(intent.TaskType)),
		zap.String("description", intent.TaskTypeDescription),
		zap.Float64("confidence", intent.Confidence),
	)

	if len(intent.Entities.Namespace) > 0 {
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
	case types.TaskTypeChat:
		return a.chatAgent.HandleQuery(ctx, userQuery, intent.Entities)
	default:
		return "", fmt.Errorf("不支持的任务类型: %s", intent.TaskType)
	}
}

// HandleQueryStream classifies the query, creates fresh sub-agents with event
// channel support, routes to the appropriate sub-agent, and emits structured
// render events followed by StreamDoneEvent on success.
func (a *Agent) HandleQueryStream(ctx context.Context, userQuery, queryID string, eventCh chan<- stream.Event) error {
	// Sub-agents are shared and mutate per-request event routing state.
	// Serialize streaming queries to prevent cross-request channel/queryID races.
	a.streamMu.Lock()
	defer a.streamMu.Unlock()

	se := stream.NewEmitter(eventCh, queryID)
	emit := func(ev stream.Event) {
		_ = se.Emit(ctx, ev)
	}

	// 1. Classify intent.
	emit(stream.Phase{QueryID: queryID, Phase: "classifying intent"})
	intent, err := a.classifyIntent(ctx, userQuery)
	if err != nil {
		a.logger().Error("intent classification failed", zap.Error(err))
		emit(stream.StreamErr{QueryID: queryID, Err: err})
		return err
	}
	a.logger().Info("intent classified",
		zap.String("task_type", string(intent.TaskType)),
		zap.Float64("confidence", intent.Confidence),
	)
	intent = a.normalizeIntent(intent)
	a.logger().Info("intent routed",
		zap.String("task_type", string(intent.TaskType)),
		zap.String("description", intent.TaskTypeDescription),
		zap.Float64("confidence", intent.Confidence),
	)

	var result string

	// 2. Route to the appropriate sub-agent (fresh instance with eventCh).
	phaseLabel := fmt.Sprintf("routing to %s agent", intent.TaskTypeDescription)
	emit(stream.Phase{QueryID: queryID, Phase: phaseLabel})
	switch intent.TaskType {
	case types.TaskTypeQuery:
		a.queryAgent.SetEventChannel(eventCh, queryID)
		a.queryAgent.SetLogger(a.log)
		result, err = a.queryAgent.HandleQuery(ctx, userQuery, intent.Entities)

	case types.TaskTypeTroubleshooting:
		a.troubleshootingAgent.SetEventChannel(eventCh, queryID)
		a.troubleshootingAgent.SetLogger(a.log)
		result, err = a.troubleshootingAgent.HandleQuery(ctx, userQuery, intent.Entities)

	case types.TaskTypeSecurity:
		a.securityAgent.SetEventChannel(eventCh, queryID)
		a.securityAgent.SetLogger(a.log)
		result, err = a.securityAgent.HandleQuery(ctx, userQuery, intent.Entities)

	case types.TaskTypeDeploy:
		// Bridge goroutine: 将 Deploy Agent 的同步 ConfirmHandler / SelectionHandler
		// 转换为通过 eventCh 发送 RequestEvent → TUI → 接收 DoneMsg → 解阻塞 Agent。
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

		a.deployAgent.SetEventChannel(eventCh, queryID)
		a.deployAgent.SetSelectionHandler(selectionHandler)
		a.deployAgent.SetConfirmHandler(confirmHandler)
		a.deployAgent.SetLogger(a.log)
		result, err = a.deployAgent.HandleQuery(ctx, userQuery, intent.Entities)

	case types.TaskTypeOperation:
		handler := operation.NewChannelConfirmationHandler()

		// Bridge goroutine: forwards InteractionRequest → operation responses.
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
					if err := se.Emit(ctx, stream.InteractionRequest{
						QueryID:    queryID,
						Kind:       stream.KindOperationStep,
						Payload:    stepBytes,
						TotalSteps: req.TotalSteps,
						RespCh:     respCh,
					}); err != nil {
						return
					}
					select {
					case raw := <-respCh:
						var cr stream.OperationConfirmResponse
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

		a.operationAgent.SetConfirmationHandler(handler)
		a.operationAgent.SetEventChannel(eventCh, queryID)
		a.operationAgent.SetLogger(a.log)
		result, err = a.operationAgent.HandleQuery(ctx, userQuery, intent.Entities)

	case types.TaskTypeChat:
		a.chatAgent.SetEventChannel(eventCh, queryID)
		a.chatAgent.SetLogger(a.log)
		result, err = a.chatAgent.HandleQuery(ctx, userQuery, intent.Entities)

	default:
		a.logger().Error("unsupported task type", zap.String("task_type", string(intent.TaskType)))
		err = fmt.Errorf("不支持的任务类型: %s", intent.TaskType)
	}

	if err != nil {
		emit(stream.StreamErr{QueryID: queryID, Err: err})
		return err
	}

	emit(stream.StreamDone{QueryID: queryID, Result: result})
	return nil
}

// classifyIntent 意图分类
func (a *Agent) classifyIntent(ctx context.Context, userQuery string) (*types.IntentClassification, error) {
	systemPrompt := `你是 KubeWise 的 Router Agent，负责将用户输入分类到一个最合适的子 Agent。

只能返回以下六种 task_type 之一：
1. operation：对已有 Kubernetes 资源执行写操作，例如扩容、缩容、重启、删除、apply、打标签、cordon/drain。
2. query：查询真实 Kubernetes 集群状态、资源列表、资源详情或统计信息，例如列出 namespace、查看 Pod、查询 Deployment。
3. troubleshooting：排查异常、失败、错误、CrashLoopBackOff、ImagePullBackOff、服务不可达、日志报错等问题。
4. security：安全审计、权限检查、RBAC、Pod 安全、NetworkPolicy、镜像安全、合规扫描。
5. deploy：安装、部署、升级或卸载完整应用或 Helm Chart，例如部署 Prometheus、ArgoCD、Nginx Ingress。
6. chat：普通聊天、寒暄、身份介绍、知识分享、概念解释、学习辅导，或者不需要读取/修改真实集群的问题。

分类规则：
- “你好”“你是谁”“你能做什么”这类寒暄或身份问题，分类为 chat。
- “Kubernetes 是什么”“Pod 和容器有什么区别”“讲讲 Helm”这类知识解释问题，分类为 chat。
- 只有用户明确要求读取真实集群状态时，才分类为 query。
- 只有用户明确要求修改真实集群资源时，才分类为 operation。
- 如果用户要安装一个完整应用或 Helm Chart，分类为 deploy，不要分类为 operation。
- 如果分类不确定，优先选择 chat，并降低 confidence。

只返回 JSON，不要返回任何解释性文字。格式如下：
{
  "task_type": "operation|query|troubleshooting|security|deploy|chat",
  "task_type_description": "任务类型中文描述",
  "entities": {
    "namespace": [],
    "resource_name": "",
    "resource_type": [],
    "app_name": "",
    "operation": ""
  },
  "confidence": 0.0
}`

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

	return &intent, nil
}

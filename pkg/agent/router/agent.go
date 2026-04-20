package router

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/llm"
	"github.com/kubewise/kubewise/pkg/types"
	"github.com/kubewise/kubewise/pkg/agent/query"
)

// Agent 路由Agent
type Agent struct {
	k8sClient  *k8s.Client
	llmClient  *llm.Client
	queryAgent *query.Agent
}

// New 创建路由Agent
func New(k8sClient *k8s.Client, llmClient *llm.Client) *Agent {
	return &Agent{
		k8sClient:  k8sClient,
		llmClient:  llmClient,
		queryAgent: query.New(k8sClient, llmClient),
	}
}

// HandleQuery 处理用户查询
func (a *Agent) HandleQuery(userQuery string) (string, error) {
	ctx := context.Background()

	// 1. 意图分类
	intent, err := a.classifyIntent(ctx, userQuery)
	if err != nil {
		return "", fmt.Errorf("意图分类失败: %w", err)
	}

	fmt.Printf("识别到任务类型：%s，置信度：%.2f\n", intent.TaskTypeDescription, intent.Confidence)
	if intent.Entities.Namespace != "" {
		fmt.Printf("目标命名空间：%s\n", intent.Entities.Namespace)
	}
	if intent.Entities.ResourceName != "" {
		fmt.Printf("目标资源：%s/%s\n", intent.Entities.ResourceType, intent.Entities.ResourceName)
	}

	// 2. 路由到对应的Agent处理
	switch intent.TaskType {
	case types.TaskTypeQuery:
		return a.queryAgent.HandleQuery(ctx, userQuery, intent.Entities)
	case types.TaskTypeOperation:
		return "操作类功能正在开发中，敬请期待", nil
	case types.TaskTypeTroubleshooting:
		return "故障排查功能正在开发中，敬请期待", nil
	case types.TaskTypeSecurity:
		return "安全审计功能正在开发中，敬请期待", nil
	default:
		return "", fmt.Errorf("不支持的任务类型: %s", intent.TaskType)
	}
}

// classifyIntent 意图分类
func (a *Agent) classifyIntent(ctx context.Context, userQuery string) (*types.IntentClassification, error) {
	systemPrompt := `你是Kubernetes智能运维系统的路由分析器，负责将用户的自然语言查询分类到以下四种任务类型之一：
1. operation（操作类）：用户需要执行创建、修改、删除、部署等主动操作
2. query（查询类）：用户需要查询集群的状态、信息、统计等
3. troubleshooting（故障排查类）：用户需要排查异常、错误、崩溃等问题
4. security（安全审计类）：用户需要进行安全检查、权限审计、合规扫描等

请分析用户查询，返回JSON格式的结果，包含：
- task_type: 任务类型枚举值（operation/query/troubleshooting/security）
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

	resp, err := a.llmClient.ChatCompletion(ctx, messages, nil)
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
		return nil, fmt.Errorf("解析意图结果失败: %w，原始内容: %s", err, content)
	}

	return &intent, nil
}

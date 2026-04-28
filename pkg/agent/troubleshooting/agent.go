package troubleshooting

import (
	"context"
	"fmt"
	"strings"

	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/llm"
	"github.com/kubewise/kubewise/pkg/tool"
	"github.com/kubewise/kubewise/pkg/types"

	// 加载查询工具和故障排查工具，触发init函数注册
	_ "github.com/kubewise/kubewise/pkg/tools/v1/query"
	_ "github.com/kubewise/kubewise/pkg/tools/v1/troubleshooting"
)

// Agent 故障排查Agent
type Agent struct {
	k8sClient    *k8s.Client
	llmClient    *llm.Client
	toolRegistry *tool.Registry
}

// New 创建故障排查Agent
func New(k8sClient *k8s.Client, llmClient *llm.Client) (*Agent, error) {
	toolDep := tool.ToolDependency{
		K8sClient: k8sClient,
	}
	registry, err := tool.LoadGlobalRegistry(toolDep)
	if err != nil {
		return nil, fmt.Errorf("加载工具注册中心失败: %w", err)
	}
	return &Agent{
		k8sClient:    k8sClient,
		llmClient:    llmClient,
		toolRegistry: registry,
	}, nil
}

// buildSystemPrompt 生成系统提示词
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

// HandleQuery 处理故障排查请求
func (a *Agent) HandleQuery(ctx context.Context, userQuery string, entities types.Entities) (string, error) {
	functions := a.toolRegistry.GetAllFunctionDefinitions()
	messages := []llm.Message{
		{Role: "system", Content: a.buildSystemPrompt()},
		{Role: "user", Content: userQuery},
	}

	maxSteps := 10
	for step := range maxSteps {
		resp, err := a.llmClient.ChatCompletion(ctx, messages, functions)
		if err != nil {
			return "", fmt.Errorf("LLM调用失败: %w", err)
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
		result, err := t.Execute(ctx, funcCall.Arguments)
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

	return "", fmt.Errorf("超过最大调用轮次，无法完成故障排查")
}

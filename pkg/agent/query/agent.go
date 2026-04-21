package query

import (
	"context"
	"fmt"
	"strings"

	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/llm"
	"github.com/kubewise/kubewise/pkg/tool"
	"github.com/kubewise/kubewise/pkg/types"

	// 导入所有工具包，触发init函数注册工具
	_ "github.com/kubewise/kubewise/pkg/tools/v1/query"
)

// Agent 查询Agent
type Agent struct {
	k8sClient    *k8s.Client
	llmClient    *llm.Client
	toolRegistry *tool.Registry
}

// New 创建查询Agent
func New(k8sClient *k8s.Client, llmClient *llm.Client) (*Agent, error) {
	// 加载工具注册中心（必须成功，否则无法工作）
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
如果已经获取到足够的信息，请直接用自然语言回答用户的问题，不要调用不必要的工具。`
}

// HandleQuery 处理查询请求
func (a *Agent) HandleQuery(ctx context.Context, userQuery string, entities types.Entities) (string, error) {
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

	// 最多允许5轮工具调用
	maxSteps := 10
	for step := range maxSteps {
		// 调用LLM
		resp, err := a.llmClient.ChatCompletion(ctx, messages, functions)
		if err != nil {
			return "", fmt.Errorf("LLM调用失败: %w", err)
		}

		// 检查是否有工具调用（使用SDK原生解析的结果）
		if len(resp.ToolCalls) == 0 {
			// 不是工具调用，直接返回内容
			return resp.Content, nil
		}

		var funcCall *llm.FunctionCall

		if len(resp.ToolCalls) > 0 {
			funcCall = &resp.ToolCalls[0].Function
		}

		if funcCall == nil {
			return "", fmt.Errorf("工具调用格式错误")
		}

		fmt.Printf("第%d步：调用工具 %s\n", step+1, funcCall.Name)

		if len(funcCall.Arguments) > 0 {
			args := make([]string, 0, len(funcCall.Arguments))
			for k, v := range funcCall.Arguments {
				args = append(args, fmt.Sprintf("%s=%v", k, v))
			}
			fmt.Printf("参数：%s\n", strings.Join(args, ", "))
		}

		// 执行工具调用（注册中心已确保存在）
		tool, exists := a.toolRegistry.GetTool(funcCall.Name)
		if !exists {
			return "", fmt.Errorf("未知工具: %s", funcCall.Name)
		}
		result, err := tool.Execute(ctx, funcCall.Arguments)

		// 处理工具调用错误，将错误信息返回给LLM让其修复
		if err != nil {
			fmt.Printf("工具调用失败：%v\n", err)
			result = fmt.Sprintf("工具调用失败：%v\n请修正参数后重新调用工具。", err)
		} else {
			fmt.Printf("工具返回结果长度：%d 字节\n", len(result))
		}

		// 将工具调用结果（成功或失败）添加到消息历史
		messages = append(messages, *resp)

		// 构造工具返回消息，使用标准的tool角色
		toolMsg := llm.Message{
			Role:    "tool",
			Content: fmt.Sprintf("工具返回结果：\n%s", result),
		}

		// 设置tool_call_id
		if len(resp.ToolCalls) > 0 {
			toolMsg.ToolCallID = resp.ToolCalls[0].ID
		}

		messages = append(messages, toolMsg)
	}

	return "", fmt.Errorf("超过最大调用轮次，无法完成查询")
}

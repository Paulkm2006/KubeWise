package query

import (
	"context"
	"fmt"
	"strings"

	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/tool"
)

// GetPodResourceUsageTool 获取Pod资源使用情况工具
type GetPodResourceUsageTool struct {
	k8sClient *k8s.Client
}

// NewGetPodResourceUsageTool 创建获取Pod资源工具实例
func NewGetPodResourceUsageTool(k8sClient *k8s.Client) *GetPodResourceUsageTool {
	return &GetPodResourceUsageTool{k8sClient: k8sClient}
}

// Name 返回工具唯一标识
func (t *GetPodResourceUsageTool) Name() string {
	return "get_pod_resource_usage"
}

// Description 返回工具功能描述
func (t *GetPodResourceUsageTool) Description() string {
	return "获取指定Pod的资源配置情况（请求和限制）"
}

// Parameters 返回工具参数定义（JSON Schema格式）
func (t *GetPodResourceUsageTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"podName": map[string]any{
				"type":        "string",
				"description": "Pod名称",
			},
			"namespace": map[string]any{
				"type":        "string",
				"description": "Pod所在的命名空间",
			},
		},
		"required": []string{"podName", "namespace"},
	}
}

// Execute 执行工具调用
func (t *GetPodResourceUsageTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	podName, ok := args["podName"].(string)
	if !ok || podName == "" {
		return "", fmt.Errorf("参数podName不能为空")
	}

	namespace, ok := args["namespace"].(string)
	if !ok || namespace == "" {
		return "", fmt.Errorf("参数namespace不能为空")
	}

	pod, err := t.k8sClient.GetPod(ctx, namespace, podName)
	if err != nil {
		return "", fmt.Errorf("获取Pod信息失败: %w", err)
	}

	var result strings.Builder
	result.WriteString(fmt.Sprintf("Pod %s/%s 的资源配置:\n", namespace, podName))

	for _, container := range pod.Spec.Containers {
		result.WriteString(fmt.Sprintf("\n容器: %s\n", container.Name))
		if container.Resources.Requests != nil {
			result.WriteString(fmt.Sprintf("  请求CPU: %s\n", container.Resources.Requests.Cpu().String()))
			result.WriteString(fmt.Sprintf("  请求内存: %s\n", container.Resources.Requests.Memory().String()))
		}
		if container.Resources.Limits != nil {
			result.WriteString(fmt.Sprintf("  限制CPU: %s\n", container.Resources.Limits.Cpu().String()))
			result.WriteString(fmt.Sprintf("  限制内存: %s\n", container.Resources.Limits.Memory().String()))
		}
	}

	return result.String(), nil
}

// 注册工具到全局注册中心
func init() {
	tool.RegisterGlobal(tool.ToolMetadata{
		Name:        "get_pod_resource_usage",
		Description: "获取指定Pod的资源配置情况（请求和限制）",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"podName": map[string]any{
					"type":        "string",
					"description": "Pod名称",
				},
				"namespace": map[string]any{
					"type":        "string",
					"description": "Pod所在的命名空间",
				},
			},
			"required": []string{"podName", "namespace"},
		},
		Factory: func(dep any) (tool.Tool, error) {
			toolDep, ok := dep.(tool.ToolDependency)
			if !ok {
				return nil, fmt.Errorf("invalid dependency type")
			}
			return NewGetPodResourceUsageTool(toolDep.K8sClient), nil
		},
	})
}

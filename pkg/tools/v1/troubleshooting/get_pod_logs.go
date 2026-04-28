package troubleshooting

import (
	"context"
	"fmt"

	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/tool"
)

// GetPodLogsTool 获取Pod日志工具
type GetPodLogsTool struct {
	k8sClient *k8s.Client
}

// NewGetPodLogsTool 创建获取Pod日志工具实例
func NewGetPodLogsTool(k8sClient *k8s.Client) *GetPodLogsTool {
	return &GetPodLogsTool{k8sClient: k8sClient}
}

// Name 返回工具唯一标识
func (t *GetPodLogsTool) Name() string { return "get_pod_logs" }

// Description 返回工具功能描述
func (t *GetPodLogsTool) Description() string {
	return "获取Pod中指定容器的日志，用于分析崩溃原因、错误信息等"
}

// Parameters 返回工具参数定义（JSON Schema格式）
func (t *GetPodLogsTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"namespace": map[string]any{
				"type":        "string",
				"description": "Pod所在的命名空间",
			},
			"podName": map[string]any{
				"type":        "string",
				"description": "Pod名称",
			},
			"container": map[string]any{
				"type":        "string",
				"description": "容器名称，可选，不指定则使用第一个容器",
			},
			"tailLines": map[string]any{
				"type":        "integer",
				"description": "返回最后N行日志，默认100行",
			},
		},
		"required": []string{"namespace", "podName"},
	}
}

// Execute 执行工具调用
func (t *GetPodLogsTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	namespace, ok := args["namespace"].(string)
	if !ok || namespace == "" {
		return "", fmt.Errorf("参数namespace不能为空")
	}
	podName, ok := args["podName"].(string)
	if !ok || podName == "" {
		return "", fmt.Errorf("参数podName不能为空")
	}

	container, _ := args["container"].(string)

	var tailLines int64
	switch v := args["tailLines"].(type) {
	case float64:
		tailLines = int64(v)
	case int64:
		tailLines = v
	case int:
		tailLines = int64(v)
	}

	logs, err := t.k8sClient.GetPodLogs(ctx, namespace, podName, container, tailLines)
	if err != nil {
		return "", fmt.Errorf("获取Pod日志失败: %w", err)
	}

	containerLabel := container
	if containerLabel == "" {
		containerLabel = "默认容器"
	}
	return fmt.Sprintf("Pod %s/%s 的日志 (容器: %s):\n%s", namespace, podName, containerLabel, logs), nil
}

// 注册工具到全局注册中心
func init() {
	tool.RegisterGlobal(tool.ToolMetadata{
		Name:        "get_pod_logs",
		Description: "获取Pod中指定容器的日志，用于分析崩溃原因、错误信息等",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"namespace": map[string]any{
					"type":        "string",
					"description": "Pod所在的命名空间",
				},
				"podName": map[string]any{
					"type":        "string",
					"description": "Pod名称",
				},
				"container": map[string]any{
					"type":        "string",
					"description": "容器名称，可选，不指定则使用第一个容器",
				},
				"tailLines": map[string]any{
					"type":        "integer",
					"description": "返回最后N行日志，默认100行",
				},
			},
			"required": []string{"namespace", "podName"},
		},
		Factory: func(dep any) (tool.Tool, error) {
			toolDep, ok := dep.(tool.ToolDependency)
			if !ok {
				return nil, fmt.Errorf("invalid dependency type")
			}
			return NewGetPodLogsTool(toolDep.K8sClient), nil
		},
	})
}

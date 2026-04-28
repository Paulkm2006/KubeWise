package troubleshooting

import (
	"context"
	"fmt"

	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/tool"
)

type GetPodLogsTool struct {
	k8sClient *k8s.Client
}

func NewGetPodLogsTool(k8sClient *k8s.Client) *GetPodLogsTool {
	return &GetPodLogsTool{k8sClient: k8sClient}
}

func (t *GetPodLogsTool) Name() string { return "get_pod_logs" }

func (t *GetPodLogsTool) Description() string {
	return "获取Pod中指定容器的日志，用于分析崩溃原因、错误信息等"
}

func (t *GetPodLogsTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"namespace": map[string]any{
				"type":        "string",
				"description": "Pod所在的命名空间",
			},
			"pod_name": map[string]any{
				"type":        "string",
				"description": "Pod名称",
			},
			"container": map[string]any{
				"type":        "string",
				"description": "容器名称，可选，不指定则使用第一个容器",
			},
			"tail_lines": map[string]any{
				"type":        "integer",
				"description": "返回最后N行日志，默认100行",
			},
		},
		"required": []string{"namespace", "pod_name"},
	}
}

func (t *GetPodLogsTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	namespace, ok := args["namespace"].(string)
	if !ok || namespace == "" {
		return "", fmt.Errorf("参数namespace不能为空")
	}
	podName, ok := args["pod_name"].(string)
	if !ok || podName == "" {
		return "", fmt.Errorf("参数pod_name不能为空")
	}

	container, _ := args["container"].(string)

	var tailLines int64
	switch v := args["tail_lines"].(type) {
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

	return fmt.Sprintf("Pod %s/%s 的日志 (容器: %s):\n%s", namespace, podName, container, logs), nil
}

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
				"pod_name": map[string]any{
					"type":        "string",
					"description": "Pod名称",
				},
				"container": map[string]any{
					"type":        "string",
					"description": "容器名称，可选，不指定则使用第一个容器",
				},
				"tail_lines": map[string]any{
					"type":        "integer",
					"description": "返回最后N行日志，默认100行",
				},
			},
			"required": []string{"namespace", "pod_name"},
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

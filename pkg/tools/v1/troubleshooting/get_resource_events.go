package troubleshooting

import (
	"context"
	"fmt"
	"strings"

	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/tool"
)

type GetResourceEventsTool struct {
	k8sClient *k8s.Client
}

func NewGetResourceEventsTool(k8sClient *k8s.Client) *GetResourceEventsTool {
	return &GetResourceEventsTool{k8sClient: k8sClient}
}

func (t *GetResourceEventsTool) Name() string { return "get_resource_events" }

func (t *GetResourceEventsTool) Description() string {
	return "获取指定Kubernetes资源的事件列表，适用于Pod、PVC、IngressRoute等任意资源类型"
}

func (t *GetResourceEventsTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"namespace": map[string]any{
				"type":        "string",
				"description": "资源所在的命名空间",
			},
			"resource_name": map[string]any{
				"type":        "string",
				"description": "资源名称",
			},
		},
		"required": []string{"namespace", "resource_name"},
	}
}

func (t *GetResourceEventsTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	namespace, ok := args["namespace"].(string)
	if !ok || namespace == "" {
		return "", fmt.Errorf("参数namespace不能为空")
	}
	resourceName, ok := args["resource_name"].(string)
	if !ok || resourceName == "" {
		return "", fmt.Errorf("参数resource_name不能为空")
	}

	events, err := t.k8sClient.GetEvents(ctx, namespace, resourceName)
	if err != nil {
		return "", fmt.Errorf("获取事件失败: %w", err)
	}

	if len(events) == 0 {
		return fmt.Sprintf("资源 %s/%s 没有相关事件", namespace, resourceName), nil
	}

	var result strings.Builder
	result.WriteString(fmt.Sprintf("资源 %s/%s 的事件列表:\n", namespace, resourceName))
	result.WriteString("时间\t类型\t原因\t消息\n")
	result.WriteString("----------------------------------------\n")

	for _, e := range events {
		ts := e.LastTimestamp.Format("2006-01-02 15:04:05")
		if e.LastTimestamp.IsZero() {
			ts = e.EventTime.Format("2006-01-02 15:04:05")
		}
		result.WriteString(fmt.Sprintf("%s\t%s\t%s\t%s\n", ts, e.Type, e.Reason, e.Message))
	}

	result.WriteString(fmt.Sprintf("\n总计: %d条事件", len(events)))
	return result.String(), nil
}

func init() {
	tool.RegisterGlobal(tool.ToolMetadata{
		Name:        "get_resource_events",
		Description: "获取指定Kubernetes资源的事件列表，适用于Pod、PVC、IngressRoute等任意资源类型",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"namespace": map[string]any{
					"type":        "string",
					"description": "资源所在的命名空间",
				},
				"resource_name": map[string]any{
					"type":        "string",
					"description": "资源名称",
				},
			},
			"required": []string{"namespace", "resource_name"},
		},
		Factory: func(dep any) (tool.Tool, error) {
			toolDep, ok := dep.(tool.ToolDependency)
			if !ok {
				return nil, fmt.Errorf("invalid dependency type")
			}
			return NewGetResourceEventsTool(toolDep.K8sClient), nil
		},
	})
}

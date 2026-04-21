package query

import (
	"context"
	"fmt"
	"strings"

	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/tool"
)

// ListConfigMapsInNamespaceTool 列出指定命名空间下的ConfigMap工具
type ListConfigMapsInNamespaceTool struct {
	k8sClient *k8s.Client
}

// NewListConfigMapsInNamespaceTool 创建列出ConfigMap工具实例
func NewListConfigMapsInNamespaceTool(k8sClient *k8s.Client) *ListConfigMapsInNamespaceTool {
	return &ListConfigMapsInNamespaceTool{k8sClient: k8sClient}
}

// Name 返回工具唯一标识
func (t *ListConfigMapsInNamespaceTool) Name() string {
	return "list_configmaps_in_namespace"
}

// Description 返回工具功能描述
func (t *ListConfigMapsInNamespaceTool) Description() string {
	return "列出指定命名空间下的所有ConfigMap，不指定命名空间则列出所有命名空间的ConfigMap"
}

// Parameters 返回工具参数定义（JSON Schema格式）
func (t *ListConfigMapsInNamespaceTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"namespace": map[string]any{
				"type":        "string",
				"description": "命名空间，可选，不指定则列出所有命名空间的ConfigMap",
			},
		},
	}
}

// Execute 执行工具调用
func (t *ListConfigMapsInNamespaceTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	namespace := ""
	if ns, ok := args["namespace"].(string); ok {
		namespace = ns
	}

	configMaps, err := t.k8sClient.ListConfigMaps(ctx, namespace)
	if err != nil {
		return "", fmt.Errorf("获取ConfigMap列表失败: %w", err)
	}

	var result strings.Builder
	if namespace == "" {
		result.WriteString("所有命名空间的ConfigMap列表:\n")
	} else {
		result.WriteString(fmt.Sprintf("命名空间 %s 的ConfigMap列表:\n", namespace))
	}
	result.WriteString("命名空间\t名称\t数据项数\t创建时间\n")
	result.WriteString("--------------------------------------------------------\n")

	for _, cm := range configMaps {
		result.WriteString(fmt.Sprintf("%s\t%s\t%d\t%s\n",
			cm.Namespace, cm.Name, len(cm.Data), cm.CreationTimestamp.Format("2006-01-02 15:04:05")))
	}

	result.WriteString(fmt.Sprintf("\n总计: %d个ConfigMap", len(configMaps)))
	return result.String(), nil
}

// 注册工具到全局注册中心
func init() {
	tool.RegisterGlobal(tool.ToolMetadata{
		Name:        "list_configmaps_in_namespace",
		Description: "列出指定命名空间下的所有ConfigMap，不指定命名空间则列出所有命名空间的ConfigMap",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"namespace": map[string]any{
					"type":        "string",
					"description": "命名空间，可选，不指定则列出所有命名空间的ConfigMap",
				},
			},
		},
		Factory: func(dep any) (tool.Tool, error) {
			toolDep, ok := dep.(tool.ToolDependency)
			if !ok {
				return nil, fmt.Errorf("invalid dependency type")
			}
			return NewListConfigMapsInNamespaceTool(toolDep.K8sClient), nil
		},
	})
}

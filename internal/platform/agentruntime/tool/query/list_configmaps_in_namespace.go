package query

import (
	"context"
	"fmt"
	"strings"

	toolv2 "github.com/kubewise/kubewise/internal/platform/agentruntime/tool/v2"
	"github.com/kubewise/kubewise/internal/platform/cluster"
)

// ListConfigMapsInNamespaceTool 列出指定命名空间下的ConfigMap工具
type ListConfigMapsInNamespaceTool struct {
	k8sClient *cluster.Client
}

// NewListConfigMapsInNamespaceTool 创建列出ConfigMap工具实例
func NewListConfigMapsInNamespaceTool(k8sClient *cluster.Client) *ListConfigMapsInNamespaceTool {
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

func (t *ListConfigMapsInNamespaceTool) Meta() toolv2.ToolMeta {
	return toolv2.TextMeta(t.Name(), "v1", t.Description(), t.Parameters(), toolv2.CapabilityRead, toolv2.RiskNone, toolv2.ConfirmNever, "query")
}

func (t *ListConfigMapsInNamespaceTool) Execute(ctx context.Context, args map[string]any) (toolv2.ToolResult, error) {
	meta := t.Meta()
	text, err := t.executeText(ctx, args)
	if err != nil {
		return toolv2.ToolResult{}, err
	}
	return toolv2.TextResult(meta, text), nil
}

func (t *ListConfigMapsInNamespaceTool) executeText(ctx context.Context, args map[string]any) (string, error) {
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

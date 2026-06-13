package query

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"

	toolv2 "github.com/kubewise/kubewise/internal/platform/agentruntime/tool/v2"
	"github.com/kubewise/kubewise/internal/platform/cluster"
)

// ListCustomResourcesByGvrTool 根据GVR列出自定义资源工具
type ListCustomResourcesByGvrTool struct {
	k8sClient *cluster.Client
}

// NewListCustomResourcesByGvrTool 创建列出自定义资源工具实例
func NewListCustomResourcesByGvrTool(k8sClient *cluster.Client) *ListCustomResourcesByGvrTool {
	return &ListCustomResourcesByGvrTool{k8sClient: k8sClient}
}

// Name 返回工具唯一标识
func (t *ListCustomResourcesByGvrTool) Name() string {
	return "list_custom_resources_by_gvr"
}

// Description 返回工具功能描述
func (t *ListCustomResourcesByGvrTool) Description() string {
	return "根据GVR（Group/Version/Resource）列出指定类型的自定义资源"
}

// Parameters 返回工具参数定义（JSON Schema格式）
func (t *ListCustomResourcesByGvrTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"group": map[string]any{
				"type":        "string",
				"description": "自定义资源的API组",
			},
			"version": map[string]any{
				"type":        "string",
				"description": "自定义资源的API版本",
			},
			"resource": map[string]any{
				"type":        "string",
				"description": "自定义资源的资源类型名称（复数形式）",
			},
			"namespace": map[string]any{
				"type":        "string",
				"description": "命名空间，可选，不指定则列出所有命名空间的资源",
			},
		},
		"required": []string{"group", "version", "resource"},
	}
}

func (t *ListCustomResourcesByGvrTool) Meta() toolv2.ToolMeta {
	return toolv2.TextMeta(t.Name(), "v1", t.Description(), t.Parameters(), toolv2.CapabilityRead, toolv2.RiskNone, toolv2.ConfirmNever, "query")
}

func (t *ListCustomResourcesByGvrTool) Execute(ctx context.Context, args map[string]any) (toolv2.ToolResult, error) {
	meta := t.Meta()
	text, err := t.executeText(ctx, args)
	if err != nil {
		return toolv2.ToolResult{}, err
	}
	return toolv2.TextResult(meta, text), nil
}

func (t *ListCustomResourcesByGvrTool) executeText(ctx context.Context, args map[string]any) (string, error) {
	group, ok := args["group"].(string)
	if !ok {
		return "", fmt.Errorf("参数group必须为字符串")
	}

	version, ok := args["version"].(string)
	if !ok || version == "" {
		return "", fmt.Errorf("参数version不能为空")
	}

	resource, ok := args["resource"].(string)
	if !ok || resource == "" {
		return "", fmt.Errorf("参数resource不能为空")
	}

	namespace := ""
	if ns, ok := args["namespace"].(string); ok {
		namespace = ns
	}

	gvr := schema.GroupVersionResource{
		Group:    group,
		Version:  version,
		Resource: resource,
	}

	customResources, err := t.k8sClient.ListCustomResources(ctx, gvr, namespace)
	if err != nil {
		return "", fmt.Errorf("获取自定义资源列表失败: %w", err)
	}

	var result strings.Builder
	if namespace == "" {
		if group == "" {
			result.WriteString(fmt.Sprintf("所有命名空间的 %s.%s 资源列表:\n", resource, version))
		} else {
			result.WriteString(fmt.Sprintf("所有命名空间的 %s.%s.%s 自定义资源列表:\n", resource, version, group))
		}
	} else {
		if group == "" {
			result.WriteString(fmt.Sprintf("命名空间 %s 的 %s.%s 资源列表:\n", namespace, resource, version))
		} else {
			result.WriteString(fmt.Sprintf("命名空间 %s 的 %s.%s.%s 自定义资源列表:\n", namespace, resource, version, group))
		}
	}

	result.WriteString("名称\t命名空间\t创建时间\n")
	result.WriteString("----------------------------------------\n")

	for _, cr := range customResources {
		crObj, ok := cr.(map[string]interface{})
		if !ok {
			continue
		}

		metadata, ok := crObj["metadata"].(map[string]interface{})
		if !ok {
			continue
		}

		name := mapString(metadata, "name")
		ns := mapString(metadata, "namespace")
		creationTimestamp := mapString(metadata, "creationTimestamp")

		result.WriteString(fmt.Sprintf("%s\t%s\t%s\n", name, ns, creationTimestamp))
	}

	result.WriteString(fmt.Sprintf("\n总计: %d个自定义资源", len(customResources)))
	return result.String(), nil
}

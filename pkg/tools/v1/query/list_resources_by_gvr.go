package query

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/tool"
)

// ListResourcesByGvrTool 根据GVR列出任意类型资源工具
type ListResourcesByGvrTool struct {
	k8sClient *k8s.Client
}

// NewListResourcesByGvrTool 创建根据GVR列出资源工具实例
func NewListResourcesByGvrTool(k8sClient *k8s.Client) *ListResourcesByGvrTool {
	return &ListResourcesByGvrTool{k8sClient: k8sClient}
}

// Name 返回工具唯一标识
func (t *ListResourcesByGvrTool) Name() string {
	return "list_resources_by_gvr"
}

// Description 返回工具功能描述
func (t *ListResourcesByGvrTool) Description() string {
	return "根据GVR（Group/Version/Resource）列出任意类型的Kubernetes资源，支持内置资源和自定义资源"
}

// Parameters 返回工具参数定义（JSON Schema格式）
func (t *ListResourcesByGvrTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"group": map[string]any{
				"type":        "string",
				"description": "资源的API组，例如：\"\"（核心API组）、\"apps\"、\"batch\"、\"mongodbcommunity.mongodb.com\"等",
			},
			"version": map[string]any{
				"type":        "string",
				"description": "资源的API版本，例如：\"v1\"、\"v1beta1\"等",
			},
			"resource": map[string]any{
				"type":        "string",
				"description": "资源类型的复数名称，例如：\"pods\"、\"services\"、\"deployments\"、\"jobs\"等",
			},
			"namespace": map[string]any{
				"type":        "string",
				"description": "命名空间，可选，不指定则列出所有命名空间的资源",
			},
		},
		"required": []string{"group", "version", "resource"},
	}
}

// Execute 执行工具调用
func (t *ListResourcesByGvrTool) Execute(ctx context.Context, args map[string]any) (string, error) {
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

	resources, err := t.k8sClient.ListCustomResources(ctx, gvr, namespace)
	if err != nil {
		return "", fmt.Errorf("获取资源列表失败: %w", err)
	}

	var result strings.Builder
	if namespace == "" {
		if group == "" {
			result.WriteString(fmt.Sprintf("所有命名空间的 %s.%s 资源列表:\n", resource, version))
		} else {
			result.WriteString(fmt.Sprintf("所有命名空间的 %s.%s.%s 资源列表:\n", resource, version, group))
		}
	} else {
		if group == "" {
			result.WriteString(fmt.Sprintf("命名空间 %s 的 %s.%s 资源列表:\n", namespace, resource, version))
		} else {
			result.WriteString(fmt.Sprintf("命名空间 %s 的 %s.%s.%s 资源列表:\n", namespace, resource, version, group))
		}
	}

	result.WriteString("名称\t命名空间\t创建时间\n")
	result.WriteString("----------------------------------------\n")

	for _, res := range resources {
		resObj, ok := res.(map[string]interface{})
		if !ok {
			continue
		}

		metadata, ok := resObj["metadata"].(map[string]interface{})
		if !ok {
			continue
		}

		name := metadata["name"].(string)
		ns, _ := metadata["namespace"].(string)
		creationTimestamp := metadata["creationTimestamp"].(string)

		result.WriteString(fmt.Sprintf("%s\t%s\t%s\n", name, ns, creationTimestamp))
	}

	result.WriteString(fmt.Sprintf("\n总计: %d个资源", len(resources)))
	return result.String(), nil
}

// 注册工具到全局注册中心
func init() {
	tool.RegisterGlobal(tool.ToolMetadata{
		Name:        "list_resources_by_gvr",
		Description: "根据GVR（Group/Version/Resource）列出任意类型的Kubernetes资源，支持内置资源和自定义资源",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"group": map[string]any{
					"type":        "string",
					"description": "资源的API组，例如：\"\"（核心API组）、\"apps\"、\"batch\"、\"mongodbcommunity.mongodb.com\"等",
				},
				"version": map[string]any{
					"type":        "string",
					"description": "资源的API版本，例如：\"v1\"、\"v1beta1\"等",
				},
				"resource": map[string]any{
					"type":        "string",
					"description": "资源类型的复数名称，例如：\"pods\"、\"services\"、\"deployments\"、\"jobs\"等",
				},
				"namespace": map[string]any{
					"type":        "string",
					"description": "命名空间，可选，不指定则列出所有命名空间的资源",
				},
			},
			"required": []string{"group", "version", "resource"},
		},
		Factory: func(dep any) (tool.Tool, error) {
			toolDep, ok := dep.(tool.ToolDependency)
			if !ok {
				return nil, fmt.Errorf("invalid dependency type")
			}
			return NewListResourcesByGvrTool(toolDep.K8sClient), nil
		},
	})
}

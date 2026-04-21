package query

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/tool"
)

// GetCustomResourceByGvrAndNameTool 获取指定自定义资源详情工具
type GetCustomResourceByGvrAndNameTool struct {
	k8sClient *k8s.Client
}

// NewGetCustomResourceByGvrAndNameTool 创建获取自定义资源详情工具实例
func NewGetCustomResourceByGvrAndNameTool(k8sClient *k8s.Client) *GetCustomResourceByGvrAndNameTool {
	return &GetCustomResourceByGvrAndNameTool{k8sClient: k8sClient}
}

// Name 返回工具唯一标识
func (t *GetCustomResourceByGvrAndNameTool) Name() string {
	return "get_custom_resource_by_gvr_and_name"
}

// Description 返回工具功能描述
func (t *GetCustomResourceByGvrAndNameTool) Description() string {
	return "根据GVR（Group/Version/Resource）和名称获取指定自定义资源的详细内容"
}

// Parameters 返回工具参数定义（JSON Schema格式）
func (t *GetCustomResourceByGvrAndNameTool) Parameters() map[string]any {
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
			"name": map[string]any{
				"type":        "string",
				"description": "自定义资源的名称，使用metadata.name字段的值",
			},
			"namespace": map[string]any{
				"type":        "string",
				"description": "命名空间，集群级资源不需要提供，命名空间级资源必须提供",
			},
		},
		"required": []string{"group", "version", "resource", "name"},
	}
}

// Execute 执行工具调用
func (t *GetCustomResourceByGvrAndNameTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	group, ok := args["group"].(string)
	if !ok || group == "" {
		return "", fmt.Errorf("参数group不能为空")
	}

	version, ok := args["version"].(string)
	if !ok || version == "" {
		return "", fmt.Errorf("参数version不能为空")
	}

	resource, ok := args["resource"].(string)
	if !ok || resource == "" {
		return "", fmt.Errorf("参数resource不能为空")
	}

	name, ok := args["name"].(string)
	if !ok || name == "" {
		return "", fmt.Errorf("参数name不能为空")
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

	cr, err := t.k8sClient.GetCustomResource(ctx, gvr, namespace, name)
	if err != nil {
		return "", fmt.Errorf("获取自定义资源详情失败: %w", err)
	}

	// 格式化输出
	var result strings.Builder
	if namespace == "" {
		fmt.Fprintf(&result, "集群级自定义资源 %s/%s/%s/%s 的详情:\n", group, version, resource, name)
	} else {
		fmt.Fprintf(&result, "命名空间 %s 下的自定义资源 %s/%s/%s/%s 的详情:\n", namespace, group, version, resource, name)
	}

	// 转换为格式化JSON
	jsonBytes, err := json.MarshalIndent(cr, "", "  ")
	if err != nil {
		return "", fmt.Errorf("格式化资源内容失败: %w", err)
	}

	result.WriteString(string(jsonBytes))
	return result.String(), nil
}

// 注册工具到全局注册中心
func init() {
	tool.RegisterGlobal(tool.ToolMetadata{
		Name:        "get_custom_resource_by_gvr_and_name",
		Description: "根据GVR（Group/Version/Resource）和名称获取指定自定义资源的详细内容",
		Parameters: map[string]any{
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
				"name": map[string]any{
					"type":        "string",
					"description": "自定义资源的名称",
				},
				"namespace": map[string]any{
					"type":        "string",
					"description": "命名空间，集群级资源不需要提供，命名空间级资源必须提供",
				},
			},
			"required": []string{"group", "version", "resource", "name"},
		},
		Factory: func(dep any) (tool.Tool, error) {
			toolDep, ok := dep.(tool.ToolDependency)
			if !ok {
				return nil, fmt.Errorf("invalid dependency type")
			}
			return NewGetCustomResourceByGvrAndNameTool(toolDep.K8sClient), nil
		},
	})
}

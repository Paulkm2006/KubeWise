package query

import (
	"context"
	"fmt"
	"strings"

	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/tool"
)

// GetConfigMapContentTool 获取指定ConfigMap的内容工具
type GetConfigMapContentTool struct {
	k8sClient *k8s.Client
}

// NewGetConfigMapContentTool 创建获取ConfigMap内容工具实例
func NewGetConfigMapContentTool(k8sClient *k8s.Client) *GetConfigMapContentTool {
	return &GetConfigMapContentTool{k8sClient: k8sClient}
}

// Name 返回工具唯一标识
func (t *GetConfigMapContentTool) Name() string {
	return "get_configmap_content"
}

// Description 返回工具功能描述
func (t *GetConfigMapContentTool) Description() string {
	return "获取指定ConfigMap的详细内容，包括标签、注解和数据项"
}

// Parameters 返回工具参数定义（JSON Schema格式）
func (t *GetConfigMapContentTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"configmapName": map[string]any{
				"type":        "string",
				"description": "ConfigMap名称",
			},
			"namespace": map[string]any{
				"type":        "string",
				"description": "ConfigMap所在的命名空间",
			},
		},
		"required": []string{"configmapName", "namespace"},
	}
}

// Execute 执行工具调用
func (t *GetConfigMapContentTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	configmapName, ok := args["configmapName"].(string)
	if !ok || configmapName == "" {
		return "", fmt.Errorf("参数configmapName不能为空")
	}

	namespace, ok := args["namespace"].(string)
	if !ok || namespace == "" {
		return "", fmt.Errorf("参数namespace不能为空")
	}

	cm, err := t.k8sClient.GetConfigMap(ctx, namespace, configmapName)
	if err != nil {
		return "", fmt.Errorf("获取ConfigMap信息失败: %w", err)
	}

	var result strings.Builder
	result.WriteString(fmt.Sprintf("ConfigMap %s/%s 的内容:\n", namespace, configmapName))
	result.WriteString(fmt.Sprintf("创建时间: %s\n", cm.CreationTimestamp.Format("2006-01-02 15:04:05")))

	if len(cm.Labels) > 0 {
		result.WriteString("\n标签:\n")
		for k, v := range cm.Labels {
			result.WriteString(fmt.Sprintf("  %s: %s\n", k, v))
		}
	}

	if len(cm.Annotations) > 0 {
		result.WriteString("\n注解:\n")
		for k, v := range cm.Annotations {
			result.WriteString(fmt.Sprintf("  %s: %s\n", k, v))
		}
	}

	if len(cm.Data) > 0 {
		result.WriteString("\n数据:\n")
		for k, v := range cm.Data {
			result.WriteString(fmt.Sprintf("  %s:\n    %s\n", k, strings.ReplaceAll(v, "\n", "\n    ")))
		}
	}

	return result.String(), nil
}

// 注册工具到全局注册中心
func init() {
	tool.RegisterGlobal(tool.ToolMetadata{
		Name:        "get_configmap_content",
		Description: "获取指定ConfigMap的详细内容，包括标签、注解和数据项",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"configmapName": map[string]any{
					"type":        "string",
					"description": "ConfigMap名称",
				},
				"namespace": map[string]any{
					"type":        "string",
					"description": "ConfigMap所在的命名空间",
				},
			},
			"required": []string{"configmapName", "namespace"},
		},
		Factory: func(dep any) (tool.Tool, error) {
			toolDep, ok := dep.(tool.ToolDependency)
			if !ok {
				return nil, fmt.Errorf("invalid dependency type")
			}
			return NewGetConfigMapContentTool(toolDep.K8sClient), nil
		},
	})
}

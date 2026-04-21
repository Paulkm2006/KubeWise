package query

import (
	"context"
	"fmt"
	"strings"

	"github.com/kubewise/kubewise/pkg/k8s"
	"github.com/kubewise/kubewise/pkg/tool"
)

// ListNamespacesTool 列出所有命名空间工具
type ListNamespacesTool struct {
	k8sClient *k8s.Client
}

// NewListNamespacesTool 创建列出命名空间工具实例
func NewListNamespacesTool(k8sClient *k8s.Client) *ListNamespacesTool {
	return &ListNamespacesTool{k8sClient: k8sClient}
}

// Name 返回工具唯一标识
func (t *ListNamespacesTool) Name() string {
	return "list_namespaces"
}

// Description 返回工具功能描述
func (t *ListNamespacesTool) Description() string {
	return "获取集群中所有命名空间的列表信息"
}

// Parameters 返回工具参数定义（JSON Schema格式）
func (t *ListNamespacesTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

// Execute 执行工具调用
func (t *ListNamespacesTool) Execute(ctx context.Context, args map[string]any) (string, error) {
	namespaces, err := t.k8sClient.ListNamespaces(ctx)
	if err != nil {
		return "", fmt.Errorf("获取命名空间列表失败: %w", err)
	}

	var result strings.Builder
	result.WriteString("命名空间列表:\n")
	result.WriteString("名称\t状态\n")
	result.WriteString("----------------\n")

	for _, ns := range namespaces {
		result.WriteString(fmt.Sprintf("%s\t%s\n", ns.Name, ns.Status.Phase))
	}

	result.WriteString(fmt.Sprintf("\n总计: %d个命名空间", len(namespaces)))
	return result.String(), nil
}

// 注册工具到全局注册中心
func init() {
	tool.RegisterGlobal(tool.ToolMetadata{
		Name:        "list_namespaces",
		Description: "获取集群中所有命名空间的列表信息",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Factory: func(dep any) (tool.Tool, error) {
			toolDep, ok := dep.(tool.ToolDependency)
			if !ok {
				return nil, fmt.Errorf("invalid dependency type")
			}
			return NewListNamespacesTool(toolDep.K8sClient), nil
		},
	})
}

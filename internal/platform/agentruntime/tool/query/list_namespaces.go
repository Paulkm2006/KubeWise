package query

import (
	"context"
	"fmt"
	"strings"

	toolv2 "github.com/kubewise/kubewise/internal/platform/agentruntime/tool/v2"
	"github.com/kubewise/kubewise/internal/platform/cluster"
)

// ListNamespacesTool 列出所有命名空间工具
type ListNamespacesTool struct {
	k8sClient *cluster.Client
}

// NewListNamespacesTool 创建列出命名空间工具实例
func NewListNamespacesTool(k8sClient *cluster.Client) *ListNamespacesTool {
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

func (t *ListNamespacesTool) Meta() toolv2.ToolMeta {
	return toolv2.TextMeta(t.Name(), "v1", t.Description(), t.Parameters(), toolv2.CapabilityRead, toolv2.RiskNone, toolv2.ConfirmNever, "query")
}

func (t *ListNamespacesTool) Execute(ctx context.Context, args map[string]any) (toolv2.ToolResult, error) {
	meta := t.Meta()
	text, err := t.executeText(ctx, args)
	if err != nil {
		return toolv2.ToolResult{}, err
	}
	return toolv2.TextResult(meta, text), nil
}

func (t *ListNamespacesTool) executeText(ctx context.Context, args map[string]any) (string, error) {
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
